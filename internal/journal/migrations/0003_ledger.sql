-- Schema v3 (budget-ledger-s1 design §2): the budget ledger. Additive only.
CREATE TABLE usage_observations (
  observation_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(run_id),
  scope TEXT NOT NULL, unit TEXT NOT NULL, source TEXT NOT NULL,
  label TEXT NOT NULL CHECK (label IN ('reported','observed','estimated','user-declared','unknown')),
  quantity TEXT NOT NULL CHECK (quantity <> ''), -- decimal text, or exactly 'unknown'; never '' and never 0-for-unknown
  producer_id TEXT, producer_sequence INTEGER,
  observed_at TEXT NOT NULL
) STRICT;
CREATE TABLE run_envelopes (
  run_id TEXT PRIMARY KEY REFERENCES runs(run_id),
  execution_seconds INTEGER NOT NULL, repairs INTEGER NOT NULL,
  replans INTEGER NOT NULL, transport_retries INTEGER NOT NULL,
  first_start_at TEXT,
  repairs_used INTEGER NOT NULL DEFAULT 0, replans_used INTEGER NOT NULL DEFAULT 0,
  transport_retries_seen INTEGER NOT NULL DEFAULT 0,
  pause_spans TEXT NOT NULL DEFAULT '[]', -- JSON array of {requested_at, quiesced_at, resumed_at}
  updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE reservations (
  reservation_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(run_id),
  bucket TEXT NOT NULL, scope TEXT NOT NULL, owner TEXT NOT NULL,
  quantity TEXT NOT NULL CHECK (quantity <> ''), -- decimal text or exactly 'unknown'
  status TEXT NOT NULL CHECK (status IN ('held','released','orphaned')),
  expires_at TEXT NOT NULL, heartbeat_at TEXT,
  release_evidence TEXT,
  created_at TEXT NOT NULL
) STRICT;
CREATE TABLE bucket_state (
  bucket TEXT PRIMARY KEY, -- QuotaBucket: harness, surface, entitlement class, identity ref
  exhausted_at TEXT NOT NULL, reset_at TEXT, -- NULL reset stays unknown, never invented
  retries_used INTEGER NOT NULL DEFAULT 0
) STRICT;
