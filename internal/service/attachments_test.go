package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

type projectionConn struct {
	requests    chan []byte
	replies     chan []byte
	closed      chan struct{}
	once        sync.Once
	peer        control.Peer
	beforeWrite func()
}

func newProjectionConn() *projectionConn {
	return &projectionConn{requests: make(chan []byte), replies: make(chan []byte, 8), closed: make(chan struct{}), peer: control.Peer{SameUser: true}}
}
func (c *projectionConn) Peer() control.Peer { return c.peer }
func (c *projectionConn) Close() error       { c.once.Do(func() { close(c.closed) }); return nil }
func (c *projectionConn) Receive(ctx context.Context) ([]byte, error) {
	select {
	case b := <-c.requests:
		return b, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	}
}
func (c *projectionConn) Respond(b []byte) error {
	if c.beforeWrite != nil {
		c.beforeWrite()
	}
	select {
	case c.replies <- bytes.Clone(b):
		return nil
	case <-c.closed:
		return io.EOF
	}
}
func (c *projectionConn) Request(ctx context.Context, b []byte) ([]byte, error) {
	select {
	case c.requests <- b:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	}
	select {
	case out := <-c.replies:
		return out, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	}
}
func attachmentWire(t *testing.T, r *runtime, c *projectionConn) control.AttachmentClient {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	l := &singleStatusListener{conn: c, closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- control.Serve(ctx, l, r.server, nil) }()
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("attachment Serve did not join")
		}
	})
	return control.NewAttachmentClient(c)
}
func seedProjectionRun(t *testing.T, r *runtime, id string) {
	t.Helper()
	seedRunState(t, r, id, "executing")
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE runs SET source_repo=?,adapter_id='builtin/fake' WHERE run_id=?`, filepath.Join(t.TempDir(), "synthetic-repo"), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func projectionSQL(t *testing.T, r *runtime, q string, args ...any) {
	t.Helper()
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}
func attachmentRows(t *testing.T, r *runtime) map[string][][]any {
	t.Helper()
	out := map[string][][]any{}
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, source := range []struct{ table, query string }{
			{"attachment_grants", "SELECT * FROM attachment_grants ORDER BY rowid"},
			{"attachment_projections", "SELECT * FROM attachment_projections ORDER BY rowid"},
			{"operations", "SELECT * FROM operations ORDER BY rowid"},
			{"runs", "SELECT * FROM runs ORDER BY rowid"},
			{"attempts", "SELECT * FROM attempts ORDER BY rowid"},
			{"run_assignments", "SELECT * FROM run_assignments ORDER BY rowid"},
			{"run_control_revisions", "SELECT * FROM run_control_revisions ORDER BY rowid"},
			{"capability_tokens", "SELECT * FROM capability_tokens ORDER BY rowid"},
			{"service_assignment_epochs", "SELECT * FROM service_assignment_epochs ORDER BY rowid"},
			{"service_admission_bindings", "SELECT * FROM service_admission_bindings ORDER BY rowid"},
			{"service_admission_consumptions", "SELECT * FROM service_admission_consumptions ORDER BY rowid"},
			{"service_pending_dispatch", "SELECT * FROM service_pending_dispatch ORDER BY rowid"},
			{"service_dispatch_claims", "SELECT * FROM service_dispatch_claims ORDER BY rowid"},
			{"prepared_runs", "SELECT * FROM prepared_runs ORDER BY rowid"},
			{"dispatch_outbox", "SELECT * FROM dispatch_outbox ORDER BY rowid"},
			{"stop_outbox", "SELECT * FROM stop_outbox ORDER BY rowid"},
			{"stop_results", "SELECT * FROM stop_results ORDER BY rowid"},
			{"journal", "SELECT * FROM journal ORDER BY rowid"},
		} {
			rows, err := tx.QueryContext(t.Context(), source.query)
			if err != nil {
				return err
			}
			columns, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return err
			}
			for rows.Next() {
				values := make([]any, len(columns))
				pointers := make([]any, len(columns))
				for i := range values {
					pointers[i] = &values[i]
				}
				if err := rows.Scan(pointers...); err != nil {
					_ = rows.Close()
					return err
				}
				for i, v := range values {
					if raw, ok := v.([]byte); ok {
						values[i] = bytes.Clone(raw)
					}
				}
				out[source.table] = append(out[source.table], values)
			}
			if err := errors.Join(rows.Err(), rows.Close()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestAttachmentOpenReadCloseComposition(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_projection")
	o := ownRun(t, r, "run_projection")
	registerActor(t, r, o)
	before := snapshotAttachmentEffects(t, r)
	c := newProjectionConn()
	client := attachmentWire(t, r, c)
	opened, err := client.Open(t.Context(), "run_projection")
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Validate(); err != nil {
		t.Fatal(err)
	}
	p, err := client.Read(t.Context(), opened.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, opened.Projection) {
		t.Fatalf("initial/read differ: %+v / %+v", opened.Projection, p)
	}
	again, err := client.Open(t.Context(), "run_projection")
	if err != nil {
		t.Fatal(err)
	}
	if again.Grant.AttachmentID == opened.Grant.AttachmentID || again.Projection.RunID != p.RunID {
		t.Fatal("reopen changed run or reused grant")
	}
	if err := client.Close(t.Context(), opened.Grant); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Read(t.Context(), opened.Grant); !errors.Is(err, &control.Error{Code: control.CodePermissionDenied}) {
		t.Fatalf("revoked read: %v", err)
	}
	if _, err := client.Read(t.Context(), again.Grant); err != nil {
		t.Fatal(err)
	}
	if after := snapshotAttachmentEffects(t, r); !reflect.DeepEqual(before, after) {
		t.Fatal("attachment changed execution/operations/owner files")
	}
}
func rawAttachment(t *testing.T, c *projectionConn, method, run string, g control.AttachmentGrant) []byte {
	t.Helper()
	var params any = map[string]any{"grant": g}
	var gen *int64
	token := ""
	if method == "attachment.open" {
		params = map[string]any{"run_id": run}
	} else {
		gen = &g.Generation
		token = g.Token
	}
	p, _ := json.Marshal(params)
	frame, err := control.Encode(control.Intent{OperationID: "op_same", Method: method, Object: run, Generation: gen, CapabilityToken: token, Params: p})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Request(t.Context(), frame)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestAttachmentPublicRefusalDoesNotRevealOwner(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_owned")
	seedProjectionRun(t, r, "run_unowned")
	registerActor(t, r, ownRun(t, r, "run_owned"))
	c := newProjectionConn()
	client := attachmentWire(t, r, c)
	opened, err := client.Open(t.Context(), "run_owned")
	if err != nil {
		t.Fatal(err)
	}
	g := opened.Grant
	g.Token = "synthetic-forged"

	before := snapshotAttachmentEffects(t, r)
	rows := attachmentRows(t, r)
	for _, method := range []string{"attachment.read", "attachment.close"} {
		var want []byte
		for _, run := range []string{"run_owned", "run_unknown", "run_unowned"} {
			g.RunID = run
			got := rawAttachment(t, c, method, run, g)
			if want == nil {
				want = got
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s discloses target %s: %s != %s", method, run, got, want)
			}
			var result control.Result
			if err := json.Unmarshal(got[4:], &result); err != nil {
				t.Fatal(err)
			}
			if result.Error == nil || result.Error.Code != control.CodePermissionDenied || result.Error.Message != "attachment unavailable" || result.Revision != 0 || (len(result.Body) != 0 && string(result.Body) != "null") {
				t.Fatalf("unsafe refusal: %+v", result)
			}
		}
	}
	var want []byte
	for _, run := range []string{"run_unknown", "run_unowned"} {
		got := rawAttachment(t, c, "attachment.open", run, g)
		if want == nil {
			want = got
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("open discloses target: %s != %s", want, got)
		}
	}
	if !reflect.DeepEqual(before, snapshotAttachmentEffects(t, r)) || !reflect.DeepEqual(rows, attachmentRows(t, r)) {
		t.Fatal("refused request changed ledger")
	}
}

func TestProjectionStableRevision(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_stable")
	registerActor(t, r, ownRun(t, r, "run_stable"))
	client := attachmentWire(t, r, newProjectionConn())
	first, err := client.Open(t.Context(), "run_stable")
	if err != nil {
		t.Fatal(err)
	}
	same, err := client.Read(t.Context(), first.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if same.ProjectionRevision != first.Projection.ProjectionRevision {
		t.Fatal("identical projection advanced revision")
	}
	projectionSQL(t, r, `UPDATE runs SET state='verifying' WHERE run_id='run_stable'`)
	changed, err := client.Read(t.Context(), first.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ProjectionRevision <= same.ProjectionRevision || changed.RunRevision <= same.RunRevision || changed.RunState != "verifying" {
		t.Fatalf("selected change did not advance: %+v -> %+v", same, changed)
	}
	// Assignment is deliberately not disclosed, but is still a complete revision
	// dependency. A run revision alone is selected content and must advance it.
	projectionSQL(t, r, `INSERT INTO operations VALUES ('op_assign','assign','run_stable','digest','done',1,'{}','now','now')`)
	projectionSQL(t, r, `INSERT INTO run_assignments(run_id,owner,assigned_at,operation_id) VALUES('run_stable','private-owner','now','op_assign')`)
	assigned, err := client.Read(t.Context(), first.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.ProjectionRevision <= changed.ProjectionRevision || assigned.RunRevision <= changed.RunRevision {
		t.Fatal("assignment omitted from complete revision")
	}
}

func TestAttachmentInitialProjectionFailureRollsBack(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_rollback")
	registerActor(t, r, ownRun(t, r, "run_rollback"))
	client := attachmentWire(t, r, newProjectionConn())
	projectionSQL(t, r, `CREATE TRIGGER fail_initial_projection BEFORE INSERT ON attachment_projections BEGIN SELECT RAISE(ABORT,'synthetic projection failure'); END`)
	before := attachmentRows(t, r)
	if got, err := client.Open(t.Context(), "run_rollback"); err == nil || got.Grant.Token != "" || got.Projection.RunID != "" {
		t.Fatalf("failed projection returned authority/data: %+v %v", got, err)
	}
	if !reflect.DeepEqual(before, attachmentRows(t, r)) {
		t.Fatal("failed initial projection persisted grant/cache")
	}
	projectionSQL(t, r, `DROP TRIGGER fail_initial_projection`)
	if _, err := client.Open(t.Context(), "run_rollback"); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionSanitization(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_safe")
	registerActor(t, r, ownRun(t, r, "run_safe"))
	const secret = "synthetic-secret-prompt-transcript-token"
	projectionSQL(t, r, `UPDATE runs SET adapter_id=?,reason=?,task_sha256=? WHERE run_id='run_safe'`, secret, "\x1b]0;"+secret+"\a", secret)
	projectionSQL(t, r, `INSERT INTO attempts(attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path,native_session_id) VALUES('att_safe','run_safe',?,1,'succeeded_native',?,?,?)`, "/private/"+secret, secret, "/private/"+secret, secret)
	projectionSQL(t, r, `INSERT INTO journal(event_id,schema_version,run_id,task_id,attempt_id,producer_id,producer_sequence,run_sequence,generation,observed_at,type,payload) VALUES('evt_safe',1,'run_safe','task_safe','att_safe','p_safe',1,1,1,'now','native.completed',?)`, `{"verified":true,"nested":{"secret":"`+secret+`","prompt":"private","transcript":"private","label":"\u001b]0;private"}}`)
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_safe")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(opened.Projection)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("/private/")) || bytes.Contains(raw, []byte(`\u001b`)) || opened.Projection.ReviewState == "verified" || opened.Projection.TaskID != nil || opened.Projection.Harness != nil {
		t.Fatalf("unsafe projection %s", raw)
	}
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		var content string
		if err := tx.QueryRowContext(t.Context(), `SELECT content FROM attachment_projections WHERE attachment_id=?`, opened.Grant.AttachmentID).Scan(&content); err != nil {
			return err
		}
		if bytes.Contains([]byte(content), []byte(secret)) {
			t.Fatal("raw secret persisted in cache")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachmentValidGrantLosesOwnerWithoutDisclosure(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_lost")
	a := registerActor(t, r, ownRun(t, r, "run_lost"))
	c := newProjectionConn()
	client := attachmentWire(t, r, c)
	opened, err := client.Open(t.Context(), "run_lost")
	if err != nil {
		t.Fatal(err)
	}
	unregister(r, a)
	before := attachmentRows(t, r)
	for _, method := range []string{"attachment.read", "attachment.close"} {
		valid := rawAttachment(t, c, method, "run_lost", opened.Grant)
		forged := opened.Grant
		forged.Token = "synthetic-forged"
		unknown := forged
		unknown.RunID = "run_missing"
		for _, g := range []control.AttachmentGrant{forged, unknown} {
			if other := rawAttachment(t, c, method, g.RunID, g); !bytes.Equal(valid, other) {
				t.Fatalf("unowned valid grant reveals existence: %s != %s", valid, other)
			}
		}
	}
	if !reflect.DeepEqual(before, attachmentRows(t, r)) {
		t.Fatal("unowned grant request changed stored rows")
	}
}

func TestAttachmentPublicOwnerBoundaryRefusals(t *testing.T) {
	for _, state := range []string{"ambiguous", "overflow", "closing"} {
		t.Run(state, func(t *testing.T) {
			r := testRuntime(t)
			seedProjectionRun(t, r, "run_boundary")
			owner := ownRun(t, r, "run_boundary")
			registerActor(t, r, owner)
			switch state {
			case "ambiguous":
				registerActor(t, r, owner)
			case "overflow":
				for range attachmentCandidateLimit {
					registerActor(t, r, nil)
				}
			case "closing":
				r.mu.Lock()
				r.closing = true
				r.mu.Unlock()
				t.Cleanup(func() { r.mu.Lock(); r.closing = false; r.mu.Unlock() })
			}
			c := newProjectionConn()
			attachmentWire(t, r, c)
			g := control.AttachmentGrant{AttachmentID: "synthetic-id", ClientID: "synthetic-client", RunID: "run_boundary", RepositoryID: "synthetic-repo", Token: "synthetic-token", Generation: 1}
			before := snapshotAttachmentEffects(t, r)
			rows := attachmentRows(t, r)
			for _, method := range []string{"attachment.read", "attachment.close"} {
				known := rawAttachment(t, c, method, g.RunID, g)
				g.RunID = "run_missing"
				unknown := rawAttachment(t, c, method, g.RunID, g)
				g.RunID = "run_boundary"
				if !bytes.Equal(known, unknown) {
					t.Fatalf("%s reveals %s owner: %s != %s", method, state, known, unknown)
				}
			}
			if !reflect.DeepEqual(before, snapshotAttachmentEffects(t, r)) || !reflect.DeepEqual(rows, attachmentRows(t, r)) {
				t.Fatal("scope refusal mutated state")
			}
		})
	}
}

func TestAttachmentRetainedOwnerQueueHandoff(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_handoff")
	owner := ownRun(t, r, "run_handoff")
	registerActor(t, r, owner)
	// Block at the actual authority gate after owner retention, before SQL.
	guard, err := r.authority.acquire(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer guard()
	writing := make(chan struct{})
	socketRelease := make(chan struct{})
	releaseSocket := sync.OnceFunc(func() { close(socketRelease) })
	defer releaseSocket()
	c := newProjectionConn()
	c.beforeWrite = func() { close(writing); <-socketRelease }
	client := attachmentWire(t, r, c)
	reply := make(chan error, 1)
	go func() { _, err := client.Open(t.Context(), "run_handoff"); reply <- err }()
	waitParked(t, "(*authorityGate).acquire")
	closed := make(chan error, 1)
	go func() { closed <- owner.Close() }()
	waitParked(t, "(*ServiceOwner).Close")
	if !runLockHeld(t, r, "run_handoff") {
		t.Fatal("owner lock released before queue acceptance")
	}
	select {
	case <-writing:
		t.Fatal("response released before authority gate")
	default:
	}
	guard()
	select {
	case <-writing:
	case <-time.After(5 * time.Second):
		t.Fatal("no response reached socket")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stalled socket retained owner after queue commit")
	}
	if runLockHeld(t, r, "run_handoff") {
		t.Fatal("owner lock remains after queue acceptance and Close")
	}
	// The request goroutine is joined after releasing its owned stalled socket.
	releaseSocket()
	select {
	case err := <-reply:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("public client did not finish")
	}
}

func protectedProjectionFixture(t *testing.T, r *runtime, run string) {
	t.Helper()
	seedProjectionRun(t, r, run)
	registerActor(t, r, ownRun(t, r, run))
	task := []byte("synthetic protected task\n")
	sum := sha256.Sum256(task)
	digest := hex.EncodeToString(sum[:])
	candidate := strings.Repeat("a", 40)
	dir := filepath.Join(r.journal.StateDir(), "runs", run)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), task, 0600); err != nil {
		t.Fatal(err)
	}
	pin := acceptance.NewPin("task_protected", digest).WithChecks(strings.Repeat("b", 64), "host", strings.Repeat("c", 64), map[string]string{"test": "5s"})
	object, err := acceptance.GitObjectOf(candidate)
	if err != nil {
		t.Fatal(err)
	}
	att := acceptance.Attest("ver_protected", "host", acceptance.Binding{Candidate: object, Contract: pin.Contract, CheckDefinition: pin.CheckDefinition, Evaluator: pin.EvaluatorDigest}, map[string]string{"test": "passed"}, nil)
	raw, err := json.Marshal(att)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"acceptance_pin": pin})
	if err != nil {
		t.Fatal(err)
	}
	projectionSQL(t, r, `UPDATE runs SET task_sha256=? WHERE run_id=?`, digest, run)
	projectionSQL(t, r, `INSERT INTO attempts(attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path) VALUES('att_protected',?,'task_protected',1,'succeeded_native','synthetic-hash','synthetic-path')`, run)
	projectionSQL(t, r, `INSERT INTO candidates VALUES('att_protected','base',?,'tree','hash','[]','[]',0)`, candidate)
	projectionSQL(t, r, `INSERT INTO journal(event_id,schema_version,run_id,producer_id,producer_sequence,run_sequence,generation,observed_at,type,payload) VALUES('evt_pin',1,?,'pin-producer',1,1,1,'now','admission.decided',?)`, run, string(payload))
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertVerification(t.Context(), tx, journal.VerificationRow{ID: "ver_protected", RunID: run, CandidateCommit: candidate, ConfigSHA256: strings.Repeat("b", 64), Result: "passed", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), EvaluatorName: "host", EvaluatorDigest: strings.Repeat("c", 64), Attestation: raw, AttestationDigest: v2contract.Digest(att), Checks: []journal.CheckRow{{Name: "test", Argv: []string{"synthetic"}, Status: "passed"}}})
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProjectionProtectedVerificationBinding(t *testing.T) {
	r := testRuntime(t)
	protectedProjectionFixture(t, r, "run_protected")
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_protected")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Projection.ReviewState != "verified" {
		t.Fatalf("genuine ADR0019 binding not verified: %+v", opened.Projection)
	}
	path := filepath.Join(r.journal.StateDir(), "runs", "run_protected", "task.md")
	if err := os.WriteFile(path, []byte("changed protected contract\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := client.Read(t.Context(), opened.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ReviewState == "verified" {
		t.Fatal("changed actual task artefact retained verified")
	}
}

func TestProjectionCanonicalStopResult(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_stopview")
	registerActor(t, r, ownRun(t, r, "run_stopview"))
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_stopview")
	if err != nil {
		t.Fatal(err)
	}
	projectionSQL(t, r, `INSERT INTO attempts(attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,worker_pid,worker_start_time,workspace_path) VALUES('att_stopview','run_stopview','task_stopview',1,'running','hash_stopview',123,'synthetic-start','synthetic-path')`)
	projectionSQL(t, r, `INSERT INTO operations VALUES('op_stopview','attachment.stop','run_stopview','digest','done',1,'{"operation_id":"op_stopview","accepted":true,"run_revision":1,"error_code":""}','now','now')`)
	projectionSQL(t, r, `INSERT INTO stop_outbox VALUES('op_stopview','run_stopview','att_stopview','hash_stopview',123,'synthetic-start',1,'pending')`)
	projectionSQL(t, r, `INSERT INTO stop_results VALUES('op_stopview','requested','',NULL,1)`)
	requested, err := client.Read(t.Context(), opened.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if requested.Stop == nil || requested.Stop.State != "requested" || requested.Stop.OperationID != "op_stopview" {
		t.Fatalf("missing canonical stop projection: %+v", requested)
	}
	projectionSQL(t, r, `UPDATE stop_results SET state='unconfirmed',reason='stop_unconfirmed',result_revision=2 WHERE operation_id='op_stopview'`)
	unconfirmed, err := client.Read(t.Context(), opened.Grant)
	if err != nil {
		t.Fatal(err)
	}
	if unconfirmed.Stop == nil || unconfirmed.Stop.State != "unconfirmed" || unconfirmed.Attention != "stop_unconfirmed" || unconfirmed.ProjectionRevision <= requested.ProjectionRevision {
		t.Fatalf("unconfirmed result lost: %+v", unconfirmed)
	}
}

func TestProjectionCompleteRevision(t *testing.T) {
	for _, change := range []struct{ name, query string }{
		{"candidate", `UPDATE candidates SET tree_id='changed-tree' WHERE attempt_id='att_protected'`},
		{"verification", `UPDATE verifications SET result='failed' WHERE verification_id='ver_protected'`},
		{"check result", `UPDATE check_results SET status='failed' WHERE verification_id='ver_protected'`},
		{"apply", `INSERT INTO applies VALUES('apply_projection','run_complete','synthetic-repo','synthetic-branch','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','applied',NULL,'now')`},
		{"assignment", `INSERT INTO operations VALUES('op_complete','assign','run_complete','digest','done',1,'{}','now','now')`},
	} {
		t.Run(change.name, func(t *testing.T) {
			r := testRuntime(t)
			protectedProjectionFixture(t, r, "run_complete")
			client := attachmentWire(t, r, newProjectionConn())
			opened, err := client.Open(t.Context(), "run_complete")
			if err != nil {
				t.Fatal(err)
			}
			projectionSQL(t, r, change.query)
			if change.name == "assignment" {
				projectionSQL(t, r, `INSERT INTO run_assignments VALUES('run_complete','synthetic-owner','now','op_complete')`)
			}
			p, err := client.Read(t.Context(), opened.Grant)
			if err != nil {
				t.Fatal(err)
			}
			if p.RunRevision <= opened.Projection.RunRevision || p.ProjectionRevision <= opened.Projection.ProjectionRevision {
				t.Fatalf("%s omitted from complete selected content revision: %+v -> %+v", change.name, opened.Projection, p)
			}
			if change.name == "apply" && p.ReviewState != "applied" {
				t.Fatalf("canonical apply did not project applied: %+v", p)
			}
			if change.name == "verification" && p.ReviewState == "verified" {
				t.Fatal("failed verification remained verified")
			}
		})
	}
}

func TestProjectionSnapshotAuthorityRace(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_raceview")
	registerActor(t, r, ownRun(t, r, "run_raceview"))
	c := newProjectionConn()
	client := attachmentWire(t, r, c)
	opened, err := client.Open(t.Context(), "run_raceview")
	if err != nil {
		t.Fatal(err)
	}
	guard, err := r.authority.acquire(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer guard()
	reply := make(chan struct {
		p   control.Projection
		err error
	}, 1)
	go func() {
		p, err := client.Read(t.Context(), opened.Grant)
		reply <- struct {
			p   control.Projection
			err error
		}{p, err}
	}()
	waitParked(t, "(*authorityGate).acquire")
	// Competing authority and data changes commit before this request obtains its
	// gate. Reading earlier authority and later data would disclose this label.
	projectionSQL(t, r, `UPDATE attachment_grants SET revoked=1,revocation_revision=revocation_revision+1 WHERE attachment_id=?`, opened.Grant.AttachmentID)
	projectionSQL(t, r, `UPDATE runs SET state='completed' WHERE run_id='run_raceview'`)
	guard()
	select {
	case got := <-reply:
		if !errors.Is(got.err, &control.Error{Code: control.CodePermissionDenied}) || got.p.RunID != "" {
			t.Fatalf("mixed authority/data snapshot: %+v %v", got.p, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("competing read did not finish")
	}
}

func TestAttachmentReadAndRevokeShareGate(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_gateview")
	registerActor(t, r, ownRun(t, r, "run_gateview"))
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_gateview")
	if err != nil {
		t.Fatal(err)
	}
	// A held read guard blocks the actual public Close before its transaction.
	readGuard, err := r.authority.acquire(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer readGuard()
	done := make(chan error, 1)
	go func() { done <- client.Close(t.Context(), opened.Grant) }()
	waitParked(t, "(*authorityGate).acquire")
	var revoked int
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT revoked FROM attachment_grants WHERE attachment_id=?`, opened.Grant.AttachmentID).Scan(&revoked)
	}); err != nil {
		t.Fatal(err)
	}
	if revoked != 0 {
		t.Fatal("revoke bypassed retained read authority")
	}
	readGuard()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revoke did not finish after read handoff")
	}
	if _, err := client.Read(t.Context(), opened.Grant); !errors.Is(err, &control.Error{Code: control.CodePermissionDenied}) {
		t.Fatalf("read after committed revoke: %v", err)
	}
}

