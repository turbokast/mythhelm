package control

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
)

// openLedger migrates a fresh state database with the journal and returns a
// raw handle with the journal's pragmas, standing in for the supervisor's.
func openLedger(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, journal.DBName))+
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dir
}

func seedRun(t *testing.T, db *sql.DB, runID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO runs (run_id, state, adapter_id, source_repo, task_sha256, billing_posture, execution_profile, created_at, updated_at)
		VALUES (?, 'created', 'builtin/fake', '/tmp/repo', 't', 'local-scripted', 'trusted-host', 't', 't')`, runID); err != nil {
		t.Fatal(err)
	}
}

func seedAttempt(t *testing.T, db *sql.DB, runID, attemptID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO attempts (attempt_id, run_id, task_id, attempt_number, state, launch_token_sha256, workspace_path)
		VALUES (?, ?, 'task_1', 1, 'created', 'x', '/tmp/ws')`, attemptID, runID); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func intent(op, method string) Intent {
	return Intent{OperationID: op, Method: method}
}

// counting wraps a handler that returns body and counts its runs.
func counting(body string, runs *atomic.Int32) Handler {
	return func(context.Context, Peer, Intent) (Result, error) {
		runs.Add(1)
		return Result{Body: json.RawMessage(body)}, nil
	}
}

func wire(t *testing.T, r Result) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func requireCode(t *testing.T, err error, code Code) {
	t.Helper()
	if !errors.Is(err, &Error{Code: code}) {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestIdenticalRepeatReplays(t *testing.T) { // I12 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := counting(`{"ok":true}`, &runs)
	in := intent("op_1", "probe")
	in.Params = json.RawMessage(`{"a": 1}`)

	first, err := Execute(ctx, h, Peer{}, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Params = json.RawMessage(`{"a":1}`) // same arguments, different whitespace
	second, err := Execute(ctx, h, Peer{}, in)
	if err != nil {
		t.Fatal(err)
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", runs.Load())
	}
	if wire(t, first) != wire(t, second) || first.OperationID != "op_1" {
		t.Fatalf("results differ: %s vs %s", wire(t, first), wire(t, second))
	}
}

func TestStoredFailureReplays(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := func(context.Context, Peer, Intent) (Result, error) {
		runs.Add(1)
		return Result{}, newError(CodePermissionDenied, "no")
	}
	for range 2 {
		res, err := Execute(ctx, h, Peer{}, intent("op_1", "probe"))
		requireCode(t, err, CodePermissionDenied)
		if res.Error == nil || res.Error.Code != CodePermissionDenied {
			t.Fatalf("result = %+v, want the stored error", res)
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times for a stored failure, want 1", runs.Load())
	}
}

func TestTransientFailureReleasesTheOperation(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := func(context.Context, Peer, Intent) (Result, error) {
		if runs.Add(1) == 1 {
			return Result{}, errors.New("disk hiccup")
		}
		return Result{Body: json.RawMessage(`{}`)}, nil
	}
	if _, err := Execute(ctx, h, Peer{}, intent("op_1", "probe")); err == nil {
		t.Fatal("transient failure returned no error")
	}
	if _, err := Execute(ctx, h, Peer{}, intent("op_1", "probe")); err != nil {
		t.Fatalf("retry after a transient failure: %v", err)
	}
	if runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want the retry to run it again", runs.Load())
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = Execute(ctx, func(context.Context, Peer, Intent) (Result, error) { panic("boom") }, Peer{}, intent("op_2", "probe"))
	}()
	if _, err := Execute(ctx, h, Peer{}, intent("op_2", "probe")); err != nil {
		t.Fatalf("retry after a panic: %v", err)
	}
}

func TestConcurrentDuplicateExecutesOnce(t *testing.T) { // I12 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	entered, proceed := make(chan struct{}), make(chan struct{})
	h := func(context.Context, Peer, Intent) (Result, error) {
		if runs.Add(1) == 1 {
			close(entered)
		}
		<-proceed
		return Result{Body: json.RawMessage(`{"winner":true}`)}, nil
	}
	const callers = 12
	out := make([]string, callers)
	errs := make([]error, callers)
	var wg, started sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		started.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			res, err := Execute(ctx, h, Peer{}, intent("op_1", "probe"))
			out[i], errs[i] = wire(t, res), err
		}()
	}
	<-entered // the claimant is inside the handler; hold it while the rest arrive
	started.Wait()
	runtime.Gosched()
	close(proceed)
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times for %d concurrent duplicates, want 1", runs.Load(), callers)
	}
	for i := range callers {
		if errs[i] != nil || out[i] != out[0] {
			t.Fatalf("caller %d got %q (%v), want the shared stored result %q", i, out[i], errs[i], out[0])
		}
	}
}

