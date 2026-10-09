package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
)

func reserveOpts(op, host, bucket, scope, quantity, owner string) ReserveOptions {
	return ReserveOptions{OperationID: op, Host: host, Bucket: bucket, Scope: scope, Quantity: quantity, Owner: owner}
}

func reservations(t *testing.T, db *sql.DB) int { return count(t, db, "reservations") }

func TestBoundsFailurePreventsWork(t *testing.T) { // I05 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	ctx := t.Context()

	over := reserveOpts("op_1", "h1", "b", "tokens", "11", "run_1")
	over.Bound = "10"
	_, err := Reserve(ctx, db, over)
	requireCode(t, err, CodeAllowanceExhausted)
	unknown := reserveOpts("op_2", "h1", "b", "tokens", "unknown", "run_1")
	unknown.Bound = "10"
	_, err = Reserve(ctx, db, unknown)
	requireCode(t, err, CodeAllowanceExhausted) // not shown to be within the bound
	if n := reservations(t, db); n != 0 {
		t.Fatalf("%d reservations recorded for refused work, want none", n)
	}

	if _, err := db.Exec(`INSERT INTO bucket_state (bucket, exhausted_at) VALUES ('spent', 't')`); err != nil {
		t.Fatal(err)
	}
	_, err = Reserve(ctx, db, reserveOpts("op_3", "h1", "spent", "tokens", "1", "run_1"))
	requireCode(t, err, CodeAllowanceExhausted)

	within := reserveOpts("op_4", "h1", "b", "tokens", "10", "run_1")
	within.Bound = "10.0"
	if _, err := Reserve(ctx, db, within); err != nil {
		t.Fatalf("a quantity at the bound is refused: %v", err)
	}
	if n := reservations(t, db); n != 1 {
		t.Fatalf("%d reservations, want only the one within bounds", n)
	}
}

func TestReservationKeyedByHostAndResource(t *testing.T) { // I05 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	ctx := t.Context()
	mustReserve := func(op, host, scope string) {
		t.Helper()
		if _, err := Reserve(ctx, db, reserveOpts(op, host, "b", scope, "1", "run_1")); err != nil {
			t.Fatalf("reserve %s on %s/%s: %v", op, host, scope, err)
		}
	}
	mustReserve("op_1", "h1", "tokens")
	mustReserve("op_2", "h2", "tokens") // same bucket and scope, another host
	mustReserve("op_3", "h1", "usd")    // same host and bucket, another scope
	_, err := Reserve(ctx, db, reserveOpts("op_4", "h1", "b", "tokens", "1", "run_1"))
	requireCode(t, err, CodeRevisionConflict)
	if n := reservations(t, db); n != 3 {
		t.Fatalf("%d reservations, want the three distinct keys", n)
	}

	if err := Release(ctx, db, "op_5", "res_op_1", "done"); err != nil {
		t.Fatal(err)
	}
	mustReserve("op_6", "h1", "tokens") // a released key can be held again
}

