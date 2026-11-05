package supervisor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// Interrupt the real CLI after it journals intent by temporarily removing the
// frozen workspace ref. ApplyBranch fails before creating the destination;
// the next CLI process must recover the original decision from the journal.
func interruptedOverride(t *testing.T, acceptFlags bool) (fixture, string, string, map[string]any) {
	t.Helper()
	f, runID := readyApplyFixture(t)
	taskPath := filepath.Join(f.state, "runs", runID, "task.md")
	original, err := os.ReadFile(taskPath) // #nosec G304 -- fixed task file in the test-owned run directory
	if err != nil {
		t.Fatal(err)
	}
	staleCases[0].tamper(t, f, runID)
	status, err := supervisor.CheckAcceptance(t.Context(), f.journal(t), runID)
	if err != nil || status.State != supervisor.AcceptanceStale {
		t.Fatalf("stale gate: %+v, %v", status, err)
	}
	attempt, err := f.journal(t).LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.candidateOf(t, runID)
	ref := "refs/mythhelm/candidates/" + attempt.AttemptID
	if _, err := workspace.Git(t.Context(), attempt.WorkspacePath, false, "update-ref", "-d", ref); err != nil {
		t.Fatal(err)
	}
	branch := "review/interrupted-override"
	args := []string{"apply", runID, "--to-branch", branch, "--accept-unverified"}
	if acceptFlags {
		args = append(args, "--accept-flags")
	}
	code, _, stderr := f.run(t, args...)
	if code == 0 || !strings.Contains(stderr, "candidate ref does not match frozen commit") {
		t.Fatalf("post-intent failure exit %d: %s", code, stderr)
	}
	if f.onlyRun(t).State != string(supervisor.RunApplying) {
		t.Fatal("failure did not leave a real applying run")
	}
	if got, err := workspace.BranchCommit(t.Context(), f.repo, branch); err != nil || got != "" {
		t.Fatalf("interrupted apply created branch: %q %v", got, err)
	}
	if typeIndex(f.events(t, runID), "apply.intent_recorded") < 0 {
		t.Fatal("no durable intent before failure")
	}
	if _, err := workspace.Git(t.Context(), attempt.WorkspacePath, false, "update-ref", ref, candidate.Commit); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath, original, 0600); err != nil { //nolint:gosec // G703: restoring the test-owned task file
		t.Fatal(err)
	}
	if status, err := supervisor.CheckAcceptance(t.Context(), f.journal(t), runID); err != nil || status.State != supervisor.AcceptanceBound {
		t.Fatalf("restored gate: %+v %v", status, err)
	}
	return f, runID, branch, map[string]any{"state": string(status.State), "reason": status.Reason, "changed": []any{"contract"}}
}

func TestApplyOverrideSurvivesDurableIntentInterruption(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                 string
		recover, acceptFlags bool
	}{
		{name: "apply without repeated flags"}, {name: "recover with original flags", recover: true, acceptFlags: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, runID, branch, want := interruptedOverride(t, tc.acceptFlags)
			args := []string{"apply", runID, "--to-branch", branch}
			if tc.recover {
				args = []string{"recover", runID}
			}
			if code, _, stderr := f.run(t, args...); code != 0 {
				t.Fatalf("resume exit %d: %s", code, stderr)
			}
			got, ok := applyOverride(t, f, runID)
			// I09 (v2 §2): preserve the original stale verdict, even though the
			// current gate is now bound and the resuming command supplied no flags.
			if !ok || !reflect.DeepEqual(got, want) {
				t.Fatalf("resumed override = %#v, want %#v", got, want)
			}
			var intent, completed map[string]any
			for _, ev := range f.events(t, runID) {
				if ev.Type == "apply.intent_recorded" || ev.Type == "apply.completed" {
					var payload map[string]any
					if err := json.Unmarshal(ev.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					if ev.Type == "apply.intent_recorded" {
						intent = payload
					} else {
						completed = payload
					}
				}
			}
			decision, _ := intent["decision"].(map[string]any)
			if decision["accept_unverified"] != true || decision["accept_flags"] != tc.acceptFlags || !reflect.DeepEqual(decision["override"], want) {
				t.Fatalf("durable decision = %#v", decision)
			}
			if !reflect.DeepEqual(intent, completed) {
				t.Fatalf("completion changed intent: %#v != %#v", completed, intent)
			}
		})
	}
}

func TestRecoverAdoptsOnlyJournaledOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "genuine"},
		{name: "missing override", mutate: func(r map[string]any) { delete(r["acceptance"].(map[string]any), "apply_override") }},
		{name: "forged reason", mutate: func(r map[string]any) {
			r["acceptance"].(map[string]any)["apply_override"].(map[string]any)["reason"] = "forged"
		}},
		{name: "forged dimension", mutate: func(r map[string]any) {
			r["acceptance"].(map[string]any)["apply_override"].(map[string]any)["changed"] = []any{"candidate"}
		}},
		{name: "unrelated acceptance edit", mutate: func(r map[string]any) { r["acceptance"].(map[string]any)["forged"] = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, runID := readyApplyFixture(t)
			staleCases[0].tamper(t, f, runID)
			if code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/adopt-override", "--accept-unverified"); code != 0 {
				t.Fatalf("apply exit %d: %s", code, stderr)
			}
			path := filepath.Join(f.state, "runs", runID, "receipt.json")
			dropV2ReceiptJournal(t, f, runID)
			if tc.mutate != nil {
				raw, _ := receiptFileSHA(t, path)
				var r map[string]any
				if err := json.Unmarshal(raw, &r); err != nil {
					t.Fatal(err)
				}
				tc.mutate(r)
				raw, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, beforeSHA := receiptFileSHA(t, path)
			beforeEvents := len(f.events(t, runID))
			beforeSource := f.fingerprint(t)
			code, _, stderr := f.run(t, "recover", runID)
			if tc.mutate == nil {
				if code != 0 {
					t.Fatalf("genuine override recovery exit %d: %s", code, stderr)
				}
				if digest, schema := latestReceiptDigest(t, f, runID); digest != beforeSHA || schema != 2 {
					t.Fatal("genuine override receipt not adopted")
				}
			} else {
				if code != 6 || !strings.Contains(stderr, "differs from the verified v1") {
					t.Fatalf("forged override recovery exit %d: %s", code, stderr)
				}
				if len(f.events(t, runID)) != beforeEvents {
					t.Fatal("refused adoption changed journal")
				}
			}
			if _, afterSHA := receiptFileSHA(t, path); afterSHA != beforeSHA {
				t.Fatal("adoption rewrote receipt")
			}
			if f.fingerprint(t) != beforeSource {
				t.Fatal("adoption changed source")
			}
		})
	}
}

func TestApplyRejectsMismatchedDurableDecision(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"apply", "recover"} {
		for _, change := range []string{"flags", "verdict", "unauthorised override"} {
			t.Run(command+"/"+change, func(t *testing.T) {
				t.Parallel()
				f, runID, branch, _ := interruptedOverride(t, false)
				var payload map[string]any
				for _, ev := range f.events(t, runID) {
					if ev.Type == "apply.intent_recorded" {
						if err := json.Unmarshal(ev.Payload, &payload); err != nil {
							t.Fatal(err)
						}
					}
				}
				decision, ok := payload["decision"].(map[string]any)
				if !ok {
					t.Fatal("intent has no durable decision")
				}
				switch change {
				case "flags":
					decision["accept_flags"] = true
				case "verdict":
					decision["override"].(map[string]any)["reason"] = "different decision"
				case "unauthorised override":
					decision["accept_unverified"] = false
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.journal(t).Append(t.Context(), journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID,
					ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "apply.completed", Payload: raw}, nil); err != nil {
					t.Fatal(err)
				}
				events := len(f.events(t, runID))
				before := f.fingerprint(t)
				receiptPath := filepath.Join(f.state, "runs", runID, "receipt.json")
				_, beforeSHA := receiptFileSHA(t, receiptPath)
				args := []string{command, runID}
				wantCode := 6
				if command == "apply" {
					args = append(args, "--to-branch", branch)
					wantCode = 3
				}
				code, _, stderr := f.run(t, args...)
				if code != wantCode {
					t.Fatalf("mismatched decision exit %d: %s, want %d", code, stderr, wantCode)
				}
				if len(f.events(t, runID)) != events || f.fingerprint(t) != before {
					t.Fatal("mismatched decision changed journal or source")
				}
				if _, sha := receiptFileSHA(t, receiptPath); sha != beforeSHA {
					t.Fatal("mismatched decision changed receipt")
				}
				if got, err := workspace.BranchCommit(t.Context(), f.repo, branch); err != nil || got != "" {
					t.Fatalf("mismatched decision created branch %q: %v", got, err)
				}
			})
		}
	}
}

func TestLegacyApplyIntentDoesNotInventDecision(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	branch := "review/legacy-intent"
	candidate := f.candidateOf(t, runID)
	recordApplyIntent(t, f, runID, branch, candidate.Commit)
	staleCases[0].tamper(t, f, runID)
	if code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch, "--accept-flags", "--accept-unverified"); code != 0 {
		t.Fatalf("legacy resume exit %d: %s", code, stderr)
	}
	// I09 (v2 §2): the new invocation cannot supply missing historical evidence.
	if override, ok := applyOverride(t, f, runID); ok {
		t.Fatalf("legacy intent invented override %#v", override)
	}
	for _, ev := range f.events(t, runID) {
		if ev.Type == "apply.completed" {
			var payload map[string]any
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if _, ok := payload["decision"]; ok {
				t.Fatalf("legacy completion invented decision %#v", payload)
			}
		}
	}
	path := filepath.Join(f.state, "runs", runID, "receipt.json")
	_, sha := receiptFileSHA(t, path)
	dropV2ReceiptJournal(t, f, runID)
	if code, _, stderr := f.run(t, "recover", runID); code != 0 {
		t.Fatalf("legacy adoption exit %d: %s", code, stderr)
	}
	if digest, schema := latestReceiptDigest(t, f, runID); digest != sha || schema != 2 {
		t.Fatal("legacy receipt not adopted exactly")
	}
}