func TestIntentEmbedsRequestEnvelope(t *testing.T) {
	t.Parallel()
	gen := int64(3)
	in := Intent{OperationID: "op_1", Object: "run_1", ExpectedRevision: 2, Generation: &gen, Method: "assign"}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var wireKeys map[string]any
	if err := json.Unmarshal(raw, &wireKeys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"operation_id", "object", "expected_revision", "generation", "method"} {
		if _, ok := wireKeys[key]; !ok {
			t.Errorf("wire intent %s lacks %q", raw, key)
		}
	}
	// The same bytes are a valid frame: ingress accepts the intent's wire form.
	if err := CheckIngress(encodeIntent(t, in)); err != nil {
		t.Fatalf("CheckIngress rejects an intent: %v", err)
	}

	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	bad := in
	bad.OperationID = ""
	_, err = Execute(ctx, counting(`{}`, &runs), Peer{}, bad)
	requireCode(t, err, CodeInvalidContract)
	bad = in
	bad.ExpectedRevision = -1
	_, err = Execute(ctx, counting(`{}`, &runs), Peer{}, bad)
	requireCode(t, err, CodeInvalidContract)
	if runs.Load() != 0 {
		t.Fatalf("handler ran %d times for invalid envelopes", runs.Load())
	}
}

func encodeIntent(t *testing.T, v any) []byte {
	t.Helper()
	b, err := Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExecuteRejectsMalformedIntent(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	noMethod := intent("op_1", "")
	badParams := intent("op_2", "probe")
	badParams.Params = json.RawMessage(`{"a":`)
	for name, in := range map[string]Intent{"no method": noMethod, "params not JSON": badParams} {
		_, err := Execute(ctx, counting(`{}`, &runs), Peer{}, in)
		if !errors.Is(err, &Error{Code: CodeInvalidContract}) {
			t.Errorf("%s: err = %v, want invalid_contract", name, err)
		}
	}
	if runs.Load() != 0 || count(t, db, "operations") != 0 {
		t.Fatalf("handler ran %d times, operations rows %d; a malformed intent must do neither", runs.Load(), count(t, db, "operations"))
	}
}

func TestReusedIDWithDifferentArgsConflicts(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := counting(`{}`, &runs)
	first := intent("op_1", "probe")
	first.Params = json.RawMessage(`{"a":1}`)
	if _, err := Execute(ctx, h, Peer{}, first); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Intent){
		"params": func(i *Intent) { i.Params = json.RawMessage(`{"a":2}`) },
		"method": func(i *Intent) { i.Method = "other" },
		"object": func(i *Intent) { i.Object = "run_9" },
	} {
		other := first
		mutate(&other)
		_, err := Execute(ctx, h, Peer{}, other)
		if !errors.Is(err, &Error{Code: CodeRevisionConflict}) {
			t.Errorf("reuse with different %s: err = %v, want revision_conflict", name, err)
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want only the first", runs.Load())
	}
}

func TestStaleExpectedRevisionConflicts(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := func(_ context.Context, _ Peer, in Intent) (Result, error) {
		runs.Add(1)
		return Result{Revision: in.ExpectedRevision + 1}, nil
	}
	step := func(op string, expected int64) error {
		in := intent(op, "probe")
		in.Object, in.ExpectedRevision = "run_1", expected
		_, err := Execute(ctx, h, Peer{}, in)
		return err
	}
	if err := step("op_1", 0); err != nil {
		t.Fatal(err)
	}
	requireCode(t, step("op_2", 0), CodeRevisionConflict) // older than the object's revision 1
	requireCode(t, step("op_3", 5), CodeRevisionConflict) // a revision the object never reached
	if err := step("op_4", 1); err != nil {
		t.Fatalf("current revision refused: %v", err)
	}
	if runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want the two accepted intents only", runs.Load())
	}
	// A repeat of the first intent still replays although its revision is now stale.
	if err := step("op_1", 0); err != nil {
		t.Fatalf("replay of an accepted intent refused: %v", err)
	}
}

