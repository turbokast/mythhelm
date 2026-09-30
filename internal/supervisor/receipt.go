package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
)

// Receipt is the version 1, reviewable record described by design §9.
// Values are assembled from an explicit allowlist of journal fields; raw
// native output, task content, environment and settings never enter it.
type Receipt map[string]any

const unknown = "unknown"

func known(s string) string {
	if s == "" {
		return unknown
	}
	return security.Redact(s)
}

func eventValue(m map[string]any, key string) any {
	v := m[key]
	if v == nil || v == "" {
		return unknown
	}
	if s, ok := v.(string); ok {
		return known(s)
	}
	return v
}

// BuildReceipt reads the durable run and attempt projections, plus the
// admission and native-result observations. Unknown measurements remain
// explicitly unknown, including when an attempt never started.
func BuildReceipt(ctx context.Context, j *journal.Journal, runID string) (Receipt, error) {
	run, err := j.Run(ctx, runID)
	if err != nil {
		return nil, err
	}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return nil, err
	}
	var adm admission.Record
	admitted := false
	var native map[string]any
	var session string
	var nativeSession map[string]any
	var denials []string
	for _, ev := range events {
		switch ev.Type {
		case "admission.decided":
			if err := json.Unmarshal(ev.Payload, &adm); err != nil {
				return nil, fmt.Errorf("receipt: admission: %w", err)
			}
			admitted = true
		case "attempt.native_result":
			if err := json.Unmarshal(ev.Payload, &native); err != nil {
				return nil, fmt.Errorf("receipt: native result: %w", err)
			}
		case "attempt.native_session":
			if err := json.Unmarshal(ev.Payload, &nativeSession); err != nil {
				return nil, fmt.Errorf("receipt: native session: %w", err)
			}
			if s, ok := nativeSession["session_id"].(string); ok {
				session = s
			}
		case "attempt.permission_denied":
			var m map[string]any
			if err := json.Unmarshal(ev.Payload, &m); err != nil {
				return nil, err
			}
			denials = append(denials, known(fmt.Sprint(m["tool_name"])))
		}
	}
	if native == nil {
		native = map[string]any{}
	}
	if nativeSession == nil {
		nativeSession = map[string]any{}
	}
	if denials == nil {
		denials = []string{}
	}
	profile := known(run.ExecutionProfile)
	if profile != unknown && profile == "trusted-host" {
		profile += " (not contained)"
	}
	digests := map[string]any{"user": unknown, "project": unknown}
	for _, source := range []string{"user", "project"} {
		if digest := adm.ConfigManifest.Digests[source]; digest != "" {
			digests[source] = known(digest)
		}
	}
	fidelity := []string{}
	for _, delta := range adm.Overrides {
		// Only the setting name and reason are safe to persist. Values can be credentials.
		fidelity = append(fidelity, known(delta.Name)+": "+known(delta.Reason))
	}
	if fidelity == nil {
		fidelity = []string{}
	}
	price := map[string]any{"value": unknown, "source": unknown, "note": "estimate, not a charge"}
	if v := eventValue(native, "retail_equivalent_estimate_usd"); v != unknown {
		price["value"], price["source"] = v, "native-reported"
	}
	usage := eventValue(native, "usage_native_reported")
	tokens := map[string]any{"source": unknown, "by_model": unknown}
	if usage != unknown {
		tokens["source"], tokens["by_model"] = "native-reported", usage
	}
	exit := receiptExit(run.State, run.Reason)
	dirty := any(unknown)
	qualified := any(unknown)
	if admitted {
		dirty = adm.Snapshot.Dirty
		qualified = adm.Billing.Qualified
	}
	mcp := any(unknown)
	if v := nativeSession["mcp"]; v != nil {
		mcp = v
	}
	taskTitle := unknown
	taskPath := filepath.Join(j.StateDir(), "runs", runID, taskFile)
	if b, err := os.ReadFile(taskPath); err == nil { // #nosec G304 -- state directory and generated run ID
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != run.TaskSHA256 {
			return nil, fmt.Errorf("receipt: admitted task digest mismatch for %s", runID)
		}
		taskTitle = known(admission.TaskTitle(b))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("receipt: reading admitted task: %w", err)
	}
	nativeAuth := any(unknown)
	if adm.NativeAuth != nil {
		nativeAuth = map[string]any{"logged_in": adm.NativeAuth.LoggedIn, "auth_method": known(adm.NativeAuth.AuthMethod),
			"api_provider": known(adm.NativeAuth.APIProvider), "subscription_type": known(adm.NativeAuth.SubscriptionType),
			"identity_ref": known(adm.NativeAuth.IdentityRef)}
	}
	r := Receipt{
		"schema_version": 1, "run_id": run.RunID, "state": run.State, "exit_code": exit,
		"requested_outcome": map[string]any{"task_file_sha256": known(run.TaskSHA256), "title": taskTitle, "deliverable": "review-candidate"},
		"admitted_snapshot": map[string]any{"source_repo": known(run.SourceRepo), "branch": known(run.SourceBranch), "base_rev": known(run.BaseRev), "dirty_at_admission": dirty},
		"execution_bundle": map[string]any{"harness": known(adm.Adapter.Harness), "adapter": known(run.AdapterID), "surface": known(adm.Adapter.Surface),
			"native_version": known(adm.Native.Version), "native_sha256": known(adm.Native.SHA256), "model": eventValue(nativeSession, "model"),
			"permission_mode": eventValue(nativeSession, "permission_mode"), "allowed_tools": unknown, "execution_profile": profile, "compatibility": known(adm.Native.Compatibility)},
		"fidelity_differences": fidelity,
		"native_configuration": map[string]any{"settings_digests": digests, "hooks": unknown, "mcp_servers": mcp, "trust_grant": unknown},
		"billing": map[string]any{"mode": known(run.BillingPosture), "qualified": qualified, "g05": known(adm.Billing.G05),
			"credential_provenance": known(adm.Billing.CredentialProvenance), "entitlement_class": known(adm.Billing.EntitlementClass),
			"entitlement_source": known(adm.Billing.EntitlementSource), "paid_continuation": known(adm.Billing.PaidContinuation),
			"paid_continuation_user_declaration": known(adm.Billing.PaidContinuationUserDeclaration), "init_api_key_source": eventValue(nativeSession, "auth_source"),
			"retail_equivalent_estimate_usd": price, "tokens": tokens, "native_auth": nativeAuth},
		"routing": map[string]any{"decision": "pinned by --adapter; no routing in this slice"},
		"native_result": map[string]any{"attempt_state": unknown, "subtype": eventValue(native, "subtype"), "num_turns": eventValue(native, "num_turns"),
			"duration_ms": eventValue(native, "duration_ms"), "session_id": known(session), "permission_denials": denials},
		"candidate": map[string]any{"base_rev": unknown, "commit": unknown, "tree_id": unknown, "patch_sha256": unknown,
			"changed_paths": unknown, "flags": []any{}, "integration": "single candidate on admitted snapshot; candidate is the combined revision"},
		"verification":     map[string]any{"config_sha256": unknown, "baseline": "not-run", "checks": []any{}},
		"external_effects": []any{}, "execution_host": map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH},
		"host_integration":       "standalone (Herdr out of scope)",
		"unknowns":               []string{"whether any inference request preceded the init event", "native telemetry egress", "descendants outside the process group"},
		"remaining_human_action": []string{"review the diff", "mythhelm apply <run> --to-branch <b>", "sign off (DCO) after review"},
	}
	attempt, err := j.LatestAttempt(ctx, runID)
	if err != nil && !errors.Is(err, journal.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		r["native_result"].(map[string]any)["attempt_state"] = known(attempt.State)
		c, err := j.Candidate(ctx, attempt.AttemptID)
		if err != nil && !errors.Is(err, journal.ErrNotFound) {
			return nil, err
		}
		if err == nil {
			changed := []any{}
			flags := []any{}
			if err := json.Unmarshal(c.ChangedPaths, &changed); err != nil {
				return nil, err
			}
			if err := json.Unmarshal(c.Flags, &flags); err != nil {
				return nil, err
			}
			r["candidate"] = map[string]any{"base_rev": known(c.BaseRev), "commit": known(c.Commit), "tree_id": known(c.Tree),
				"patch_sha256": known(c.PatchSHA256), "changed_paths": len(changed), "flags": flags,
				"integration": "single candidate on admitted snapshot; candidate is the combined revision"}
		}
	}
	v, err := j.LatestVerification(ctx, runID)
	if err != nil && !errors.Is(err, journal.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		checks := []any{}
		for _, c := range v.Checks {
			code := any(unknown)
			if c.ExitCode != nil {
				code = *c.ExitCode
			}
			checks = append(checks, map[string]any{"name": known(c.Name), "status": known(c.Status), "exit_code": code,
				"evidence": known(c.EvidencePath), "sha256": known(c.EvidenceSHA256)})
		}
		r["verification"] = map[string]any{"config_sha256": known(v.ConfigSHA256), "baseline": "not-run", "checks": checks}
	}
	return r, nil
}

func receiptExit(state, reason string) int {
	switch state {
	case string(RunReadyForReview):
		if reason == "unverified" {
			return 5
		}
		return 0
	case string(RunBlocked):
		return 3
	case string(RunCancelled):
		return 130
	case string(RunInterrupted):
		return 6
	case string(RunFailed):
		if reason == "native_failed" || reason == "protocol_error" {
			return 4
		}
		if reason == "verification_failed" || reason == "verification_unavailable" {
			return 5
		}
	}
	return 1
}

// WriteReceipt atomically installs a private JSON receipt and returns the
// digest of the exact file bytes. The caller journals that digest afterward.
func WriteReceipt(dir string, r Receipt) (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	sum := sha256.Sum256(b)
	f, err := os.CreateTemp(dir, ".receipt-*.json")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, "receipt.json")); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum[:]), nil
}
