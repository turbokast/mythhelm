-- Schema v6 (contained-execution-profiles design §2.7): a verification records
-- the evaluator that ran its checks, so a later change of boundary, check
-- policy or check definitions never reuses an old row. A side table keeps the
-- released verifications DDL untouched; a verification without a row here has
-- an unrecorded evaluator (unknown, never a claimed one).
CREATE TABLE verification_evaluators (
  verification_id TEXT PRIMARY KEY REFERENCES verifications(verification_id),
  evaluator_name TEXT NOT NULL, evaluator_digest TEXT NOT NULL
) STRICT;