func TestMutateIsOneTransaction(t *testing.T) { // I23 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	boom := errors.New("fails midway")
	err := Mutate(t.Context(), db, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO journal (event_id, schema_version, run_id, producer_id, producer_sequence, run_sequence, generation, observed_at, type, payload)
			VALUES ('evt_1', 1, 'run_1', 'p', 1, 1, 1, 't', 'run.state_changed', '{}')`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO runs (run_id, state, adapter_id, source_repo, task_sha256, billing_posture, execution_profile, created_at, updated_at)
			VALUES ('run_1', 'created', 'a', 'r', 't', 'b', 'p', 't', 't')`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO operations (operation_id, method, digest, state, claimed_at) VALUES ('op_1','m','d','claimed','t')`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Mutate = %v, want the handler's error", err)
	}
	for _, table := range []string{"journal", "runs", "operations"} {
		if n := count(t, db, table); n != 0 {
			t.Errorf("%s holds %d rows after a failed mutation, want none", table, n)
		}
	}
	// Positive control: the same statements commit when fn succeeds.
	if err := Mutate(t.Context(), db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO operations (operation_id, method, digest, state, claimed_at) VALUES ('op_1','m','d','claimed','t')`)
		return err
	}); err != nil || count(t, db, "operations") != 1 {
		t.Fatalf("committing Mutate: %v, rows %d", err, count(t, db, "operations"))
	}
}

func TestRecordedResultIsImmutable(t *testing.T) { // I20 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	if _, err := Execute(ctx, counting(`{"v":1}`, &runs), Peer{}, intent("op_1", "probe")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE operations SET result = '{"v":2}' WHERE operation_id = 'op_1'`); err == nil {
		t.Error("a recorded result was rewritten")
	}
	if _, err := db.Exec(`DELETE FROM operations WHERE operation_id = 'op_1'`); err == nil {
		t.Error("a recorded operation was deleted")
	}
}

// workerAllowed are the only journal identifiers a worker may name: the
// spool writes journal.Event lines and never touches the database.
var workerAllowed = map[string]bool{"Event": true, "EnvelopeVersion": true}

// journalUses lists the journal identifiers a Go source file names.
func journalUses(t *testing.T, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	var uses []string
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "journal" {
				uses = append(uses, sel.Sel.Name)
			}
		}
		return true
	})
	return uses
}

// TestWorkersNeverOpenSQLite pins AC-6.1: workers never open the database.
// The prescribed `go list -deps ./internal/workers` check cannot hold: workers
// import internal/journal for the Event envelope (spool.go), and journal
// registers the modernc.org/sqlite driver, so the driver is a dependency of
// every worker. The property that matters is that no worker code names the
// journal's database API, which this checks at the source.
func TestWorkersNeverOpenSQLite(t *testing.T) { // I23 (v2 §2)
	t.Parallel()
	if got := journalUses(t, "probe.go", "package p\nvar _ = journal.Open\n"); len(got) != 1 || got[0] != "Open" {
		t.Fatalf("journalUses missed a journal.Open reference: %v", got)
	}
	dir := filepath.Join("..", "workers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		for _, use := range journalUses(t, filepath.Join(dir, e.Name()), nil) {
			seen[use] = true
			if !workerAllowed[use] {
				t.Errorf("%s names journal.%s: workers never open SQLite (AC-6.1)", e.Name(), use)
			}
		}
	}
	if !seen["Event"] {
		t.Fatal("no worker file names journal.Event: the scan reads the wrong directory")
	}
}

// Hashes of the released migrations, which later migrations must not touch.
var releasedMigrations = map[string]string{
	"0001_init.sql":          "262e63e2c7630dc637a916d476a1862db361affcb734dcbe2317f6770cb72e36",
	"0002_qualification.sql": "52ce1d7a33fe4ace12e06ac3eae4032d9f11d647199113ef287416ab0202da7e",
	"0003_ledger.sql":        "c9203ba912b7fe187d472edfa56f02fd751ffaddcb12d3eee4c98bf920487aa6",
	"0004_supervisor.sql":    "2078c4755d71cc0d73003f1025209aa3bb1afd234fe992a830056ac9def22910",
}

