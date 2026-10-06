package admission

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// ProjectConfigTrustKind is the trust-grant kind for admitted project configuration digests.
const ProjectConfigTrustKind = "project_config"

// RepoIdentity is the real source checkout path bound to a config grant.
func RepoIdentity(repo string) (string, error) {
	canonical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return "", fmt.Errorf("resolve repository identity: %w", err)
	}
	return filepath.Abs(canonical)
}

// HasTrust reports whether the local journal has a matching grant. An
// uninitialised state directory has no grants and is not itself an error.
func HasTrust(ctx context.Context, j *journal.Journal, kind, repoID, digest string) (bool, error) {
	if j == nil {
		return false, nil
	}
	return j.TrustGrantExists(ctx, kind, repoID, digest)
}

func storedProjectTrust(ctx context.Context, stateDir, repoID, digest string) (bool, error) {
	j, err := journal.OpenReadOnly(ctx, stateDir)
	if errors.Is(err, journal.ErrNoDatabase) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = j.Close() }()
	return HasTrust(ctx, j, ProjectConfigTrustKind, repoID, digest)
}

// admitProjectConfig reads the selected committed revision, never the source
// checkout. The supervisor checks the clone against this digest before launch.
func (d *Decision) admitProjectConfig(ctx context.Context, req Request) error {
	if d.NoChecks {
		if req.TrustProjectConfig != "" {
			return fmt.Errorf("%w: --trust-project-config has no effect with --no-checks", ErrInvalid)
		}
		return nil
	}
	entry, err := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "ls-tree", d.Snapshot.BaseRev, "--", ProjectConfigFile)
	if err != nil {
		return fmt.Errorf("%w: locate config in admitted revision: %w", ErrProjectConfig, err)
	}
	if len(entry) == 0 {
		return &BlockedError{Code: "no_checks_configured", Field: ProjectConfigFile,
			Action: "commit mythhelm.toml or pass --no-checks"}
	}
	if !strings.HasPrefix(string(entry), "100644 blob ") && !strings.HasPrefix(string(entry), "100755 blob ") {
		return fmt.Errorf("%w: mythhelm.toml must be a regular committed file", ErrProjectConfig)
	}
	raw, err := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "show", d.Snapshot.BaseRev+":"+ProjectConfigFile)
	if err != nil {
		return fmt.Errorf("%w: read admitted config: %w", ErrProjectConfig, err)
	}
	if d.ProjectConfig, d.ConfigDigest, err = ParseProjectConfig(raw); err != nil {
		return err
	}
	if len(d.ProjectConfig.Checks) == 0 {
		return &BlockedError{Code: "no_checks_configured", Field: ProjectConfigFile,
			Action: "configure at least one check or pass --no-checks"}
	}
	if d.RepoIdentity, err = RepoIdentity(d.Snapshot.SourceRepo); err != nil {
		return err
	}
	if req.TrustProjectConfig != "" {
		if req.TrustProjectConfig != "sha256:"+d.ConfigDigest {
			return fmt.Errorf("%w: --trust-project-config must match sha256:%s", ErrInvalid, d.ConfigDigest)
		}
		d.RecordTrust = true
		return nil
	}
	trusted, err := storedProjectTrust(ctx, req.StateDir, d.RepoIdentity, d.ConfigDigest)
	if err != nil {
		return err
	}
	if trusted {
		return nil
	}
	question := fmt.Sprintf("Trust committed mythhelm.toml sha256:%s?\nChecks:\n%s\nAllowed tools: %v\nEnvironment passthrough: %v",
		d.ConfigDigest, describeChecks(d.ProjectConfig.Checks), d.ProjectConfig.Adapters.ClaudeCode.AllowedTools, d.ProjectConfig.Environment.Passthrough)
	ok, err := confirm(req, question)
	if err != nil || !ok {
		return errors.Join(&BlockedError{Code: "untrusted_project_config", Field: "sha256:" + d.ConfigDigest,
			Action: "review the committed config and pass --trust-project-config sha256:" + d.ConfigDigest}, err)
	}
	d.RecordTrust = true
	return nil
}

func describeChecks(checks []CheckConfig) string {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = fmt.Sprintf("- %s: %q (timeout %s, fail_on_output %t)", c.Name, c.Argv, c.Timeout, c.FailOnOutput)
	}
	return strings.Join(names, "\n")
}
