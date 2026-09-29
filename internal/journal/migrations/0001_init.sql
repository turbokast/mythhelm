-- Schema v1 (design §5, ADR 0003). A released migration is never edited; later
-- changes go in a new numbered file.
CREATE TABLE runs (
  run_id TEXT PRIMARY KEY, state TEXT NOT NULL, reason TEXT,
  adapter_id TEXT NOT NULL, source_repo TEXT NOT NULL, source_branch TEXT,
  base_rev TEXT, task_sha256 TEXT NOT NULL, billing_posture TEXT NOT NULL,
  execution_profile TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE attempts (
  attempt_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(run_id),
  task_id TEXT NOT NULL, attempt_number INTEGER NOT NULL, state TEXT NOT NULL, reason TEXT,
  launch_token_sha256 TEXT NOT NULL, worker_pid INTEGER, worker_start_time TEXT,
  native_pid INTEGER, native_pgid INTEGER, native_session_id TEXT,
  workspace_path TEXT NOT NULL, spool_offset INTEGER NOT NULL DEFAULT 0,
  UNIQUE (run_id, task_id, attempt_number)
) STRICT;
CREATE TABLE journal (
  seq INTEGER PRIMARY KEY, event_id TEXT NOT NULL UNIQUE, schema_version INTEGER NOT NULL,
  run_id TEXT NOT NULL, task_id TEXT, attempt_id TEXT,
  producer_id TEXT NOT NULL, producer_sequence INTEGER NOT NULL,
  run_sequence INTEGER NOT NULL, generation INTEGER NOT NULL, caused_by TEXT,
  observed_at TEXT NOT NULL, type TEXT NOT NULL,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  UNIQUE (producer_id, producer_sequence), UNIQUE (run_id, run_sequence)
) STRICT;
CREATE TRIGGER journal_no_update BEFORE UPDATE ON journal BEGIN SELECT RAISE(ABORT, 'journal is append-only'); END;
CREATE TRIGGER journal_no_delete BEFORE DELETE ON journal BEGIN SELECT RAISE(ABORT, 'journal is append-only'); END;
CREATE TABLE producers (producer_id TEXT PRIMARY KEY, last_sequence INTEGER NOT NULL, generation INTEGER NOT NULL) STRICT;
CREATE TABLE candidates (
  attempt_id TEXT PRIMARY KEY REFERENCES attempts(attempt_id), base_rev TEXT NOT NULL,
  candidate_commit TEXT NOT NULL, tree_id TEXT NOT NULL, patch_sha256 TEXT NOT NULL,
  changed_paths TEXT NOT NULL CHECK (json_valid(changed_paths)),
  flags TEXT NOT NULL CHECK (json_valid(flags)), partial INTEGER NOT NULL
) STRICT;
CREATE TABLE verifications (
  verification_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(run_id),
  candidate_commit TEXT NOT NULL, config_sha256 TEXT NOT NULL, result TEXT NOT NULL,
  started_at TEXT NOT NULL, finished_at TEXT
) STRICT;
CREATE TABLE check_results (
  verification_id TEXT NOT NULL REFERENCES verifications(verification_id), name TEXT NOT NULL,
  argv TEXT NOT NULL, status TEXT NOT NULL, exit_code INTEGER, duration_ms INTEGER,
  evidence_path TEXT, evidence_sha256 TEXT, PRIMARY KEY (verification_id, name)
) STRICT;
CREATE TABLE trust_grants (
  kind TEXT NOT NULL CHECK (kind IN ('project_config','native_config')),
  repo_identity TEXT NOT NULL, digest TEXT NOT NULL, granted_at TEXT NOT NULL,
  PRIMARY KEY (kind, repo_identity, digest)
) STRICT;
CREATE TABLE declarations (
  adapter_id TEXT NOT NULL, plan_class TEXT NOT NULL,
  extra_usage TEXT NOT NULL CHECK (extra_usage = 'disabled'),
  identity_ref TEXT NOT NULL,  -- sha256(orgId || configDirectory) from auth status
  declared_at TEXT NOT NULL, superseded_at TEXT
) STRICT;
-- at most one current declaration per adapter and identity; declaring supersedes the previous row in the same transaction
CREATE UNIQUE INDEX declarations_current ON declarations (adapter_id, identity_ref) WHERE superseded_at IS NULL;
CREATE TABLE applies (
  apply_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(run_id),
  target_repo TEXT NOT NULL, branch TEXT NOT NULL, candidate_commit TEXT NOT NULL,
  state TEXT NOT NULL, reason TEXT, created_at TEXT NOT NULL
) STRICT;
