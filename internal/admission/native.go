package admission

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// admittedProjectSources maps the inventory's project source names to their
// paths inside the admitted revision.
var admittedProjectSources = map[string]string{
	"project":       ".claude/settings.json",
	"project_local": ".claude/settings.local.json",
	"project_mcp":   ".mcp.json",
}

// admitNativeConfig inventories the native configuration the run will use
// (AC-2.5) and resolves its trust. Project sources come from the admitted
// revision's committed blobs, never from the mutable source checkout; the
// selection key stays the future native working directory. A manifest that
// needs trust resolves by explicit digest, by a stored grant, or by an
// interactive question; under --non-interactive it blocks naming
// --trust-native-config. Repository content can never grant that trust.
func (d *Decision) admitNativeConfig(ctx context.Context, req Request, childEnv []string) (claudecode.Manifest, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return claudecode.Manifest{}, &BlockedError{Code: "native_config_unreadable", Field: "user home",
			Action: "set HOME to the user's home directory so native settings can be inventoried"}
	}
	blobs, err := readAdmittedBlobs(ctx, d.Snapshot.SourceRepo, d.Snapshot.BaseRev)
	if err != nil {
		return claudecode.Manifest{}, err
	}
	manifest, err := claudecode.InventoryAdmittedProject(home, d.Workdir, blobs, childEnv)
	if err != nil {
		return claudecode.Manifest{}, NativeAdmissionError(err)
	}
	if d.RepoIdentity == "" {
		if d.RepoIdentity, err = RepoIdentity(d.Snapshot.SourceRepo); err != nil {
			return claudecode.Manifest{}, err
		}
	}
	if err := d.admitNativeTrust(ctx, req, manifest); err != nil {
		return claudecode.Manifest{}, err
	}
	return manifest, nil
}

// readAdmittedBlobs reads the admitted revision's project settings blobs.
// A path that is not a regular committed file fails closed: the native
// would read something this inventory cannot establish.
func readAdmittedBlobs(ctx context.Context, repo, rev string) (map[string][]byte, error) {
	blobs := map[string][]byte{}
	for name, path := range admittedProjectSources {
		entry, err := workspace.Git(ctx, repo, true, "ls-tree", rev, "--", path)
		if err != nil {
			return nil, &BlockedError{Code: "native_config_unreadable", Field: name,
				Action: "make the admitted revision readable so native settings can be inventoried"}
		}
		if len(entry) == 0 {
			continue
		}
		if !strings.HasPrefix(string(entry), "100644 blob ") && !strings.HasPrefix(string(entry), "100755 blob ") {
			return nil, &BlockedError{Code: "native_config_unreadable", Field: name,
				Action: "keep admitted native settings a regular committed file"}
		}
		raw, err := workspace.Git(ctx, repo, true, "show", rev+":"+path)
		if err != nil {
			return nil, &BlockedError{Code: "native_config_unreadable", Field: name,
				Action: "make the admitted revision readable so native settings can be inventoried"}
		}
		blobs[name] = raw
	}
	return blobs, nil
}

