package control_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

type attachmentAuthFixture struct {
	j      *journal.Journal
	lease  *control.InstanceLease
	owner  *supervisor.ServiceOwner
	owners map[string]*supervisor.ServiceOwner
	gate   sync.RWMutex
	repo   string
}

func newAttachmentFixture(t *testing.T) *attachmentAuthFixture {
	t.Helper()
	// Sequential: instance-lock isolation uses the process environment.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()
	j, err := journal.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	f := &attachmentAuthFixture{j: j, repo: filepath.Join(t.TempDir(), "recorded-repository")}
	err = j.Transact(t.Context(), func(tx *sql.Tx) error {
		attachmentExec(t, tx, `UPDATE migration_state SET phase='adopted' WHERE id=1`)
		for _, id := range []string{"run_attachment", "run_other"} {
			attachmentExec(t, tx, `INSERT INTO runs VALUES (?, 'executing', NULL, 'fake', ?, NULL, NULL, 'task-digest', 'local-scripted', 'trusted-host', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, id, f.repo)
		}
		attachmentExec(t, tx, `INSERT INTO attempts (attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,worker_pid,worker_start_time,workspace_path) VALUES ('att_attachment','run_attachment','task_attachment',1,'executing','immutable-launch-digest',123,'synthetic-start','synthetic-workspace')`)
		attachmentExec(t, tx, `INSERT INTO capability_tokens VALUES ('att_attachment','immutable-attempt-digest','2026-01-01T00:00:00Z')`)
		attachmentExec(t, tx, `INSERT INTO operations VALUES ('op_assignment','synthetic.seed','run_attachment','synthetic-digest','done',1,'{}','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`)
		attachmentExec(t, tx, `INSERT INTO run_assignments VALUES ('run_attachment','synthetic-owner-label','2026-01-01T00:00:00Z','op_assignment')`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.openOwner(t)
	// Seed nonempty synthetic launch provenance in the real ledger. This is
	// retained evidence for isolation assertions, not a claim a worker launched.
	gen := f.lease.Generation()
	record, err := json.Marshal(control.LaunchProvenanceV1{
		SchemaVersion: 1, RunID: "run_attachment", AttemptID: "att_attachment",
		LaunchTokenSHA256: "immutable-launch-digest", NonceSHA256: "synthetic-nonce-digest",
		LadderVersion: "synthetic-ladder-v1", Generation: &gen,
		ExecutionDeadline: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(control.AdmissionPinnedPayload{LadderVersion: "synthetic-ladder-v1", NonceSHA256: "synthetic-nonce-digest", LaunchProvenance: record})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.j.Transact(t.Context(), func(tx *sql.Tx) error {
		attachmentExec(t, tx, `INSERT INTO journal (event_id,schema_version,run_id,task_id,attempt_id,producer_id,producer_sequence,run_sequence,generation,observed_at,type,payload)
		VALUES ('evt_launch',1,'run_attachment','task_attachment','att_attachment','synthetic-producer',1,1,?,'2026-01-01T00:00:00Z',?,?)`, gen, control.EventAdmissionPinned, string(payload))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := control.ReadLaunchProvenance(t.Context(), f.j, "run_attachment", "att_attachment"); err != nil {
		t.Fatalf("fixture launch evidence: %v", err)
	}
	return f
}

func (f *attachmentAuthFixture) openOwner(t *testing.T) {
	t.Helper()
	var err error
	f.lease, err = control.AcquireInstanceLease(f.j.StateDir())
	if err != nil {
		t.Fatal(err)
	}
	lease := f.lease
	t.Cleanup(func() { _ = lease.Close() })
	f.owner, err = supervisor.AcquireServiceOwner(t.Context(), f.j, f.lease, "run_attachment")
	if err != nil {
		t.Fatal(err)
	}
	owner := f.owner
	f.owners = map[string]*supervisor.ServiceOwner{"run_attachment": owner}
	t.Cleanup(func() { _ = owner.Close() })
}

func (f *attachmentAuthFixture) prepareOwner(t *testing.T, run string) {
	t.Helper()
	owner, err := supervisor.AcquireServiceOwner(t.Context(), f.j, f.lease, run)
	if err != nil {
		t.Fatal(err)
	}
	f.owners[run] = owner
	t.Cleanup(func() { _ = owner.Close() })
}

// transact honours the trusted-core precondition with actual capabilities.
// These SQL primitives cannot acquire or prove ownership themselves.
func (f *attachmentAuthFixture) transact(t *testing.T, write bool, fn func(*sql.Tx, int64) error) error {
	t.Helper()
	return f.transactFor(t, "run_attachment", write, fn)
}

func (f *attachmentAuthFixture) transactFor(t *testing.T, run string, write bool, fn func(*sql.Tx, int64) error) error {
	t.Helper()
	if write {
		f.gate.Lock()
		defer f.gate.Unlock()
	} else {
		f.gate.RLock()
		defer f.gate.RUnlock()
	}
	releaseOwner, err := f.owners[run].RetainObservation(t.Context(), f.j, run)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOwner()
	releaseInstance, err := f.lease.Hold()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseInstance()
	return f.j.Transact(t.Context(), func(tx *sql.Tx) error { return fn(tx, f.lease.Generation()) })
}

func attachmentExec(t *testing.T, tx *sql.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func attachmentCode(t *testing.T, err error, code control.Code) {
	t.Helper()
	var typed *control.Error
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("error = %v; want typed %s", err, code)
	}
	if typed.Message != "attachment unavailable" {
		t.Errorf("unsafe error message: %q", typed.Message)
	}
}

func (f *attachmentAuthFixture) mint(t *testing.T) control.AttachmentGrant {
	t.Helper()
	var grant control.AttachmentGrant
	err := f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		var err error
		grant, err = control.MintAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, "run_attachment", gen)
		return err
	})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if err := grant.Validate(); err != nil {
		t.Fatalf("minted grant: %v", err)
	}
	return grant
}

func TestAttachmentGrantScope(t *testing.T) { // I03/I17 (v2 §§8/14): pane text grants no authority.
	f := newAttachmentFixture(t)
	// Acquire each target's real owner before any SQL primitive probe. Even an
	// absent ledger row can have a previously acquired owner; absence is not
	// simulated by substituting a run_assignments label for that capability.
	f.prepareOwner(t, "run_other")
	f.prepareOwner(t, "run_unknown")
	g := f.mint(t)
	if g.RunID != "run_attachment" || g.Generation != f.lease.Generation() {
		t.Fatalf("wrong grant scope: %+v", g)
	}
	encoded := []byte("mythhelm.attachment.repository.v1\x00")
	encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(f.repo)))
	encoded = append(encoded, f.repo...)
	wantRepo := sha256.Sum256(encoded)
	if g.RepositoryID != hex.EncodeToString(wantRepo[:]) || g.RepositoryID == f.repo {
		t.Fatalf("repository identity = %q", g.RepositoryID)
	}
	second := f.mint(t)
	if second.ClientID == g.ClientID || second.Token == g.Token || second.AttachmentID == g.AttachmentID {
		t.Fatal("mint reused core identity or token")
	}
	if err := f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		var hash, client, repo string
		if err := tx.QueryRowContext(t.Context(), `SELECT token_sha256,client_id,repository_id FROM attachment_grants WHERE attachment_id=?`, g.AttachmentID).Scan(&hash, &client, &repo); err != nil {
			return err
		}
		want := sha256.Sum256([]byte(g.Token))
		if hash != hex.EncodeToString(want[:]) || hash == g.Token || client != g.ClientID || repo != g.RepositoryID {
			t.Fatal("stored scope/hash differs from grant")
		}
		return control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		change func(*control.AttachmentGrant)
	}{
		{"pane client", func(g *control.AttachmentGrant) { g.ClientID = "pane-17" }},
		{"repository", func(g *control.AttachmentGrant) { g.RepositoryID = "foreign-repository" }},
		{"other run", func(g *control.AttachmentGrant) { g.RunID = "run_other" }},
		{"unknown run", func(g *control.AttachmentGrant) { g.RunID = "run_unknown" }},
		{"token", func(g *control.AttachmentGrant) { g.Token = "forged-token" }},
		{"unknown attachment", func(g *control.AttachmentGrant) { g.AttachmentID = "unknown" }},
		{"generation forgery", func(g *control.AttachmentGrant) { g.Generation++ }},
	} {
		t.Run(c.name, func(t *testing.T) {
			forged := g
			c.change(&forged)
			err := f.transactFor(t, forged.RunID, true, func(tx *sql.Tx, gen int64) error {
				attachmentCode(t, control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, forged, gen), control.CodePermissionDenied)
				attachmentCode(t, control.RevokeAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, forged, gen), control.CodePermissionDenied)
				var revoked int
				if err := tx.QueryRowContext(t.Context(), `SELECT revoked FROM attachment_grants WHERE attachment_id=?`, g.AttachmentID).Scan(&revoked); err != nil {
					return err
				}
				if revoked != 0 {
					t.Fatal("forged request changed attachment authority before rollback")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	// A nil transaction makes accidental lookup-before-peer fail with another code.
	for _, run := range []string{"run_attachment", "run_unknown"} {
		grant, err := control.MintAttachmentGrant(t.Context(), nil, control.Peer{}, run, 0)
		attachmentCode(t, err, control.CodePermissionDenied)
		if grant != (control.AttachmentGrant{}) {
			t.Fatal("unauthenticated mint returned data")
		}
	}
	attachmentCode(t, control.ValidateAttachmentGrant(t.Context(), nil, control.Peer{}, g, 0), control.CodePermissionDenied)
	attachmentCode(t, control.RevokeAttachmentGrant(t.Context(), nil, control.Peer{}, g, 0), control.CodePermissionDenied)
}

func attachmentExecutionSnapshot(t *testing.T, f *attachmentAuthFixture) string {
	t.Helper()
	var snapshot strings.Builder
	err := f.j.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, query := range []string{
			"SELECT * FROM runs ORDER BY rowid", "SELECT * FROM attempts ORDER BY rowid",
			"SELECT * FROM journal ORDER BY rowid", "SELECT * FROM capability_tokens ORDER BY rowid",
			"SELECT * FROM run_assignments ORDER BY rowid", "SELECT * FROM operations ORDER BY rowid",
			"SELECT * FROM service_assignment_epochs ORDER BY rowid", "SELECT * FROM run_control_revisions ORDER BY rowid",
		} {
			rows, err := tx.QueryContext(t.Context(), query)
			if err != nil {
				return err
			}
			cols, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return err
			}
			for rows.Next() {
				values := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range values {
					ptrs[i] = &values[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					_ = rows.Close()
					return err
				}
				fmt.Fprintf(&snapshot, "%s:%#v\n", query, values)
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.String()
}

func TestAttachmentGrantMutationIsolation(t *testing.T) { // I06/I12/I18/I23 (v2 §§8/14): presentation does not change execution.
	attachmentStorageBoundary(t)
	f := newAttachmentFixture(t)
	before := attachmentExecutionSnapshot(t, f)
	entriesBefore, err := os.ReadDir(filepath.Join(f.j.StateDir(), "runs", "run_attachment"))
	if err != nil {
		t.Fatal(err)
	}
	g := f.mint(t)
	if err := f.transact(t, true, func(tx *sql.Tx, gen int64) error {
		if err := control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen); err != nil {
			return err
		}
		return control.RevokeAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	}); err != nil {
		t.Fatal(err)
	}
	if after := attachmentExecutionSnapshot(t, f); after != before {
		t.Fatalf("attachment changed execution ledger\nbefore %s\nafter %s", before, after)
	}
	entriesAfter, err := os.ReadDir(filepath.Join(f.j.StateDir(), "runs", "run_attachment"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entriesBefore, entriesAfter) {
		t.Fatal("attachment changed run files")
	}
	release, err := supervisor.AcquireOwner(filepath.Join(f.j.StateDir(), "runs", "run_attachment"))
	if err == nil {
		release()
		t.Fatal("attachment released actual execution owner")
	}
	if !errors.Is(err, supervisor.ErrOwnerHeld) {
		t.Fatal(err)
	}
	err = f.transact(t, false, func(tx *sql.Tx, _ int64) error {
		var revoked, revision int
		if err := tx.QueryRowContext(t.Context(), `SELECT revoked,revocation_revision FROM attachment_grants WHERE attachment_id=?`, g.AttachmentID).Scan(&revoked, &revision); err != nil {
			return err
		}
		if revoked != 1 || revision != 1 {
			t.Fatalf("revocation=%d revision=%d", revoked, revision)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Runtime ledger/file observations above detect execution effects. This source
// boundary also rejects even an unsuccessful attempt to enter execution APIs.
func attachmentStorageBoundary(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source location unavailable")
	}
	source, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(file), "attachment_auth.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range source.Imports {
		if strings.Contains(imp.Path.Value, "/internal/service") || strings.Contains(imp.Path.Value, "/internal/supervisor") {
			t.Fatal("storage primitive imports execution owner")
		}
	}
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		}
		switch name {
		case "RecoverHandler", "AssignHandler", "MintToken", "StopHandler", "AcquireInstanceLease", "AcquireServiceOwner", "AcquireOwner", "Spawn", "Execute", "ExecuteLong":
			t.Errorf("storage primitive called execution API %s", name)
		}
		return true
	})
}

func TestAttachmentGrantRevocationGeneration(t *testing.T) { // I03/I06/I17 (v2 §§8/14): revocation and restart invalidate view authority only.
	f := newAttachmentFixture(t)
	before := attachmentExecutionSnapshot(t, f)
	g := f.mint(t)
	if err := f.transact(t, true, func(tx *sql.Tx, gen int64) error {
		return control.RevokeAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	}); err != nil {
		t.Fatal(err)
	}
	err := f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		return control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	})
	attachmentCode(t, err, control.CodePermissionDenied)
	old := f.mint(t)
	if err := f.owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.lease.Close(); err != nil {
		t.Fatal(err)
	}
	f.openOwner(t)
	if f.lease.Generation() <= old.Generation {
		t.Fatal("fixture failed real generation rollover")
	}
	err = f.transact(t, true, func(tx *sql.Tx, gen int64) error {
		forged := old
		forged.Token = "forged-old-token"
		attachmentCode(t, control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, forged, gen), control.CodePermissionDenied)
		attachmentCode(t, control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen), control.CodePermissionDenied)
		attachmentCode(t, control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, old, gen), control.CodeRevisionConflict)
		return control.RevokeAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, old, gen)
	})
	attachmentCode(t, err, control.CodeRevisionConflict)
	fresh := f.mint(t)
	if fresh.Generation != f.lease.Generation() || fresh.Token == old.Token {
		t.Fatal("fresh scope did not use current identity")
	}
	rollback := errors.New("synthetic transaction rollback")
	var uncommitted control.AttachmentGrant
	err = f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		var err error
		uncommitted, err = control.MintAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, "run_attachment", gen)
		if err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	err = f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		var count int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM attachment_grants WHERE attachment_id=? OR token_sha256=?`, uncommitted.AttachmentID, attachmentTokenHash(uncommitted.Token)).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("rollback persisted grant/hash")
		}
		return control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, uncommitted, gen)
	})
	attachmentCode(t, err, control.CodePermissionDenied)
	// A later failed write cannot return a usable raw token.
	err = f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		attachmentExec(t, tx, `CREATE TEMP TRIGGER reject_attachment BEFORE INSERT ON attachment_grants BEGIN SELECT RAISE(ABORT,'private database diagnostic'); END`)
		failed, err := control.MintAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, "run_attachment", gen)
		if failed != (control.AttachmentGrant{}) {
			t.Fatal("failed mint returned raw grant")
		}
		return err
	})
	attachmentCode(t, err, control.CodePersistenceUnavailable)
	if after := attachmentExecutionSnapshot(t, f); after != before {
		t.Fatal("revocation/restart/rollback rotated attempt authority or changed execution")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(filepath.Join(f.j.StateDir(), journal.DBName+suffix)) // #nosec G304 -- isolated fixture database and fixed sidecar suffixes.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{g.Token, old.Token, fresh.Token, uncommitted.Token} {
			if token == "" || bytes.Contains(data, []byte(token)) {
				t.Fatal("raw attachment token reached durable database storage")
			}
		}
	}
}

func attachmentTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestAttachmentGrantUnavailableScope(t *testing.T) {
	f := newAttachmentFixture(t)
	f.prepareOwner(t, "run_missing")
	for _, c := range []struct{ name, scope string }{
		{"relative", "relative/repository"}, {"unclean", f.repo + string(filepath.Separator) + ".."}, {"empty", ""}, {"control character", f.repo + "\x00"}, {"invalid UTF8", f.repo + "\xff"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := f.transact(t, false, func(tx *sql.Tx, gen int64) error {
				attachmentExec(t, tx, `UPDATE runs SET source_repo=? WHERE run_id='run_attachment'`, c.scope)
				g, err := control.MintAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, "run_attachment", gen)
				if g != (control.AttachmentGrant{}) {
					t.Fatal("unqualified scope returned grant")
				}
				return err
			})
			attachmentCode(t, err, control.CodeOwnershipUnresolved)
		})
	}
	g := f.mint(t)
	err := f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		attachmentExec(t, tx, `UPDATE runs SET source_repo=? WHERE run_id='run_attachment'`, filepath.Join(t.TempDir(), "changed"))
		return control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	})
	attachmentCode(t, err, control.CodePermissionDenied)
	err = f.transactFor(t, "run_missing", false, func(tx *sql.Tx, gen int64) error {
		got, err := control.MintAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, "run_missing", gen)
		if got != (control.AttachmentGrant{}) {
			t.Fatal("unknown run returned grant")
		}
		return err
	})
	attachmentCode(t, err, control.CodePermissionDenied)
}

func TestAttachmentGrantAtomicFailures(t *testing.T) {
	f := newAttachmentFixture(t)
	g := f.mint(t)
	rollback := errors.New("rollback rejected revoke")
	err := f.transact(t, true, func(tx *sql.Tx, gen int64) error {
		attachmentExec(t, tx, `CREATE TEMP TRIGGER reject_revoke BEFORE UPDATE ON attachment_grants BEGIN SELECT RAISE(ABORT,'private revoke diagnostic'); END`)
		attachmentCode(t, control.RevokeAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen), control.CodePersistenceUnavailable)
		var revoked, revision int
		if err := tx.QueryRowContext(t.Context(), `SELECT revoked,revocation_revision FROM attachment_grants WHERE attachment_id=?`, g.AttachmentID).Scan(&revoked, &revision); err != nil {
			return err
		}
		if revoked != 0 || revision != 0 {
			t.Fatal("failed revoke changed authority")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	err = f.transact(t, true, func(tx *sql.Tx, gen int64) error {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		failed, err := control.MintAttachmentGrant(ctx, tx, control.Peer{SameUser: true}, "run_attachment", gen)
		attachmentCode(t, err, control.CodePersistenceUnavailable)
		if failed != (control.AttachmentGrant{}) {
			t.Fatal("cancelled mint returned token")
		}
		attachmentCode(t, control.ValidateAttachmentGrant(ctx, tx, control.Peer{SameUser: true}, g, gen), control.CodePersistenceUnavailable)
		attachmentCode(t, control.RevokeAttachmentGrant(ctx, tx, control.Peer{SameUser: true}, g, gen), control.CodePersistenceUnavailable)
		var count int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM attachment_grants WHERE revoked=0 AND revocation_revision=0`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("cancelled operations left %d live grants; want original only", count)
		}
		return control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.transact(t, true, func(tx *sql.Tx, gen int64) error {
		if err := control.RevokeAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	err = f.transact(t, false, func(tx *sql.Tx, gen int64) error {
		return control.ValidateAttachmentGrant(t.Context(), tx, control.Peer{SameUser: true}, g, gen)
	})
	if err != nil {
		t.Fatalf("rolled-back revoke invalidated grant: %v", err)
	}
}

func TestAttachmentGrantStorageFailure(t *testing.T) {
	t.Parallel()
	peer := control.Peer{SameUser: true}
	g, err := control.MintAttachmentGrant(t.Context(), nil, peer, "run_attachment", 1)
	attachmentCode(t, err, control.CodePersistenceUnavailable)
	if g != (control.AttachmentGrant{}) {
		t.Fatal("missing transaction returned grant")
	}
	attachmentCode(t, control.ValidateAttachmentGrant(t.Context(), nil, peer, control.AttachmentGrant{}, 1), control.CodePersistenceUnavailable)
}