func TestAttachmentAuthorizedStaleGenerationOnly(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_epoch")
	registerActor(t, r, ownRun(t, r, "run_epoch"))
	c := newProjectionConn()
	client := attachmentWire(t, r, c)
	opened, err := client.Open(t.Context(), "run_epoch")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Grant.Generation <= 1 {
		t.Fatal("fixture requires actual post-adoption generation")
	}
	old := opened.Grant
	old.Generation--
	for _, method := range []string{"attachment.read", "attachment.close"} {
		forged := old
		forged.Token = "synthetic-forged"
		got := rawAttachment(t, c, method, forged.RunID, forged)
		var denied control.Result
		if err := json.Unmarshal(got[4:], &denied); err != nil {
			t.Fatal(err)
		}
		if denied.Error == nil || denied.Error.Code != control.CodePermissionDenied {
			t.Fatalf("forged stale grant classified generation: %s", got)
		}
	}
	projectionSQL(t, r, `UPDATE attachment_grants SET generation=? WHERE attachment_id=?`, old.Generation, old.AttachmentID)
	for _, method := range []string{"attachment.read", "attachment.close"} {
		got := rawAttachment(t, c, method, old.RunID, old)
		var conflict control.Result
		if err := json.Unmarshal(got[4:], &conflict); err != nil {
			t.Fatal(err)
		}
		if conflict.Error == nil || conflict.Error.Code != control.CodeRevisionConflict || conflict.Revision != 0 || len(conflict.Body) != 0 {
			t.Fatalf("authorized old grant lost generation conflict: %s", got)
		}
	}
}