// admitNativeTrust resolves the manifest's trust grant. True from
// CheckNativeTrust means the supervisor must persist the explicit grant
// with the admitted event.
func (d *Decision) admitNativeTrust(ctx context.Context, req Request, manifest claudecode.Manifest) error {
	trustDB, err := openTrustJournal(ctx, req.StateDir)
	if err != nil {
		return err
	}
	if trustDB != nil {
		defer func() { _ = trustDB.Close() }()
	}
	record, err := CheckNativeTrust(ctx, trustDB, d.RepoIdentity, manifest, req.TrustNativeConfig)
	if err == nil {
		d.RecordNativeTrust = record
		d.NativeConfigDigest = manifest.Digest
		d.NativeHooks = manifest.Hooks
		// Trust holds here via the explicit flag or a stored row; a
		// grant backs it exactly when the manifest needed trusting.
		if manifest.RequiresTrust {
			d.NativeTrustGrant = "native_config:sha256:" + manifest.Digest
		}
		return nil
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "untrusted_native_config" {
		return err
	}
	ok, cerr := confirm(req, describeNativeTrust(manifest))
	if cerr != nil || !ok {
		return errors.Join(blocked, cerr)
	}
	d.RecordNativeTrust = true
	d.NativeConfigDigest = manifest.Digest
	d.NativeHooks = manifest.Hooks
	d.NativeTrustGrant = "native_config:sha256:" + manifest.Digest
	return nil
}

// openTrustJournal opens the state journal for trust reads. An
// uninitialised state directory has no grants and is not itself an error.
func openTrustJournal(ctx context.Context, stateDir string) (*journal.Journal, error) {
	j, err := journal.OpenReadOnly(ctx, stateDir)
	if errors.Is(err, journal.ErrNoDatabase) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return j, nil
}

// describeNativeTrust summarises the manifest for the trust question. The
// manifest holds digests, counts and server labels only, never native
// commands, URLs or values, so the question is safe to print.
func describeNativeTrust(manifest claudecode.Manifest) string {
	sources := make([]string, 0, len(manifest.Digests))
	for name, digest := range manifest.Digests {
		sources = append(sources, name+":sha256:"+digest)
	}
	slices.Sort(sources)
	return fmt.Sprintf("Trust native Claude Code configuration sha256:%s?\nHooks: %d\nMCP servers: %v\nSources:\n%s",
		manifest.Digest, manifest.Hooks, manifest.MCPServers, strings.Join(sources, "\n"))
}

// resolveDeclaration returns the declaration billing must check: a fresh
// --declare-entitlement assertion bound to the current native identity, or
// the stored current row for that identity, or nil when neither exists.
func resolveDeclaration(ctx context.Context, req Request, evidence AuthEvidence) (*Declaration, error) {
	if req.DeclareEntitlement != "" {
		plan, extra, err := parseDeclareEntitlement(req.DeclareEntitlement)
		if err != nil {
			return nil, err
		}
		return &Declaration{
			AdapterID:   claudecode.AdapterID,
			PlanClass:   plan,
			ExtraUsage:  extra,
			IdentityRef: evidence.IdentityRef,
			DeclaredAt:  time.Now().UTC(),
		}, nil
	}
	j, err := openTrustJournal(ctx, req.StateDir)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, nil
	}
	defer func() { _ = j.Close() }()
	decl, err := j.CurrentDeclaration(ctx, claudecode.AdapterID, evidence.IdentityRef)
	if errors.Is(err, journal.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &decl, nil
}

// parseDeclareEntitlement parses --declare-entitlement
// plan=<pro|max|team|enterprise>,extra-usage=disabled. Anything else is an
// invalid request, never a silent default.
func parseDeclareEntitlement(raw string) (plan, extra string, err error) {
	seen := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || key == "" || value == "" {
			return "", "", fmt.Errorf("%w: --declare-entitlement must be plan=<pro|max|team|enterprise>,extra-usage=disabled", ErrInvalid)
		}
		if _, dup := seen[key]; dup {
			return "", "", fmt.Errorf("%w: --declare-entitlement repeats %q", ErrInvalid, key)
		}
		seen[key] = value
	}
	if len(seen) != 2 || !slices.Contains([]string{"pro", "max", "team", "enterprise"}, seen["plan"]) || seen["extra-usage"] != "disabled" {
		return "", "", fmt.Errorf("%w: --declare-entitlement must be plan=<pro|max|team|enterprise>,extra-usage=disabled", ErrInvalid)
	}
	return seen["plan"], seen["extra-usage"], nil
}

// claudePrepareInput builds the adapter input from the admitted run. The
// task bytes are the prompt the native will read on stdin.
func claudePrepareInput(d *Decision, childEnv []string) adapter.PrepareInput {
	return adapter.PrepareInput{
		Workdir:      d.Workdir,
		AttemptID:    d.AttemptID,
		Env:          childEnv,
		Prompt:       strings.NewReader(string(d.Task.Content)),
		Probe:        d.Probe,
		AllowedTools: d.ProjectConfig.Adapters.ClaudeCode.AllowedTools,
		Passthrough:  d.ProjectConfig.Environment.Passthrough,
	}
}
