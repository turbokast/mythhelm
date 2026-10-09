-- Schema v5 (supervisor-service design §8): reservations are keyed by execution
-- host. The table comes from 0003_ledger.sql, which has no host column; this is
-- the additive reconciliation (no table rebuild). Rows written without a host
-- keep host '' and sit outside the uniqueness rule.
ALTER TABLE reservations ADD COLUMN execution_host TEXT NOT NULL DEFAULT '';
-- At most one held reservation per host, bucket and scope (AC-6.2).
CREATE UNIQUE INDEX reservations_held_key ON reservations (execution_host, bucket, scope)
  WHERE status = 'held' AND execution_host <> '';