func TestProjectionProtectedArtifactChangesBeforeSnapshot(t *testing.T) {
	r := testRuntime(t)
	protectedProjectionFixture(t, r, "run_artefactrace")
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_artefactrace")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Projection.ReviewState != "verified" {
		t.Fatal("positive control lacks verified evidence")
	}
	guard, err := r.authority.acquire(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer guard()
	reply := make(chan control.Projection, 1)
	failed := make(chan error, 1)
	go func() {
		p, err := client.Read(t.Context(), opened.Grant)
		if err != nil {
			failed <- err
		} else {
			reply <- p
		}
	}()
	waitParked(t, "(*authorityGate).acquire")
	if err := os.WriteFile(filepath.Join(r.journal.StateDir(), "runs", "run_artefactrace", "task.md"), []byte("changed behind deterministic gate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	guard()
	select {
	case p := <-reply:
		if p.ReviewState == "verified" {
			t.Fatal("changed actual task artefact released as verified")
		}
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("read did not complete")
	}
}

func TestAttachmentRetainedOwnerFullQueueDiscard(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_queuefull")
	owner := ownRun(t, r, "run_queuefull")
	registerActor(t, r, owner)
	writing := make(chan struct{})
	socketRelease := make(chan struct{})
	releaseSocket := sync.OnceFunc(func() { close(socketRelease) })
	defer releaseSocket()
	c := newProjectionConn()
	c.beforeWrite = func() {
		select {
		case <-writing:
		default:
			close(writing)
		}
		<-socketRelease
	}
	attachmentWire(t, r, c)
	frame, err := control.Encode(control.Intent{OperationID: "op_queue", Method: "attachment.open", Object: "run_queuefull", Params: json.RawMessage(`{"run_id":"run_queuefull"}`)})
	if err != nil {
		t.Fatal(err)
	}
	send := func() {
		select {
		case c.requests <- frame:
		case <-time.After(5 * time.Second):
			t.Fatal("request not consumed")
		}
	}
	send()
	select {
	case <-writing:
	case <-time.After(5 * time.Second):
		t.Fatal("socket not reached")
	}
	send()
	send()
	// The first response is in flight and the second is accepted into the sole
	// pending slot. The third retains scope while the actual queue is full.
	waitParked(t, "control.offer(")
	deadline := time.Now().Add(time.Second)
	for {
		r.authority.mu.Lock()
		readers := r.authority.readers
		r.authority.mu.Unlock()
		if readers == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("third handler did not retain its scope while queue was full")
		}
		goruntime.Gosched()
	}
	closed := make(chan error, 1)
	go func() { closed <- owner.Close() }()
	waitParked(t, "(*ServiceOwner).Close")
	if !runLockHeld(t, r, "run_queuefull") {
		t.Fatal("full queue released owner before discard")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("queue deadline failed to release retained owner")
	}
	if runLockHeld(t, r, "run_queuefull") {
		t.Fatal("discarded frame retained owner")
	}
	select {
	case <-c.replies:
		t.Fatal("stalled socket exposed queued response")
	default:
	}
	releaseSocket()
}

func TestAttachmentRootShutdownRetainsUntilDiscard(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_rootwait")
	owner := ownRun(t, r, "run_rootwait")
	if _, err := r.startEntry(t.Context(), owner, func(ctx context.Context, _ supervisor.Hooks) error { <-ctx.Done(); return nil }); err != nil {
		t.Fatal(err)
	}
	guard, err := r.authority.acquire(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer guard()
	c := newProjectionConn()
	client := attachmentWire(t, r, c)
	reply := make(chan error, 1)
	go func() { _, err := client.Open(t.Context(), "run_rootwait"); reply <- err }()
	waitParked(t, "(*authorityGate).acquire")
	closed := make(chan error, 1)
	go func() { closed <- r.shutdown(context.Background()) }()
	waitParked(t, "(*ServiceOwner).Close")
	if !runLockHeld(t, r, "run_rootwait") {
		t.Fatal("root shutdown released retained run ownership")
	}
	if lease, err := control.AcquireInstanceLease(r.journal.StateDir()); err == nil {
		_ = lease.Close()
		t.Fatal("root shutdown released retained instance")
	}
	guard()
	select {
	case err := <-reply:
		if !errors.Is(err, &control.Error{Code: control.CodePermissionDenied}) {
			t.Fatalf("closing request exposed response: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing request did not discard scope")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("root shutdown did not finish after discard")
	}
}

func TestProjectionRejectsIncompleteProtectedEvidence(t *testing.T) {
	for _, field := range []string{"contract", "check_definition", "evaluator", "candidate", "digest", "missing_binding"} {
		t.Run(field, func(t *testing.T) {
			r := testRuntime(t)
			protectedProjectionFixture(t, r, "run_incomplete")
			client := attachmentWire(t, r, newProjectionConn())
			opened, err := client.Open(t.Context(), "run_incomplete")
			if err != nil {
				t.Fatal(err)
			}
			if opened.Projection.ReviewState != "verified" {
				t.Fatal("positive binding control failed")
			}
			var raw string
			if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
				return tx.QueryRowContext(t.Context(), `SELECT attestation FROM verification_bindings WHERE verification_id='ver_protected'`).Scan(&raw)
			}); err != nil {
				t.Fatal(err)
			}
			var att v2contract.Verification
			if err := json.Unmarshal([]byte(raw), &att); err != nil {
				t.Fatal(err)
			}
			att.VerificationID = "ver_later"
			switch field {
			case "contract":
				att.AcceptanceContractDigest = ""
			case "check_definition":
				att.CheckDefinition = ""
			case "evaluator":
				att.CheckVersion = ""
			case "candidate":
				att.Candidate.Value = strings.Repeat("d", 40)
			}
			encoded, err := json.Marshal(att)
			if err != nil {
				t.Fatal(err)
			}
			digest := v2contract.Digest(att)
			if field == "digest" {
				digest = "sha256:" + strings.Repeat("0", 64)
			}
			projectionSQL(t, r, `INSERT INTO verifications VALUES('ver_later','run_incomplete',?,'cfg','passed','9999-01-01T00:00:00Z','9999-01-01T00:00:01Z')`, att.Candidate.Value)
			if field != "missing_binding" {
				projectionSQL(t, r, `INSERT INTO verification_bindings VALUES('ver_later','',?,?,NULL)`, string(encoded), digest)
			}
			p, err := client.Read(t.Context(), opened.Grant)
			if err != nil {
				t.Fatal(err)
			}
			if p.ReviewState == "verified" || p.ReviewState == "applied" {
				t.Fatalf("%s evidence released protected state: %+v", field, p)
			}
		})
	}
}

func TestAttachmentMalformedContractIndependentOfTarget(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_malformed")
	registerActor(t, r, ownRun(t, r, "run_malformed"))
	c := newProjectionConn()
	attachmentWire(t, r, c)
	for _, method := range []string{"attachment.open", "attachment.read", "attachment.close"} {
		var want []byte
		for _, run := range []string{"run_malformed", "run_missing"} {
			params := json.RawMessage(`{"unknown_field":"synthetic"}`)
			frame, err := control.Encode(control.Intent{OperationID: "op_same", Method: method, Object: run, Params: params})
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Request(t.Context(), frame)
			if err != nil {
				t.Fatal(err)
			}
			if want == nil {
				want = got
			} else if !bytes.Equal(want, got) {
				t.Fatalf("malformed %s leaked target: %s != %s", method, want, got)
			}
			var result control.Result
			if err := json.Unmarshal(got[4:], &result); err != nil {
				t.Fatal(err)
			}
			if result.Error == nil || result.Error.Code != control.CodeInvalidContract || result.Revision != 0 || len(result.Body) != 0 {
				t.Fatalf("malformed request did not fail closed: %s", got)
			}
		}
	}
}

func TestAttachmentPublicPhysicalIdentityRefusals(t *testing.T) {
	for _, target := range []string{"root", "database"} {
		t.Run(target, func(t *testing.T) {
			r := testRuntime(t)
			seedProjectionRun(t, r, "run_identityview")
			registerActor(t, r, ownRun(t, r, "run_identityview"))
			c := newProjectionConn()
			client := attachmentWire(t, r, c)
			opened, err := client.Open(t.Context(), "run_identityview")
			if err != nil {
				t.Fatal(err)
			}
			// Close SQLite before replacing its physical identity on Windows. Keep
			// the original owner and lease, but use a valid replacement journal so
			// refusal cannot be explained by an empty file or a closed database.
			root := r.journal.StateDir()
			if err := r.journal.Close(); err != nil {
				t.Fatal(err)
			}
			path := root
			if target == "database" {
				path = filepath.Join(root, journal.DBName)
			}
			if err := os.Rename(path, path+".synthetic-moved"); err != nil {
				t.Fatal(err)
			}
			if target == "root" {
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			originalDB := path + ".synthetic-moved"
			if target == "root" {
				originalDB = filepath.Join(originalDB, journal.DBName)
			}
			data, err := os.ReadFile(originalDB) // #nosec G304 -- Closed original database in this isolated replacement fixture.
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, journal.DBName), data, 0600); err != nil { // #nosec G703 -- Fixed database name under the fixture-owned root.
				t.Fatal(err)
			}
			replacement, err := journal.Open(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			r.journal = replacement
			g := opened.Grant
			for _, method := range []string{"attachment.open", "attachment.read", "attachment.close"} {
				known := rawAttachment(t, c, method, "run_identityview", g)
				g.RunID = "run_missing"
				unknown := rawAttachment(t, c, method, "run_missing", g)
				g.RunID = "run_identityview"
				if !bytes.Equal(known, unknown) {
					t.Fatalf("%s replacement exposed target: %s != %s", target, known, unknown)
				}
				var result control.Result
				if err := json.Unmarshal(known[4:], &result); err != nil {
					t.Fatal(err)
				}
				if result.Error == nil || result.Error.Code != control.CodePermissionDenied || len(result.Body) != 0 {
					t.Fatalf("replaced physical %s disclosed response: %s", target, known)
				}
			}
		})
	}
}

func TestProjectionAppliedDoesNotInventVerification(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_applied")
	registerActor(t, r, ownRun(t, r, "run_applied"))
	client := attachmentWire(t, r, newProjectionConn())
	projectionSQL(t, r, `INSERT INTO attempts(attempt_id,run_id,task_id,attempt_number,state,launch_token_sha256,workspace_path) VALUES('att_applied','run_applied','task_applied',1,'succeeded_native','synthetic-hash','synthetic-path')`)
	projectionSQL(t, r, `INSERT INTO candidates VALUES('att_applied','base','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','tree','hash','[]','[]',0)`)
	projectionSQL(t, r, `INSERT INTO applies VALUES('apply_without_checks','run_applied','synthetic-repo','synthetic-branch','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','applied',NULL,'now')`)
	// Existing Apply permits explicit acceptance of an unverified candidate. Its
	// durable applied effect is distinct from claiming that checks verified it.
	opened, err := client.Open(t.Context(), "run_applied")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Projection.ReviewState != "applied" {
		t.Fatalf("canonical applied effect hidden: %+v", opened.Projection)
	}
}

func TestIndependentLatestVerificationOrder(t *testing.T) {
	r := testRuntime(t)
	protectedProjectionFixture(t, r, "run_order")
	projectionSQL(t, r, `INSERT INTO verifications VALUES('ver_later_finish','run_order','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','cfg','failed','1970-01-01T00:00:00Z','9999-01-01T00:00:00Z')`)
	canonical, err := supervisor.CheckAcceptance(t.Context(), r.journal, "run_order")
	if err != nil {
		t.Fatal(err)
	}
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_order")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("canonical acceptance=%+v projection review=%s", canonical, opened.Projection.ReviewState)
	if canonical.State != supervisor.AcceptanceRejected {
		t.Fatalf("canonical latest-verdict fixture is not rejected: %+v", canonical)
	}
	if opened.Projection.ReviewState != "candidate" {
		t.Fatalf("projection did not preserve canonical rejected latest verdict as candidate: %s", opened.Projection.ReviewState)
	}
}

func TestAttachmentRootReplacedWhileWaitingGate(t *testing.T) {
	r := testRuntime(t)
	seedProjectionRun(t, r, "run_gate_root")
	registerActor(t, r, ownRun(t, r, "run_gate_root"))
	client := attachmentWire(t, r, newProjectionConn())
	opened, err := client.Open(t.Context(), "run_gate_root")
	if err != nil {
		t.Fatal(err)
	}
	guard, err := r.authority.acquire(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer guard()
	type response struct {
		p   control.Projection
		err error
	}
	done := make(chan response, 1)
	go func() { p, err := client.Read(t.Context(), opened.Grant); done <- response{p, err} }()
	waitParked(t, "(*authorityGate).acquire")
	root := r.journal.StateDir()
	moved := root + "-original"
	renameErr := os.Rename(root, moved)
	if renameErr != nil {
		var linkErr *os.LinkError
		if goruntime.GOOS != "windows" || !errors.As(renameErr, &linkErr) ||
			(!errors.Is(linkErr.Err, syscall.Errno(5)) && !errors.Is(linkErr.Err, syscall.Errno(32)) && !errors.Is(linkErr.Err, syscall.Errno(33))) {
			t.Fatal(renameErr)
		}
		if err := r.checkAttachmentIdentity(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Logf("Windows refused live namespace mutation; verifying unchanged read: %v", renameErr)
		guard()
		select {
		case got := <-done:
			if got.err != nil || got.p.RunID != opened.Projection.RunID || got.p.RunRevision != opened.Projection.RunRevision {
				t.Fatalf("unchanged read: %+v, %v", got.p, got.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("unchanged request did not finish")
		}
		return
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
		if err := os.Rename(moved, root); err != nil {
			t.Error(err)
		}
	}()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	replacement, err := journal.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.journal.CheckStateIdentity(t.Context()); err == nil {
		t.Fatal("replacement did not break original physical identity")
	}
	before := attachmentRows(t, r)
	guard()
	select {
	case got := <-done:
		var ce *control.Error
		if !errors.As(got.err, &ce) || ce.Code != control.CodePermissionDenied || ce.Message != "attachment unavailable" || got.p.RunID != "" {
			t.Fatalf("replaced root released projection: run=%q err=%v", got.p.RunID, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
	if !reflect.DeepEqual(before, attachmentRows(t, r)) {
		t.Fatal("replaced root request changed attachment rows")
	}
}
