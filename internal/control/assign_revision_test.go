package control

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
)

func assignmentRevision(t *testing.T, db *sql.DB, runID string) uint64 {
	t.Helper()
	var revision uint64
	if err := Mutate(t.Context(), db, func(tx *sql.Tx) error {
		var err error
		revision, err = journal.RunRevision(t.Context(), tx, runID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestAssignmentAdvancesRunRevisionAtomically(t *testing.T) { // I23 (v2 section 5): the real control writer shares canonical revision SQL.
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "assigned")
	before := assignmentRevision(t, db, "assigned")
	ctx := WithLedger(t.Context(), db)
	in := intent("assignment", "assign")
	in.Object = "assigned"
	handler := AssignHandler(db)
	observed := false
	wrapped := func(ctx context.Context, peer Peer, in Intent) (Result, error) {
		result, err := handler(ctx, peer, in)
		if err != nil {
			return result, err
		}
		err = Mutate(ctx, db, func(tx *sql.Tx) error {
			got, err := journal.RunRevision(ctx, tx, "assigned")
			if err != nil {
				return err
			}
			if got != before+1 {
				t.Fatalf("assignment transaction revision=%d want%d", got, before+1)
			}
			observed = true
			return nil
		})
		return result, err
	}
	first, err := Execute(ctx, wrapped, Peer{}, in)
	if err != nil {
		t.Fatal(err)
	}
	if !observed {
		t.Fatal("assignment handler not reached")
	}
	if got := assignmentRevision(t, db, "assigned"); got != before+1 {
		t.Fatalf("committed revision=%d", got)
	}
	second, err := Execute(ctx, wrapped, Peer{}, in)
	if err != nil {
		t.Fatal(err)
	}
	if wire(t, first) != wire(t, second) {
		t.Fatalf("replay acknowledgment changed")
	}
	if got := assignmentRevision(t, db, "assigned"); got != before+1 {
		t.Fatalf("replay advanced revision=%d", got)
	}
}

func TestAssignmentRollbackKeepsRunRevision(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	seedRun(t, db, "rollback")
	before := assignmentRevision(t, db, "rollback")
	in := intent("rollback_assignment", "assign")
	in.Object = "rollback"
	sentinel := errors.New("abort after assignment")
	handler := AssignHandler(db)
	_, err := Execute(WithLedger(t.Context(), db), func(ctx context.Context, peer Peer, in Intent) (Result, error) {
		result, err := handler(ctx, peer, in)
		if err != nil {
			return result, err
		}
		return result, sentinel
	}, Peer{}, in)
	if !errors.Is(err, sentinel) {
		t.Fatalf("execute error=%v", err)
	}
	if got := assignmentRevision(t, db, "rollback"); got != before {
		t.Fatalf("rolled back revision=%d want%d", got, before)
	}
	if got := count(t, db, "run_assignments"); got != 0 {
		t.Fatalf("rolled back assignments=%d", got)
	}
	if got := count(t, db, "operations"); got != 0 {
		t.Fatalf("rolled back operation acknowledgments=%d", got)
	}
}

func TestCompletedOperationClaimKeepsWinner(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	in := intent("claim_winner", "probe")
	digest := intentDigest(in)
	claimed, err := insertClaim(t.Context(), db, in, digest)
	if err != nil || !claimed {
		t.Fatalf("fresh claim=%t %v", claimed, err)
	}
	claimed, err = insertClaim(t.Context(), db, in, digest)
	if err != nil || claimed {
		t.Fatalf("duplicate live claim=%t %v", claimed, err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE operations SET state='done',result='{"operation_id":"claim_winner","revision":1,"body":{"accepted":true}}',revision=1,finished_at='finished' WHERE operation_id='claim_winner'`); err != nil {
		t.Fatal(err)
	}
	winner, err := readClaim(t.Context(), db, in.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	// This is the actual loser-claim insertion after a concurrent winner
	// completed, as used by ExecuteLong. It must still return false, not fail.
	claimed, err = insertClaim(t.Context(), db, in, digest)
	if err != nil || claimed {
		t.Fatalf("duplicate completed claim=%t %v", claimed, err)
	}
	replay, err := readClaim(t.Context(), db, in.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if replay != winner || replay.state != "done" || replay.digest != digest {
		t.Fatalf("winner changed: %+v -> %+v", winner, replay)
	}
	result, err := replayClaim(replay, digest)
	if err != nil || result.OperationID != in.OperationID || result.Revision != 1 {
		t.Fatalf("winner replay=%+v %v", result, err)
	}
}
