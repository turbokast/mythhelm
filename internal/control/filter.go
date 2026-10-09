package control

import (
	"context"
	"database/sql"
	"encoding/json"
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
// params ({"repos": [...]}), filtered before they are counted. The
// repository set is the authority the same-user peer presents; no
// per-repository policy store exists yet.
func ReadHandler(db *sql.DB) Handler {
	return func(ctx context.Context, peer Peer, in Intent) (Result, error) {
		var p struct {
			Repos []string `json:"repos"`
		}
		if err := strictParams(in, &p); err != nil {
			return Result{}, err
		}
		if _, err := Filter(peer, p.Repos, nil); err != nil {
			return Result{}, err
		}
		rows, err := readRuns(ctx, db)
		if err != nil {
			return Result{}, err
		}
		kept, err := Filter(peer, p.Repos, rows)
		if err != nil {
			return Result{}, err
		}
		body, err := json.Marshal(map[string]any{"count": len(kept), "rows": kept})
		return Result{Body: body}, err
	}
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
