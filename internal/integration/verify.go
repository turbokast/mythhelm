package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/workspace"
)

const maxEvidence = 1 << 20

type CheckResult struct {
	Name           string   `json:"name"`
	Argv           []string `json:"argv"`
	Status         string   `json:"status"`
	ExitCode       *int     `json:"exit_code,omitempty"`
	DurationMS     int64    `json:"duration_ms"`
	EvidencePath   string   `json:"evidence_path,omitempty"`
	EvidenceSHA256 string   `json:"evidence_sha256,omitempty"`
}

type Verification struct {
	CandidateCommit string        `json:"candidate_commit"`
	ConfigSHA256    string        `json:"config_sha256"`
	Result          string        `json:"result"`
	Checks          []CheckResult `json:"checks"`
	StartedAt       time.Time     `json:"started_at"`
	FinishedAt      time.Time     `json:"finished_at"`
	Workdir         string        `json:"-"`
	EvidenceDir     string        `json:"-"`
}

// RunChecks executes the admitted check list in a detached worktree of the
// frozen candidate. It never reads the candidate's mythhelm.toml.
func RunChecks(ctx context.Context, cand Candidate, cfg admission.ProjectConfig, env []string) (Verification, error) {
	return RunChecksWithOptions(ctx, cand, cfg, env, false)
}

func RunChecksWithOptions(ctx context.Context, cand Candidate, cfg admission.ProjectConfig, env []string, keepGoing bool) (Verification, error) {
	v := Verification{CandidateCommit: cand.Commit, StartedAt: time.Now().UTC()}
	if cand.Workspace == "" || cand.Commit == "" || len(cand.Commit) < 12 {
		return v, errors.New("candidate has no managed workspace or commit")
	}
	runDir := filepath.Dir(cand.Workspace)
	verID := cand.Commit[:12]
	v.Workdir = filepath.Join(runDir, "verify", verID)
	v.EvidenceDir = filepath.Join(runDir, "evidence", verID)
	if err := ensureRealDir(filepath.Dir(v.Workdir)); err != nil {
		return v, err
	}
	if err := ensureRealDir(filepath.Dir(v.EvidenceDir)); err != nil {
		return v, err
	}
	if err := os.Mkdir(v.EvidenceDir, 0o700); err != nil {
		return v, err
	}
	if _, err := workspace.Git(ctx, cand.Workspace, false, "worktree", "add", "--detach", v.Workdir, cand.Commit); err != nil {
		return v, fmt.Errorf("create candidate verification worktree: %w", err)
	}
	defer func() {
		_, _ = workspace.Git(context.Background(), cand.Workspace, false, "worktree", "remove", "--force", v.Workdir)
	}()
	bad, unavailable := false, false
	for _, check := range cfg.Checks {
		result := CheckResult{Name: check.Name, Argv: redactedArgv(check.Argv)}
		if unavailable && !keepGoing {
			result.Status = "not_run"
		} else {
			var err error
			result, err = runCheck(ctx, v.Workdir, v.EvidenceDir, check, env)
			if err != nil {
				return v, err
			}
			if result.Status != "passed" {
				bad = true
			}
			if result.Status == "unavailable" {
				unavailable = true
			}
		}
		v.Checks = append(v.Checks, result)
	}
	if bad {
		v.Result = "failed"
	} else {
		v.Result = "passed"
	}
	v.FinishedAt = time.Now().UTC()
	return v, nil
}

func runCheck(ctx context.Context, dir, evidenceDir string, check admission.CheckConfig, env []string) (CheckResult, error) {
	r := CheckResult{Name: check.Name, Argv: redactedArgv(check.Argv)}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, check.Duration())
	defer cancel()
	cmd := exec.Command(check.Argv[0], check.Argv[1:]...) // #nosec G204 -- reviewed, digest-bound project config, argv only
	cmd.Dir, cmd.Env = dir, env
	var output tailBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	setProcessGroup(cmd)
	err := cmd.Start()
	if errors.Is(err, exec.ErrNotFound) {
		r.Status = "unavailable"
	} else if err != nil {
		return r, fmt.Errorf("start check %s: %w", check.Name, err)
	} else {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-checkCtx.Done():
			killProcessGroup(cmd)
			err = <-done
			r.Status = "timed_out"
		}
		if r.Status == "" {
			var exit *exec.ExitError
			switch {
			case err == nil:
				r.Status = "passed"
			case errors.As(err, &exit):
				r.Status = "failed"
			default:
				return r, fmt.Errorf("wait check %s: %w", check.Name, err)
			}
		}
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			r.ExitCode = &code
		}
	}
	r.DurationMS = time.Since(started).Milliseconds()
	raw := output.data
	// If the ring discarded a prefix, the first retained token may be a
	// fragment of a secret whose identifying prefix was discarded. Drop that
	// partial line before redaction; a very long line yields empty evidence.
	if output.count > int64(len(raw)) {
		if nl := bytes.IndexByte(raw, '\n'); nl >= 0 {
			raw = raw[nl+1:]
		} else {
			raw = nil
		}
	}
	redacted := []byte(security.Redact(string(raw)))
	if len(redacted) > maxEvidence {
		redacted = redacted[len(redacted)-maxEvidence:]
	}
	evidence := filepath.Join(evidenceDir, check.Name+".log")
	if err := ensureRealDir(evidenceDir); err != nil {
		return r, err
	}
	f, err := os.OpenFile(evidence, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // Exclusive create in a checked managed evidence dir.
	if err != nil {
		return r, err
	}
	if _, err := f.Write(redacted); err != nil {
		_ = f.Close()
		return r, err
	}
	if err := f.Close(); err != nil {
		return r, err
	}
	sum := sha256.Sum256(redacted)
	r.EvidencePath, r.EvidenceSHA256 = evidence, hex.EncodeToString(sum[:])
	if r.Status == "passed" && check.FailOnOutput && output.count > 0 {
		r.Status = "failed"
	}
	return r, nil
}

// tailBuffer keeps the last MiB while draining both pipes to completion.
type tailBuffer struct {
	mu    sync.Mutex
	data  []byte
	count int64
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.count += int64(n)
	if n >= maxEvidence {
		b.data = append(b.data[:0], p[n-maxEvidence:]...)
	} else {
		b.data = append(b.data, p...)
		if len(b.data) > maxEvidence {
			b.data = append(b.data[:0], b.data[len(b.data)-maxEvidence:]...)
		}
	}
	return n, nil
}

var _ io.Writer = (*tailBuffer)(nil)

func redactedArgv(argv []string) []string {
	out := make([]string, len(argv))
	for i, arg := range argv {
		out[i] = security.Redact(arg)
	}
	return out
}

func ensureRealDir(path string) error {
	err := os.Mkdir(path, 0o700)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed directory %s is not a real directory", path)
	}
	return nil
}