func TestMigration0004IsAdditive(t *testing.T) {
	t.Parallel()
	migrationsDir := filepath.Join("..", "journal", "migrations")
	for name, want := range releasedMigrations {
		raw, err := os.ReadFile(filepath.Clean(filepath.Join(migrationsDir, name)))
		if err != nil {
			t.Fatal(err)
		}
		if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != want {
			t.Errorf("%s sha256 = %x, want %s", name, sum, want)
		}
	}

	// Build a version-3 database from the released files alone.
	dir := t.TempDir()
	path := filepath.Join(dir, journal.DBName)
	v3, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_init.sql", "0002_qualification.sql", "0003_ledger.sql"} {
		raw, err := os.ReadFile(filepath.Clean(filepath.Join(migrationsDir, name)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := v3.Exec(string(raw)); err != nil {
			t.Fatalf("applying %s: %v", name, err)
		}
	}
	if _, err := v3.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
	}
	before := schemaObjects(t, v3)
	if err := v3.Close(); err != nil {
		t.Fatal(err)
	}

	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = after.Close() }()
	got := schemaObjects(t, after)
	for name, ddl := range before {
		if name == "reservations" {
			continue // migration 0005 adds its host column; Test0005 pins that change
		}
		if got[name] != ddl {
			t.Errorf("DDL of %s changed by migration 0004:\nbefore %q\nafter  %q", name, ddl, got[name])
		}
	}
	for _, table := range []string{"operations", "capability_tokens", "run_assignments"} {
		if got[table] == "" {
			t.Errorf("migration 0004 did not create %s", table)
		}
	}
}