func TestQuantityUnknownNeverZero(t *testing.T) { // I09 (v2 §2)
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "run_1")
	if _, err := Reserve(t.Context(), db, reserveOpts("op_1", "h", "b", "s", "unknown", "run_1")); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT quantity FROM reservations`).Scan(&stored); err != nil || stored != "unknown" {
		t.Fatalf("stored quantity = %q (%v), want exactly unknown", stored, err)
	}
	for _, q := range []string{"0", "", "0.0", "-1", "1e3", "abc"} {
		_, err := Reserve(t.Context(), db, reserveOpts("op_"+q, "h", "b2", "s", q, "run_1"))
		if !errors.Is(err, &Error{Code: CodeInvalidContract}) {
			t.Errorf("quantity %q: err = %v, want invalid_contract", q, err)
		}
	}
	if n := reservations(t, db); n != 1 {
		t.Fatalf("%d reservations, want only the unknown one", n)
	}
}

func TestReserveRejectsUnknownOwnerAndEmptyFields(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	for name, o := range map[string]ReserveOptions{
		"unknown owner": reserveOpts("op_1", "h", "b", "s", "1", "nobody"),
		"empty bucket":  reserveOpts("op_2", "h", "", "s", "1", "run_1"),
		"empty host":    reserveOpts("op_3", "", "b", "s", "1", "run_1"),
		"bad bound":     {OperationID: "op_4", Host: "h", Bucket: "b", Scope: "s", Quantity: "1", Owner: "run_1", Bound: "x"},
	} {
		if _, err := Reserve(t.Context(), db, o); !errors.Is(err, &Error{Code: CodeInvalidContract}) {
			t.Errorf("%s: err = %v, want invalid_contract", name, err)
		}
	}
}

func supervisorFixture(t *testing.T) (*sql.DB, context.Context, *Server) {
	t.Helper()
	db, _ := openLedger(t)
	return db, WithLedger(t.Context(), db), NewSupervisorServer(db)
}

func params(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReserveIntentViaExecute(t *testing.T) {
	t.Parallel()
	db, ctx, srv := supervisorFixture(t)
	seedRun(t, db, "run_1")
	in := intent("op_1", "reserve")
	in.Params = params(t, map[string]string{"bucket": "b", "scope": "tokens", "quantity": "5", "owner": "run_1"})
	first, err := srv.Dispatch(ctx, Peer{SameUser: true}, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := srv.Dispatch(ctx, Peer{SameUser: true}, in)
	if err != nil || wire(t, first) != wire(t, second) {
		t.Fatalf("repeat = %s (%v), want %s", wire(t, second), err, wire(t, first))
	}
	if n := reservations(t, db); n != 1 {
		t.Fatalf("%d reservation rows after an identical repeat, want 1", n)
	}
	host, _ := os.Hostname()
	var gotHost, status string
	if err := db.QueryRow(`SELECT execution_host, status FROM reservations`).Scan(&gotHost, &status); err != nil || gotHost != host || status != "held" {
		t.Fatalf("reservation host %q status %q (%v), want this host %q held", gotHost, status, err, host)
	}

	bad := intent("op_2", "reserve")
	bad.Params = params(t, map[string]string{"bucket": "b", "scope": "tokens", "quantity": "5", "owner": "run_1", "host": "elsewhere"})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, bad); !errors.Is(err, &Error{Code: CodeInvalidContract}) {
		t.Fatalf("a client-chosen host: err = %v, want invalid_contract", err)
	}
}

func TestReleaseIntentViaExecute(t *testing.T) {
	t.Parallel()
	db, ctx, srv := supervisorFixture(t)
	seedRun(t, db, "run_1")
	reserve := intent("op_1", "reserve")
	reserve.Params = params(t, map[string]string{"bucket": "b", "scope": "s", "quantity": "unknown", "owner": "run_1"})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, reserve); err != nil {
		t.Fatal(err)
	}
	release := intent("op_2", "release")
	release.Params = params(t, map[string]string{"reservation_id": "res_op_1", "evidence": "attempt concluded"})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, release); err != nil {
		t.Fatal(err)
	}
	var status, evidence string
	if err := db.QueryRow(`SELECT status, release_evidence FROM reservations`).Scan(&status, &evidence); err != nil || status != "released" || evidence != "attempt concluded" {
		t.Fatalf("reservation = %q %q (%v), want released with the evidence", status, evidence, err)
	}
	again := intent("op_3", "release")
	again.Params = release.Params
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, again); !errors.Is(err, &Error{Code: CodeRevisionConflict}) {
		t.Fatalf("second release: err = %v, want revision_conflict", err)
	}
	noEvidence := intent("op_4", "release")
	noEvidence.Params = params(t, map[string]string{"reservation_id": "res_op_1"})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, noEvidence); !errors.Is(err, &Error{Code: CodeInvalidContract}) {
		t.Fatalf("release without evidence: err = %v, want invalid_contract", err)
	}
}

func TestHeartbeatIntentViaExecute(t *testing.T) {
	t.Parallel()
	db, ctx, srv := supervisorFixture(t)
	seedRun(t, db, "run_1")
	reserve := intent("op_1", "reserve")
	reserve.Params = params(t, map[string]string{"bucket": "b", "scope": "s", "quantity": "1", "owner": "run_1"})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, reserve); err != nil {
		t.Fatal(err)
	}
	var before sql.NullString
	if err := db.QueryRow(`SELECT heartbeat_at FROM reservations`).Scan(&before); err != nil || before.Valid {
		t.Fatalf("heartbeat_at = %v (%v) before any heartbeat, want NULL", before, err)
	}
	beat := intent("op_2", "heartbeat")
	beat.Params = params(t, map[string]string{"reservation_id": "res_op_1"})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, beat); err != nil {
		t.Fatal(err)
	}
	var after sql.NullString
	if err := db.QueryRow(`SELECT heartbeat_at FROM reservations`).Scan(&after); err != nil || !after.Valid || after.String == "" {
		t.Fatalf("heartbeat_at = %v (%v) after a heartbeat, want a time", after, err)
	}
	if _, err := db.Exec(`UPDATE reservations SET status = 'released'`); err != nil {
		t.Fatal(err)
	}
	again := intent("op_3", "heartbeat")
	again.Params = beat.Params
	if _, err := srv.Dispatch(ctx, Peer{SameUser: true}, again); !errors.Is(err, &Error{Code: CodeRevisionConflict}) {
		t.Fatalf("heartbeat on a released reservation: err = %v, want revision_conflict", err)
	}
}

func TestTokenScopedToAttempt(t *testing.T) { // I03 (v2 §2)
	t.Parallel()
	db, ctx, srv := supervisorFixture(t)
	seedRun(t, db, "run_1")
	seedAttempt(t, db, "run_1", "att_a")
	if _, err := db.Exec(`INSERT INTO attempts (attempt_id, run_id, task_id, attempt_number, state, launch_token_sha256, workspace_path)
		VALUES ('att_b', 'run_1', 'task_1', 2, 'created', 'x', '/tmp/ws')`); err != nil {
		t.Fatal(err)
	}
	tokenA, err := MintToken(ctx, db, "att_a")
	if err != nil {
		t.Fatal(err)
	}
	reserve := func(op, owner, token string) error {
		in := intent(op, "reserve")
		in.CapabilityToken = token
		in.Params = params(t, map[string]string{"bucket": "b", "scope": op, "quantity": "1", "owner": owner})
		_, err := srv.Dispatch(ctx, Peer{SameUser: true}, in)
		return err
	}
	if err := reserve("op_1", "att_a", tokenA); err != nil {
		t.Fatalf("attempt A with its own token: %v", err)
	}
	requireCode(t, reserve("op_2", "att_b", tokenA), CodePermissionDenied)
	requireCode(t, reserve("op_3", "att_a", ""), CodePermissionDenied)
	// Another attempt's token does not release A's reservation either.
	tokenB, err := MintToken(ctx, db, "att_b")
	if err != nil {
		t.Fatal(err)
	}
	rel := intent("op_4", "release")
	rel.CapabilityToken = tokenB
	rel.Params = params(t, map[string]string{"reservation_id": "res_op_1", "evidence": "x"})
	_, err = srv.Dispatch(ctx, Peer{SameUser: true}, rel)
	requireCode(t, err, CodePermissionDenied)
	if n := reservations(t, db); n != 1 {
		t.Fatalf("%d reservations, want only attempt A's", n)
	}
}

func TestFilterHidesForeignRepos(t *testing.T) { // I03 (v2 §2)
	t.Parallel()
	rows := []Row{
		{RepoID: "repo_a", Payload: json.RawMessage(`{"run_id":"run_1"}`)},
		{RepoID: "repo_b", Payload: json.RawMessage(`{"run_id":"run_2"}`)},
		{RepoID: "repo_b", Payload: json.RawMessage(`{"run_id":"run_3"}`)},
	}
	peer := Peer{SameUser: true}
	got, err := Filter(peer, []string{"repo_a"}, rows)
	if err != nil || len(got) != 1 || got[0].RepoID != "repo_a" {
		t.Fatalf("Filter = %+v, %v; want only repo_a's row", got, err)
	}
	none, err := Filter(peer, []string{"repo_c"}, rows)
	if err != nil || len(none) != 0 {
		t.Fatalf("Filter for a repo with no rows = %+v, %v; want a count of 0", none, err)
	}
	raw, _ := json.Marshal(none)
	if strings.Contains(string(raw), "repo_") || strings.Contains(string(raw), "run_") {
		t.Fatalf("filtered output %s names a foreign repository or run", raw)
	}
	_, err = Filter(peer, nil, rows)
	requireCode(t, err, CodePermissionDenied)
	_, err = Filter(Peer{SameUser: false}, []string{"repo_a"}, rows)
	requireCode(t, err, CodePermissionDenied)
}

func TestReadIntentFiltersForeignRepos(t *testing.T) {
	t.Parallel()
	db, ctx, srv := supervisorFixture(t)
	for _, r := range [][2]string{{"run_a1", "/repos/a"}, {"run_b1", "/repos/b"}, {"run_b2", "/repos/b"}} {
		if _, err := db.Exec(`INSERT INTO runs (run_id, state, adapter_id, source_repo, task_sha256, billing_posture, execution_profile, created_at, updated_at)
			VALUES (?, 'created', 'a', ?, 't', 'b', 'p', 't', 't')`, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	read := func(op string, repos ...string) (struct {
		Count int `json:"count"`
		Rows  []Row
	}, string, error) {
		in := intent(op, "read")
		in.Params = params(t, map[string]any{"repos": repos})
		res, err := srv.Dispatch(ctx, Peer{SameUser: true}, in)
		var body struct {
			Count int `json:"count"`
			Rows  []Row
		}
		if err == nil {
			err = json.Unmarshal(res.Body, &body)
		}
		return body, string(res.Body), err
	}
	body, _, err := read("op_1", "/repos/a")
	if err != nil || body.Count != 1 || len(body.Rows) != 1 || body.Rows[0].RepoID != "/repos/a" {
		t.Fatalf("read = %+v, %v; want only /repos/a", body, err)
	}
	body, raw, err := read("op_2", "/repos/elsewhere")
	if err != nil || body.Count != 0 || strings.Contains(raw, "/repos/a") || strings.Contains(raw, "/repos/b") || strings.Contains(raw, "run_") {
		t.Fatalf("read of a foreign repo = %s, %v; want a count of 0 and no names", raw, err)
	}
	_, _, err = read("op_3")
	requireCode(t, err, CodePermissionDenied)
	in := intent("op_4", "read")
	in.Params = params(t, map[string]any{"repos": []string{"/repos/a"}})
	if _, err := srv.Dispatch(ctx, Peer{SameUser: false}, in); !errors.Is(err, &Error{Code: CodePermissionDenied}) {
		t.Fatalf("read by another user: err = %v, want permission_denied", err)
	}
}

func TestMigration0005KeysReservationsByHost(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, journal.DBName)
	v4, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_init.sql", "0002_qualification.sql", "0003_ledger.sql", "0004_supervisor.sql"} {
		raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "journal", "migrations", name)))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := v4.Exec(string(raw)); err != nil {
			t.Fatalf("applying %s: %v", name, err)
		}
	}
	for _, q := range []string{
		`PRAGMA user_version = 4`,
		`INSERT INTO runs (run_id, state, adapter_id, source_repo, task_sha256, billing_posture, execution_profile, created_at, updated_at)
			VALUES ('run_1','created','a','r','t','b','p','t','t')`,
		`INSERT INTO reservations (reservation_id, run_id, bucket, scope, owner, quantity, status, expires_at, created_at)
			VALUES ('res_old','run_1','b','s','o','1','held','t','t')`,
	} {
		if _, err := v4.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	before := schemaObjects(t, v4)
	if err := v4.Close(); err != nil {
		t.Fatal(err)
	}

	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	after := schemaObjects(t, db)
	for name, ddl := range before {
		if name != "reservations" && after[name] != ddl {
			t.Errorf("DDL of %s changed by migration 0005", name)
		}
	}
	var host string
	if err := db.QueryRow(`SELECT execution_host FROM reservations WHERE reservation_id = 'res_old'`).Scan(&host); err != nil || host != "" {
		t.Fatalf("existing reservation host = %q (%v), want it kept with an empty host", host, err)
	}
	// Rows without a host stay outside the uniqueness rule; rows with one do not.
	insert := func(id, host string) error {
		_, err := db.Exec(`INSERT INTO reservations (reservation_id, run_id, execution_host, bucket, scope, owner, quantity, status, expires_at, created_at)
			VALUES (?, 'run_1', ?, 'b', 's', 'o', '1', 'held', 't', 't')`, id, host)
		return err
	}
	if err := insert("res_old2", ""); err != nil {
		t.Errorf("a second host-less held reservation is refused: %v", err)
	}
	if err := insert("res_h1", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := insert("res_h1b", "h1"); err == nil {
		t.Error("two held reservations on one host, bucket and scope were accepted")
	}
	if err := insert("res_h2", "h2"); err != nil {
		t.Errorf("another host with the same bucket and scope is refused: %v", err)
	}
}

func TestReadIntentTokenBindsRepoEntitlement(t *testing.T) { // I03 (v2 §2)
	t.Parallel()
	db, ctx, srv := supervisorFixture(t)
	for _, r := range [][2]string{{"run_a", "/repos/a"}, {"run_b", "/repos/b"}} {
		if _, err := db.Exec(`INSERT INTO runs (run_id, state, adapter_id, source_repo, task_sha256, billing_posture, execution_profile, created_at, updated_at)
			VALUES (?, 'created', 'a', ?, 't', 'b', 'p', 't', 't')`, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	seedAttempt(t, db, "run_a", "att_a")
	token, err := MintToken(ctx, db, "att_a")
	if err != nil {
		t.Fatal(err)
	}
	read := func(op, token string, repos ...string) (string, error) {
		in := intent(op, "read")
		in.CapabilityToken = token
		in.Params = params(t, map[string]any{"repos": repos})
		res, err := srv.Dispatch(ctx, Peer{SameUser: true}, in)
		return string(res.Body), err
	}
	// Asking for more than its entitlement narrows to the entitlement.
	body, err := read("op_1", token, "/repos/a", "/repos/b")
	if err != nil || !strings.Contains(body, `"count":1`) || strings.Contains(body, "run_b") || strings.Contains(body, "/repos/b") {
		t.Fatalf("read widened by the list = %s, %v; want only /repos/a", body, err)
	}
	// An empty request means the entitlement.
	body, err = read("op_2", token)
	if err != nil || !strings.Contains(body, `"count":1`) || !strings.Contains(body, "run_a") {
		t.Fatalf("read with no list = %s, %v; want the attempt's own repository", body, err)
	}
	// Asking only for a foreign repository leaves nothing in scope.
	body, err = read("op_3", token, "/repos/b")
	requireCode(t, err, CodePermissionDenied)
	if strings.Contains(body, "run_b") || strings.Contains(body, "/repos/b") {
		t.Fatalf("refused read leaked %s", body)
	}
	// A forged token is not a token-less operator.
	_, err = read("op_4", "forged", "/repos/a", "/repos/b")
	requireCode(t, err, CodePermissionDenied)
	// The token-less same-user peer keeps the operator scope it presents.
	body, err = read("op_5", "", "/repos/a", "/repos/b")
	if err != nil || !strings.Contains(body, `"count":2`) {
		t.Fatalf("operator read = %s, %v; want both repositories", body, err)
	}
}
