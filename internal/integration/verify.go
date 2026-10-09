package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/workspace"
)

const maxEvidence = 1 << 20

// CheckResult is the recorded outcome of one admitted check.
type CheckResult struct {
	Name           string   `json:"name"`
	Argv           []string `json:"argv"`
	Status         string   `json:"status"`
	ExitCode       *int     `json:"exit_code,omitempty"`
	DurationMS     int64    `json:"duration_ms"`
	EvidencePath   string   `json:"evidence_path,omitempty"`
	EvidenceSHA256 string   `json:"evidence_sha256,omitempty"`
}

// Verification is the recorded outcome of running the admitted checks.
type Verification struct {
	CandidateCommit string        `json:"candidate_commit"`
	ConfigSHA256    string        `json:"config_sha256"`
	Result          string        `json:"result"`
	Checks          []CheckResult `json:"checks"`
	// Evaluator names what ran the checks and digests its policy and the
	// check definitions (design §2.7).
	Evaluator   contain.Evaluator `json:"evaluator"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	Workdir     string            `json:"-"`
	EvidenceDir string            `json:"-"`
}

// RunChecks executes the admitted check list in a detached worktree of the
// frozen candidate. It never reads the candidate's mythhelm.toml.
func RunChecks(ctx context.Context, cand Candidate, cfg admission.ProjectConfig, env []string) (Verification, error) {
	return RunChecksWithOptions(ctx, cand, cfg, env, false)
}

// RunChecksWithOptions executes the admitted check list like RunChecks,
// continuing past an unavailable check when keepGoing is set.
func RunChecksWithOptions(ctx context.Context, cand Candidate, cfg admission.ProjectConfig, env []string, keepGoing bool) (Verification, error) {
	return RunChecksWithPolicy(ctx, cand, cfg, env, RunOptions{KeepGoing: keepGoing})
}

// RunOptions tunes a check run. A nil Policy runs the checks with host
// authority (trusted-host); otherwise each check runs inside the boundary with
// a read-only candidate worktree, scratch /tmp and HOME, and no credential
// binds (design §2.7).
type RunOptions struct {
	KeepGoing bool
	Policy    *contain.Policy
}

// RunChecksWithPolicy executes the admitted check list in a detached worktree
// of the frozen candidate under opts. Check definitions come only from cfg, the
// admitted configuration; the candidate's own files never define an argv.
func RunChecksWithPolicy(ctx context.Context, cand Candidate, cfg admission.ProjectConfig, env []string, opts RunOptions) (Verification, error) {
	v := Verification{CandidateCommit: cand.Commit, StartedAt: time.Now().UTC()}
	if cand.Workspace == "" || cand.Commit == "" || len(cand.Commit) < 12 {
		return v, errors.New("candidate has no managed workspace or commit")
	}
	runDir := filepath.Dir(cand.Workspace)
	verID := cand.Commit[:12]
	v.Workdir = filepath.Join(runDir, "verify", verID)
	v.EvidenceDir = filepath.Join(runDir, "evidence", verID)
	var policy *contain.Policy
	if opts.Policy != nil {
		if runtime.GOOS != "linux" {
			return v, fmt.Errorf("contained checks on %s: %w", runtime.GOOS, contain.ErrUnsupported)
		}
		if !slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "HOME=") }) {
			return v, errors.New("contained checks need HOME in the check environment")
		}
		p := *opts.Policy
		p.Workdir, p.ReadOnly, p.AuthBinds = v.Workdir, true, nil
		policy = &p
	}
	var err error
	if v.Evaluator, err = evaluatorOf(policy, cfg.Checks); err != nil {
		return v, err
	}
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
		if unavailable && !opts.KeepGoing {
			result.Status = "not_run"
		} else {
			var err error
			result, err = runCheck(ctx, v.Workdir, v.EvidenceDir, check, env, policy)
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

// evaluatorOf digests the evaluator: the boundary and canonical check policy
// when contained, "host" otherwise, plus every admitted check definition.
func evaluatorOf(policy *contain.Policy, checks []admission.CheckConfig) (contain.Evaluator, error) {
	digests := make([]string, len(checks))
	for i, c := range checks {
		b, err := json.Marshal(struct {
			Name         string   `json:"name"`
			Argv         []string `json:"argv"`
			FailOnOutput bool     `json:"fail_on_output"`
			Timeout      string   `json:"timeout"`
		}{c.Name, c.Argv, c.FailOnOutput, c.Timeout})
		if err != nil {
			return contain.Evaluator{}, err
		}
		sum := sha256.Sum256(b)
		digests[i] = hex.EncodeToString(sum[:])
	}
	if policy == nil {
		return contain.EvaluatorDigest("host", "", contain.Policy{}, digests), nil
	}
	return contain.EvaluatorDigest(contain.BoundaryName, contain.BoundaryVersion, *policy, digests), nil
}

// errCheckExecutableNotFound marks a check whose argv resolves to nothing:
// runCheck reports it unavailable without starting a process.
var errCheckExecutableNotFound = errors.New("check executable not found")

// checkCommand builds the process for one check: the argv itself, or the same
// argv inside the boundary through `mythhelm __contain` (design §2.2). A command
// that cannot be found surfaces from Start as an unavailable check.
func checkCommand(check admission.CheckConfig, dir string, env []string, policy *contain.Policy) (*exec.Cmd, error) {
	cmd := exec.Command(check.Argv[0], check.Argv[1:]...) // #nosec G204 -- reviewed, digest-bound project config, argv only
	if policy == nil {
		cmd.Dir, cmd.Env = dir, env
		setProcessGroup(cmd)
		return cmd, nil
	}
	if cmd.Err != nil {
		if errors.Is(cmd.Err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", errCheckExecutableNotFound, check.Argv[0])
		}
		// Anything else (a relative-path ErrDot, a directory) still
		// surfaces from Start as a hard start failure.
		return cmd, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locating mythhelm for __contain: %w", err)
	}
	body, err := json.Marshal(contain.ContainSpec{Path: cmd.Path, Args: check.Argv, Dir: dir, Env: env, Policy: *policy})
	if err != nil {
		return nil, err
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}
	contained := exec.Command(exe, contain.Command) // #nosec G204 -- re-executes this binary with a fixed argv
	contained.Dir, contained.Env = string(filepath.Separator), []string{}
	contained.Stdin = bytes.NewReader(body)
	// fd 3 becomes the check's stdin: checks read nothing.
	contained.ExtraFiles = []*os.File{null}
	setProcessGroup(contained)
	contain.ApplyNamespaces(contained.SysProcAttr)
	return contained, nil
}

func runCheck(ctx context.Context, dir, evidenceDir string, check admission.CheckConfig, env []string, policy *contain.Policy) (CheckResult, error) {
	r := CheckResult{Name: check.Name, Argv: redactedArgv(check.Argv)}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, check.Duration())
	defer cancel()
	cmd, err := checkCommand(check, dir, env, policy)
	if errors.Is(err, errCheckExecutableNotFound) {
		r.Status = "unavailable"
		return r, nil
	}
	if err != nil {
		return r, err
	}
	var output tailBuffer
	// Give exec an *os.File so Wait reaps only the leader. We drain the pipe
	// ourselves until EOF or the configured deadline, including descendants
	// that inherited stdout after their leader exited.
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		for _, f := range cmd.ExtraFiles {
			_ = f.Close()
		}
		return r, err
	}
	cmd.Stdout, cmd.Stderr = pipeWrite, pipeWrite
	err = cmd.Start()
	_ = pipeWrite.Close()
	for _, f := range cmd.ExtraFiles {
		_ = f.Close()
	}
	switch {
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist):
		_ = pipeRead.Close()
		r.Status = "unavailable"
	case err != nil:
		_ = pipeRead.Close()
		return r, fmt.Errorf("start check %s: %w", check.Name, err)
	default:
		waitDone, pipeDone := make(chan error, 1), make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()
		go func() { _, readErr := io.Copy(&output, pipeRead); pipeDone <- readErr }()
		var waitErr, pipeErr error
		leaderDone, streamDone := false, false
		for !leaderDone || !streamDone {
			select {
			case waitErr = <-waitDone:
				leaderDone = true
			case pipeErr = <-pipeDone:
				streamDone = true
			case <-checkCtx.Done():
				killProcessGroup(cmd)
				if !leaderDone {
					_ = cmd.Process.Kill()
				}
				_ = pipeRead.Close() // force a breakaway pipe holder to release the reader
				if !leaderDone {
					waitErr = <-waitDone
				}
				if !streamDone {
					pipeErr = <-pipeDone
				}
				leaderDone, streamDone = true, true
				r.Status = "timed_out"
			}
		}
		_ = pipeRead.Close()
		if r.Status == "" {
			var exit *exec.ExitError
			switch {
			case pipeErr != nil:
				return r, fmt.Errorf("read check %s output: %w", check.Name, pipeErr)
			case waitErr == nil:
				r.Status = "passed"
			case errors.As(waitErr, &exit):
				r.Status = "failed"
			default:
				return r, fmt.Errorf("wait check %s: %w", check.Name, waitErr)
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