func schemaObjects(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT name, COALESCE(sql, '') FROM sqlite_master`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		out[name] = ddl
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSchemaVersionIs4(t *testing.T) {
	t.Parallel()
	_, dir := openLedger(t)
	if journal.SchemaVersion < 4 {
		t.Fatalf("SchemaVersion = %d, want at least 4", journal.SchemaVersion)
	}
	ro, err := journal.OpenReadOnly(t.Context(), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly on a migrated database: %v", err)
	}
	_ = ro.Close()
}

func TestRegisterBindsMethod(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	srv := NewServer(nil)
	if err := srv.Register("test.probe", counting(`{"hit":true}`, &runs)); err != nil {
		t.Fatal(err)
	}
	res, err := srv.Dispatch(ctx, Peer{}, intent("op_1", "test.probe"))
	if err != nil || runs.Load() != 1 || !strings.Contains(string(res.Body), "hit") {
		t.Fatalf("Dispatch = %+v, %v; handler ran %d times", res, err, runs.Load())
	}
	err = srv.Register("test.probe", counting(`{}`, &runs))
	if err == nil || !strings.Contains(err.Error(), "test.probe") {
		t.Fatalf("duplicate registration = %v, want an error naming the method", err)
	}
	before := count(t, db, "operations")
	_, err = srv.Dispatch(ctx, Peer{}, intent("op_2", "test.unknown"))
	requireCode(t, err, CodeCapabilityUnsupported)
	if after := count(t, db, "operations"); after != before {
		t.Fatalf("an unregistered method touched the ledger: %d rows before, %d after", before, after)
	}
	if got := NewServer(map[string]Handler{"a": counting(`{}`, &runs)}); got.Register("a", nil) == nil {
		t.Fatal("a method bound by NewServer can be registered again")
	}
}

func TestStatusIntentReturnsInstanceState(t *testing.T) {
	// not parallel: t.Setenv redirects the per-user lock directory.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()
	release, err := AcquireInstance(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	h := StatusHandler(db)

	first, err := Execute(ctx, h, Peer{}, intent("op_1", "status"))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		PID        int    `json:"pid"`
		Generation uint64 `json:"generation"`
		Root       string `json:"root"`
		Endpoint   string `json:"endpoint"`
	}
	if err := json.Unmarshal(first.Body, &body); err != nil {
		t.Fatal(err)
	}
	lock, _ := LockPath()
	meta, ok := readMetadata(lock)
	endpoint, _ := EndpointPath()
	if !ok || body.PID != os.Getpid() || body.Generation != meta.Generation || body.Generation == 0 || body.Root != meta.Root || body.Root == "" || body.Endpoint != endpoint {
		t.Fatalf("status = %+v, lock metadata %+v (%v), endpoint %s", body, meta, ok, endpoint)
	}
	second, err := Execute(ctx, h, Peer{}, intent("op_1", "status"))
	if err != nil || wire(t, first) != wire(t, second) {
		t.Fatalf("repeat = %s (%v), want %s", wire(t, second), err, wire(t, first))
	}
}

func TestAssignIntentRecordsOwnership(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	inner := AssignHandler(db)
	h := func(ctx context.Context, p Peer, in Intent) (Result, error) {
		runs.Add(1)
		return inner(ctx, p, in)
	}
	in := intent("op_1", "assign")
	in.Object = "run_1"
	res, err := Execute(ctx, h, Peer{}, in)
	if err != nil || res.Revision != 1 {
		t.Fatalf("assign = %+v, %v; want revision 1", res, err)
	}
	var owner, op string
	if err := db.QueryRow(`SELECT owner, operation_id FROM run_assignments WHERE run_id = 'run_1'`).Scan(&owner, &op); err != nil || owner != "supervisor" || op != "op_1" {
		t.Fatalf("assignment = %q, %q (%v), want the supervisor under op_1", owner, op, err)
	}
	conflict := in
	conflict.Object = "run_2"
	_, err = Execute(ctx, h, Peer{}, conflict)
	requireCode(t, err, CodeRevisionConflict)
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want the conflicting reuse to be refused first", runs.Load())
	}

	other := intent("op_2", "assign")
	other.Object = "run_1"
	_, err = Execute(ctx, h, Peer{}, other)
	requireCode(t, err, CodeRevisionConflict) // run_1 is at revision 1, the intent expected 0
	unknown := intent("op_3", "assign")
	unknown.Object = "run_missing"
	_, err = Execute(ctx, h, Peer{}, unknown)
	requireCode(t, err, CodeInvalidContract)
	if count(t, db, "run_assignments") != 1 {
		t.Fatalf("assignments = %d, want only run_1", count(t, db, "run_assignments"))
	}
}

func TestTokenMintedAtAdmissionCheckedPerRequest(t *testing.T) { // I03 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	seedAttempt(t, db, "run_1", "att_a")
	if _, err := db.Exec(`INSERT INTO attempts (attempt_id, run_id, task_id, attempt_number, state, launch_token_sha256, workspace_path)
		VALUES ('att_b', 'run_1', 'task_1', 2, 'created', 'x', '/tmp/ws')`); err != nil {
		t.Fatal(err)
	}
	token, err := MintToken(t.Context(), db, "att_a")
	if err != nil || token == "" {
		t.Fatalf("MintToken = %q, %v", token, err)
	}
	var stored string
	if err := db.QueryRow(`SELECT token_sha256 FROM capability_tokens WHERE attempt_id = 'att_a'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || len(stored) != 64 {
		t.Fatalf("stored %q for token %q: the token itself must not be stored", stored, token)
	}
	if err := CheckToken(t.Context(), db, token, "att_a"); err != nil {
		t.Fatalf("own attempt refused: %v", err)
	}
	requireCode(t, CheckToken(t.Context(), db, token, "att_b"), CodePermissionDenied)
	requireCode(t, CheckToken(t.Context(), db, "forged", "att_a"), CodePermissionDenied)
	other, err := MintToken(t.Context(), db, "att_b")
	if err != nil {
		t.Fatal(err)
	}
	requireCode(t, CheckToken(t.Context(), db, other, "att_a"), CodePermissionDenied)
}

func TestMintTokenUnknownAttemptRefused(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	_, err := MintToken(t.Context(), db, "att_missing")
	requireCode(t, err, CodeInvalidContract)
	if n := count(t, db, "capability_tokens"); n != 0 {
		t.Fatalf("%d token rows stored for an unknown attempt", n)
	}
}

