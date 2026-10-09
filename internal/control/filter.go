package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
)

// Row is the supervisor-internal read shape. It never crosses the wire
// unfiltered.
type Row struct {
	RepoID  string          `json:"repo_id"`
	Payload json.RawMessage `json:"payload"`
}

// Filter applies repository authority before any content leaves the
// supervisor: rows outside repos are removed before counts, names or index
// metadata are computed, so a filtered-out repository contributes nothing,
// not even a redacted stub. A peer that is not the supervisor's user, or
// authorised for no repository, gets permission_denied.
func Filter(peer Peer, repos []string, rows []Row) ([]Row, error) {
	if !peer.SameUser {
		return nil, newError(CodePermissionDenied, "peer is not the supervisor's user")
	}
	if len(repos) == 0 {
		return nil, newError(CodePermissionDenied, "peer is authorised for no repository")
	}
	kept := make([]Row, 0, len(rows))
	for _, r := range rows {
		if slices.Contains(repos, r.RepoID) {
			kept = append(kept, r)
		}
	}
	return kept, nil
}

// ReadHandler answers the read intent: the runs of the repositories named in
// params ({"repos": [...]}), filtered before they are counted. A request
// carrying an attempt's capability token is bounded to that attempt's run
// repository, which the supervisor reads from its own ledger; the list can
// only narrow it, never widen it. A token-less same-user peer is the operator:
// no per-repository policy store exists yet, so the list it presents is its
// scope.
func ReadHandler(db *sql.DB) Handler {
	return func(ctx context.Context, peer Peer, in Intent) (Result, error) {
		var p struct {
			Repos []string `json:"repos"`
		}
		if err := strictParams(in, &p); err != nil {
			return Result{}, err
		}
		repos := p.Repos
		if in.CapabilityToken != "" {
			entitled, err := tokenRepo(ctx, db, in.CapabilityToken)
			if err != nil {
				return Result{}, err
			}
			repos = narrow(p.Repos, entitled)
		}
		if _, err := Filter(peer, repos, nil); err != nil {
			return Result{}, err
		}
		rows, err := readRuns(ctx, db)
		if err != nil {
			return Result{}, err
		}
		kept, err := Filter(peer, repos, rows)
		if err != nil {
			return Result{}, err
		}
		body, err := json.Marshal(map[string]any{"count": len(kept), "rows": kept})
		return Result{Body: body}, err
	}
}

// narrow bounds a requested repository list by the one repository an
// attempt is entitled to: an empty request means the entitled repository,
// and a request that omits it leaves nothing in scope.
func narrow(requested []string, entitled string) []string {
	if len(requested) == 0 || slices.Contains(requested, entitled) {
		return []string{entitled}
	}
	return nil
}

// tokenRepo is the repository of the run the token's attempt belongs to. An
// unknown token is permission_denied.
func tokenRepo(ctx context.Context, db *sql.DB, token string) (string, error) {
	var repo string
	err := db.QueryRowContext(ctx, `SELECT r.source_repo FROM capability_tokens t
		JOIN attempts a ON a.attempt_id = t.attempt_id JOIN runs r ON r.run_id = a.run_id
		WHERE t.token_sha256 = ?`, tokenDigest(token)).Scan(&repo)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", newError(CodePermissionDenied, "capability token is not valid")
	case err != nil:
		return "", newError(CodePersistenceUnavailable, "reading token scope: %v", err)
	}
	return repo, nil
}

func readRuns(ctx context.Context, db *sql.DB) ([]Row, error) {
	rs, err := db.QueryContext(ctx, `SELECT run_id, state, source_repo FROM runs ORDER BY run_id`)
	if err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading runs: %v", err)
	}
	defer func() { _ = rs.Close() }()
	var rows []Row
	for rs.Next() {
		var runID, state, repo string
		if err := rs.Scan(&runID, &state, &repo); err != nil {
			return nil, newError(CodePersistenceUnavailable, "scanning run: %v", err)
		}
		payload, err := json.Marshal(map[string]string{"run_id": runID, "state": state})
		if err != nil {
			return nil, err
		}
		rows = append(rows, Row{RepoID: repo, Payload: payload})
	}
	if err := rs.Err(); err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading runs: %v", err)
	}
	return rows, nil
}
