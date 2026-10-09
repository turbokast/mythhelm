-- Schema v7 (supervisor-migration design §2, D2; renumbered from 0004:
-- budget-ledger-s1, supervisor-service, reservation-host and evaluator work
-- claimed 0003-0006 first). Additive only: five new tables storing stream-1
-- records verbatim as canonical JSON (Digest-addressed) plus index columns.
-- No v1 table is altered; v1 rows stay for decode (AC-7.5).
CREATE TABLE tasks (
  task_id TEXT PRIMARY KEY,
  head_revision INTEGER NOT NULL DEFAULT 0,
  run_id TEXT NOT NULL DEFAULT '' -- the legacy v1 run this task was imported from, if any
) STRICT;
CREATE TABLE task_revisions (
  task_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  record TEXT NOT NULL CHECK (json_valid(record)), -- the v2contract.TaskRevision, verbatim
  digest TEXT NOT NULL, -- v2contract.Digest of the canonical record
  PRIMARY KEY (task_id, revision)
) STRICT;
CREATE INDEX task_revisions_run ON task_revisions (run_id) WHERE run_id <> '';
CREATE TABLE policies (
  policy_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  record TEXT NOT NULL CHECK (json_valid(record)), -- the v2contract.PolicyVersion, verbatim
  digest TEXT NOT NULL,
  PRIMARY KEY (policy_id, version)
) STRICT;
CREATE TABLE grants (
  grant_id TEXT PRIMARY KEY,
  record TEXT NOT NULL CHECK (json_valid(record)), -- the v2contract.Grant, verbatim
  digest TEXT NOT NULL
) STRICT;
-- One row: the durable migration phase plus schema/build identities.
CREATE TABLE migration_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  phase TEXT NOT NULL CHECK (phase IN ('not_started', 'previewed', 'drained', 'imported', 'adopted')),
  started_at TEXT NOT NULL DEFAULT '',
  backup_path TEXT NOT NULL DEFAULT '',
  schema_version INTEGER NOT NULL,
  build_version TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO migration_state (id, phase, started_at, backup_path, schema_version, build_version)
  VALUES (1, 'not_started', '', '', 7, '');
