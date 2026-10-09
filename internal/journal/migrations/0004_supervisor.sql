-- Schema v4 (supervisor-service design §2, renumbered from 0003: budget-ledger-s1
-- task 3 took 0003 first). Additive only. The reservations table already exists
-- from 0003_ledger.sql; supervisor-service task 6 reconciles it.
CREATE TABLE operations (
  operation_id TEXT PRIMARY KEY,
  method TEXT NOT NULL,
  object TEXT NOT NULL DEFAULT '',
  digest TEXT NOT NULL, -- sha256 of method, object and compact params; a repeat must match
  state TEXT NOT NULL CHECK (state IN ('claimed','done')),
  revision INTEGER NOT NULL DEFAULT 0, -- the object's revision after this operation
  result TEXT CHECK (result IS NULL OR json_valid(result)),
  claimed_at TEXT NOT NULL, finished_at TEXT,
  CHECK ((state = 'done') = (result IS NOT NULL))
) STRICT;
CREATE INDEX operations_object ON operations (object) WHERE object <> '';
-- A recorded result is immutable: repeats replay it, never rewrite it (I20).
CREATE TRIGGER operations_done_no_update BEFORE UPDATE ON operations WHEN OLD.state = 'done'
  BEGIN SELECT RAISE(ABORT, 'a recorded operation result is immutable'); END;
CREATE TRIGGER operations_done_no_delete BEFORE DELETE ON operations WHEN OLD.state = 'done'
  BEGIN SELECT RAISE(ABORT, 'a recorded operation result is immutable'); END;
CREATE TABLE capability_tokens (
  attempt_id TEXT PRIMARY KEY REFERENCES attempts(attempt_id),
  token_sha256 TEXT NOT NULL,
  minted_at TEXT NOT NULL
) STRICT;
CREATE TABLE run_assignments (
  run_id TEXT PRIMARY KEY REFERENCES runs(run_id),
  owner TEXT NOT NULL,
  assigned_at TEXT NOT NULL,
  operation_id TEXT NOT NULL REFERENCES operations(operation_id) DEFERRABLE INITIALLY DEFERRED -- the result row is inserted after the handler, in the same transaction
) STRICT;
