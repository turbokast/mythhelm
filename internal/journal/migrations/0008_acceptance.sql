-- Schema v8 (protected-acceptance design §2.4, D14). Additive only: one side
-- table binding a verification to its reason, canonical attestation and
-- measured overhead, plus triggers that make it and the v7 policies table
-- immutable (I20). The released verifications and check_results DDL is
-- untouched (ADR 0016); a verification without a row here was recorded
-- before binding existed (unbound, never a default verdict).
CREATE TABLE verification_bindings (
  verification_id TEXT PRIMARY KEY REFERENCES verifications(verification_id),
  reason TEXT NOT NULL DEFAULT '',
  attestation TEXT CHECK (attestation IS NULL OR json_valid(attestation)), -- the v2contract.Verification, verbatim
  digest TEXT, -- v2contract.Digest of the attestation
  overhead_ms INTEGER,
  CHECK ((attestation IS NULL) = (digest IS NULL))
) STRICT;
-- BEFORE INSERT triggers close INSERT OR REPLACE, which fires no delete
-- trigger unless recursive_triggers is on.
CREATE TRIGGER verification_bindings_no_update BEFORE UPDATE ON verification_bindings
BEGIN SELECT RAISE(ABORT, 'verification_bindings is immutable'); END;
CREATE TRIGGER verification_bindings_no_delete BEFORE DELETE ON verification_bindings
BEGIN SELECT RAISE(ABORT, 'verification_bindings is immutable'); END;
CREATE TRIGGER verification_bindings_no_replace BEFORE INSERT ON verification_bindings
WHEN EXISTS (SELECT 1 FROM verification_bindings WHERE verification_id = NEW.verification_id)
BEGIN SELECT RAISE(ABORT, 'verification_bindings is immutable'); END;
CREATE TRIGGER policies_no_replace BEFORE INSERT ON policies
WHEN EXISTS (SELECT 1 FROM policies WHERE policy_id = NEW.policy_id AND version = NEW.version)
BEGIN SELECT RAISE(ABORT, 'policies is immutable'); END;
CREATE TRIGGER policies_no_update BEFORE UPDATE ON policies
BEGIN SELECT RAISE(ABORT, 'policies is immutable'); END;
CREATE TRIGGER policies_no_delete BEFORE DELETE ON policies
BEGIN SELECT RAISE(ABORT, 'policies is immutable'); END;
-- migration_state records the schema identity (0007); keep it current.
UPDATE migration_state SET schema_version = 8 WHERE id = 1;