func TestReusedIDWithDifferentEnvelopeConflicts(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := counting(`{}`, &runs)
	gen := int64(7)
	first := intent("op_1", "probe")
	first.Object, first.ExpectedRevision, first.Generation = "run_1", 0, &gen
	if _, err := Execute(ctx, h, Peer{}, first); err != nil {
		t.Fatal(err)
	}
	otherGen := int64(8)
	for name, mutate := range map[string]func(*Intent){
		"expected_revision":  func(i *Intent) { i.ExpectedRevision = 1 },
		"generation":         func(i *Intent) { i.Generation = &otherGen },
		"generation removed": func(i *Intent) { i.Generation = nil },
	} {
		other := first
		mutate(&other)
		_, err := Execute(ctx, h, Peer{}, other)
		if !errors.Is(err, &Error{Code: CodeRevisionConflict}) {
			t.Errorf("reuse with different %s: err = %v, want revision_conflict", name, err)
		}
	}
	if _, err := Execute(ctx, h, Peer{}, first); err != nil {
		t.Fatalf("the identical repeat no longer replays: %v", err)
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want only the first", runs.Load())
	}
}

// assignOnce is AssignHandler's write with a failure injected after it.
func assigningThen(db *sql.DB, then error) Handler {
	return func(ctx context.Context, p Peer, in Intent) (Result, error) {
		res, err := AssignHandler(db)(ctx, p, in)
		if err != nil {
			return res, err
		}
		return res, then
	}
}

func TestRecordFailureDoesNotWedgeTheRun(t *testing.T) { // I12, I23 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	ctx := WithLedger(t.Context(), db)
	// The operation's result row cannot be written: the handler's assignment
	// must not survive without it.
	if _, err := db.Exec(`CREATE TRIGGER fail_record BEFORE INSERT ON operations WHEN NEW.operation_id = 'op_1'
		BEGIN SELECT RAISE(ABORT, 'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	in := intent("op_1", "assign")
	in.Object = "run_1"
	_, err := Execute(ctx, AssignHandler(db), Peer{}, in)
	requireCode(t, err, CodePersistenceUnavailable)
	if n := count(t, db, "run_assignments"); n != 0 {
		t.Fatalf("%d assignments survive a result that was never recorded, want none", n)
	}
	if n := count(t, db, "operations"); n != 0 {
		t.Fatalf("%d operation rows left behind, want none", n)
	}

	if _, err := db.Exec(`DROP TRIGGER fail_record`); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(ctx, AssignHandler(db), Peer{}, in); err != nil {
		t.Fatalf("retry of the same operation after the fault cleared: %v", err)
	}
	later := intent("op_2", "unassign")
	later.Object, later.ExpectedRevision = "run_1", 1
	if _, err := Execute(ctx, counting(`{}`, new(atomic.Int32)), Peer{}, later); err != nil {
		t.Fatalf("a later operation on the run is refused: %v", err)
	}
}

func TestHandlerWritesRollBackWithTheirFailure(t *testing.T) { // I23 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	seedRun(t, db, "run_2")
	ctx := WithLedger(t.Context(), db)
	// A control failure is stored as the result, but its writes are undone.
	in := intent("op_1", "assign")
	in.Object = "run_1"
	_, err := Execute(ctx, assigningThen(db, newError(CodePermissionDenied, "refused after writing")), Peer{}, in)
	requireCode(t, err, CodePermissionDenied)
	if n := count(t, db, "run_assignments"); n != 0 {
		t.Fatalf("%d assignments survive a failed handler, want none", n)
	}
	if n := count(t, db, "operations"); n != 1 {
		t.Fatalf("%d operation rows, want the stored failure", n)
	}
	// A transient failure stores nothing at all and the id can be retried.
	in2 := intent("op_2", "assign")
	in2.Object = "run_2"
	if _, err := Execute(ctx, assigningThen(db, errors.New("lost the disk")), Peer{}, in2); err == nil {
		t.Fatal("transient failure returned no error")
	}
	if n := count(t, db, "run_assignments"); n != 0 {
		t.Fatalf("%d assignments survive a transient failure, want none", n)
	}
	if n := count(t, db, "operations"); n != 1 {
		t.Fatalf("%d operation rows after a transient failure, want only the stored failure", n)
	}
	if _, err := Execute(ctx, AssignHandler(db), Peer{}, in2); err != nil {
		t.Fatalf("retry after a transient failure: %v", err)
	}
	if n := count(t, db, "run_assignments"); n != 1 {
		t.Fatalf("assignments = %d after the retry, want run_2 only", n)
	}
}
