-- Schema v11 (ADR0022; design section3.2). Additive durable evidence only.
-- Task30 validates canonical codecs, consent and fresh semantic bindings;
-- SQL enforces structural identity, immutable records and legal transitions.
-- A planned attempt is not an attempts row and grants no launch authority.
CREATE TABLE service_admission_bindings (
 preparation_id TEXT PRIMARY KEY REFERENCES prepared_runs(preparation_id),
 codec_version INTEGER NOT NULL CHECK (codec_version = 1),
 canonical_binding TEXT NOT NULL CHECK (json_valid(canonical_binding) AND json_type(canonical_binding) = 'object' AND length(CAST(canonical_binding AS BLOB)) BETWEEN 1 AND 1048576),
 binding_sha256 TEXT NOT NULL,
 root_id TEXT NOT NULL CHECK (root_id <> ''),
 peer_id TEXT NOT NULL CHECK (peer_id <> ''),
 generation INTEGER NOT NULL CHECK (generation > 0),
 context_epoch TEXT NOT NULL CHECK (context_epoch <> ''),
 policy_revision TEXT NOT NULL CHECK (policy_revision <> ''),
 task_artifact_sha256 TEXT NOT NULL,
 task_artifact_ref TEXT NOT NULL CHECK (task_artifact_ref <> ''),
 expires_at TEXT NOT NULL CHECK (expires_at <> ''),
 retention_policy TEXT NOT NULL CHECK (json_valid(retention_policy) AND json_type(retention_policy) = 'object' AND length(CAST(retention_policy AS BLOB)) BETWEEN 1 AND 1048576),
 retention_policy_sha256 TEXT NOT NULL,
 retention_approval TEXT NOT NULL CHECK (json_valid(retention_approval) AND json_type(retention_approval) = 'object' AND length(CAST(retention_approval AS BLOB)) BETWEEN 1 AND 65536),
 retention_approval_sha256 TEXT NOT NULL,
 CHECK (length(binding_sha256) = 64 AND binding_sha256 NOT GLOB '*[^0-9a-f]*'),
 CHECK (length(task_artifact_sha256) = 64 AND task_artifact_sha256 NOT GLOB '*[^0-9a-f]*'),
 CHECK (length(retention_policy_sha256) = 64 AND retention_policy_sha256 NOT GLOB '*[^0-9a-f]*'),
 CHECK (length(retention_approval_sha256) = 64 AND retention_approval_sha256 NOT GLOB '*[^0-9a-f]*')
) STRICT;
CREATE TRIGGER service_admission_bindings_identity BEFORE INSERT ON service_admission_bindings
WHEN NOT EXISTS (SELECT 1 FROM prepared_runs p WHERE p.preparation_id = NEW.preparation_id AND p.peer_id = NEW.peer_id AND p.generation = NEW.generation AND p.binding_sha256 = NEW.binding_sha256 AND p.expires_at = NEW.expires_at)
 OR json_extract(NEW.canonical_binding, '$.SchemaVersion') IS NOT NEW.codec_version
 OR json_extract(NEW.canonical_binding, '$.PreparationID') IS NOT NEW.preparation_id
 OR json_extract(NEW.canonical_binding, '$.RootID') IS NOT NEW.root_id
 OR json_extract(NEW.canonical_binding, '$.PeerID') IS NOT NEW.peer_id
 OR json_extract(NEW.canonical_binding, '$.Generation') IS NOT NEW.generation
 OR json_extract(NEW.canonical_binding, '$.ContextEpoch') IS NOT NEW.context_epoch
 OR json_extract(NEW.canonical_binding, '$.PolicyRevision') IS NOT NEW.policy_revision
 OR json_extract(NEW.canonical_binding, '$.ExpiresAt') IS NOT NEW.expires_at
 OR json_extract(NEW.canonical_binding, '$.Decision.Task.SHA256') IS NOT NEW.task_artifact_sha256
 OR json_extract(NEW.canonical_binding, '$.Decision.Task.ArtifactRef') IS NOT NEW.task_artifact_ref
 OR json_extract(NEW.retention_policy, '$.Content.RootID') IS NOT NEW.root_id
 OR json_extract(NEW.retention_policy, '$.Revision') IS NOT NEW.policy_revision
 OR json_extract(NEW.retention_approval, '$.PolicyRevision') IS NOT NEW.policy_revision
 OR json_extract(NEW.retention_approval, '$.PreparationID') IS NOT NEW.preparation_id
 OR json_extract(NEW.retention_approval, '$.BindingSHA256') IS NOT NEW.binding_sha256
 OR json_extract(NEW.retention_approval, '$.RootID') IS NOT NEW.root_id
 OR json_extract(NEW.retention_approval, '$.PeerID') IS NOT NEW.peer_id
 OR json_extract(NEW.retention_approval, '$.Generation') IS NOT NEW.generation
 OR json_extract(NEW.retention_approval, '$.ContextEpoch') IS NOT NEW.context_epoch
 OR json_extract(NEW.retention_approval, '$.ExpiresAt') IS NOT NEW.expires_at
 OR json_extract(NEW.retention_approval, '$.Decision') IS NOT 'approve'
BEGIN SELECT RAISE(ABORT, 'service admission binding identity mismatch'); END;
CREATE TRIGGER prepared_runs_binding_identity BEFORE UPDATE ON prepared_runs
WHEN EXISTS (SELECT 1 FROM service_admission_bindings WHERE preparation_id = OLD.preparation_id)
 AND (NEW.rowid IS NOT OLD.rowid OR NEW.preparation_id IS NOT OLD.preparation_id OR NEW.peer_id IS NOT OLD.peer_id OR NEW.generation IS NOT OLD.generation OR NEW.binding_sha256 IS NOT OLD.binding_sha256 OR NEW.spec IS NOT OLD.spec OR NEW.challenges IS NOT OLD.challenges OR NEW.expires_at IS NOT OLD.expires_at)
BEGIN SELECT RAISE(ABORT, 'retained preparation identity is immutable'); END;
-- Retained parents must survive implicit OR REPLACE deletion too. Unbound
-- historical preparations retain their released behavior.
CREATE TRIGGER prepared_runs_retained_no_replace BEFORE INSERT ON prepared_runs
WHEN EXISTS (SELECT 1 FROM prepared_runs p JOIN service_admission_bindings b USING (preparation_id)
 WHERE p.preparation_id = NEW.preparation_id OR p.rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'retained preparation replacement'); END;
CREATE TRIGGER prepared_runs_retained_victim BEFORE UPDATE ON prepared_runs
WHEN EXISTS (SELECT 1 FROM prepared_runs p JOIN service_admission_bindings b USING (preparation_id)
 WHERE p.rowid <> OLD.rowid AND (p.preparation_id = NEW.preparation_id OR p.rowid = NEW.rowid))
BEGIN SELECT RAISE(ABORT, 'retained preparation victim replacement'); END;
CREATE TABLE service_admission_consumptions (
 preparation_id TEXT PRIMARY KEY REFERENCES service_admission_bindings(preparation_id),
 operation_id TEXT NOT NULL UNIQUE REFERENCES operations(operation_id) DEFERRABLE INITIALLY DEFERRED,
 run_id TEXT NOT NULL UNIQUE REFERENCES runs(run_id),
 attempt_id TEXT NOT NULL UNIQUE,
 consumed_generation INTEGER NOT NULL CHECK (consumed_generation > 0),
 binding_sha256 TEXT NOT NULL CHECK (length(binding_sha256) = 64 AND binding_sha256 NOT GLOB '*[^0-9a-f]*')
) STRICT;
CREATE TRIGGER service_admission_consumptions_identity BEFORE INSERT ON service_admission_consumptions
WHEN NOT EXISTS (SELECT 1 FROM service_admission_bindings b JOIN prepared_runs p USING (preparation_id)
 WHERE b.preparation_id = NEW.preparation_id AND b.generation = NEW.consumed_generation AND b.binding_sha256 = NEW.binding_sha256 AND p.ready = 1 AND p.consumed = 0
 AND json_extract(b.canonical_binding, '$.RunID') = NEW.run_id AND json_extract(b.canonical_binding, '$.AttemptID') = NEW.attempt_id)
 OR NOT EXISTS (SELECT 1 FROM operations WHERE operation_id = NEW.operation_id AND method = 'run.start' AND object = NEW.run_id AND state = 'claimed')
BEGIN SELECT RAISE(ABORT, 'service admission consumption mismatch'); END;
-- A consumed operation's identity is pinned while its acknowledgement may
-- still finish in this transaction. Completed insertion replay keeps the
-- released schema9 IGNORE behavior; unlinked claimed rows remain mutable.
CREATE TRIGGER operations_consumed_no_replace BEFORE INSERT ON operations
WHEN EXISTS (SELECT 1 FROM operations o JOIN service_admission_consumptions c USING (operation_id)
 WHERE o.state = 'claimed' AND (o.operation_id = NEW.operation_id OR o.rowid = NEW.rowid))
BEGIN SELECT RAISE(ABORT, 'consumed operation replacement'); END;
CREATE TRIGGER operations_consumed_identity BEFORE UPDATE ON operations
WHEN EXISTS (SELECT 1 FROM service_admission_consumptions WHERE operation_id = OLD.operation_id)
 AND (NEW.rowid IS NOT OLD.rowid OR NEW.operation_id IS NOT OLD.operation_id OR NEW.method IS NOT OLD.method OR NEW.object IS NOT OLD.object OR NEW.digest IS NOT OLD.digest OR NEW.claimed_at IS NOT OLD.claimed_at)
BEGIN SELECT RAISE(ABORT, 'consumed operation identity is immutable'); END;
CREATE TRIGGER operations_consumed_victim BEFORE UPDATE ON operations
WHEN EXISTS (SELECT 1 FROM operations o JOIN service_admission_consumptions c USING (operation_id)
 WHERE o.rowid <> OLD.rowid AND (o.operation_id = NEW.operation_id OR o.rowid = NEW.rowid))
BEGIN SELECT RAISE(ABORT, 'consumed operation victim replacement'); END;
CREATE TRIGGER operations_consumed_no_delete BEFORE DELETE ON operations
WHEN EXISTS (SELECT 1 FROM service_admission_consumptions WHERE operation_id = OLD.operation_id)
BEGIN SELECT RAISE(ABORT, 'consumed operation is retained'); END;
CREATE TRIGGER prepared_runs_retained_no_delete BEFORE DELETE ON prepared_runs
WHEN EXISTS (SELECT 1 FROM service_admission_bindings WHERE preparation_id = OLD.preparation_id)
BEGIN SELECT RAISE(ABORT, 'retained preparation is immutable'); END;
CREATE TABLE service_pending_dispatch (
 operation_id TEXT PRIMARY KEY REFERENCES service_admission_consumptions(operation_id),
 preparation_id TEXT NOT NULL UNIQUE REFERENCES service_admission_bindings(preparation_id),
 run_id TEXT NOT NULL UNIQUE REFERENCES runs(run_id),
 attempt_id TEXT NOT NULL UNIQUE,
 binding_sha256 TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK (generation > 0),
 state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','armed','unconfirmed','settled')),
 revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
) STRICT;
CREATE TRIGGER service_pending_dispatch_identity BEFORE INSERT ON service_pending_dispatch
WHEN NEW.state <> 'pending' OR NEW.revision <> 1 OR NOT EXISTS (SELECT 1 FROM service_admission_consumptions c
 WHERE c.operation_id = NEW.operation_id AND c.preparation_id = NEW.preparation_id AND c.run_id = NEW.run_id AND c.attempt_id = NEW.attempt_id AND c.binding_sha256 = NEW.binding_sha256 AND c.consumed_generation = NEW.generation)
BEGIN SELECT RAISE(ABORT, 'service pending dispatch mismatch'); END;
CREATE TABLE service_dispatch_claims (
 operation_id TEXT PRIMARY KEY REFERENCES dispatch_outbox(operation_id),
 claim_generation INTEGER NOT NULL CHECK (claim_generation > 0),
 state TEXT NOT NULL DEFAULT 'claimed' CHECK (state IN ('claimed','spawn_intent','identified','unconfirmed','settled')),
 revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
 snapshot_event_id TEXT NOT NULL REFERENCES journal(event_id),
 launch_token_sha256 TEXT NOT NULL CHECK (length(launch_token_sha256) = 64 AND launch_token_sha256 NOT GLOB '*[^0-9a-f]*'),
 nonce_sha256 TEXT NOT NULL CHECK (length(nonce_sha256) = 64 AND nonce_sha256 NOT GLOB '*[^0-9a-f]*'),
 ladder_version TEXT NOT NULL CHECK (ladder_version <> ''),
 execution_deadline TEXT NOT NULL CHECK (execution_deadline <> ''),
 worker_pid INTEGER CHECK (worker_pid > 0),
 worker_start_time TEXT,
 identifying_event_id TEXT REFERENCES journal(event_id),
 CHECK ((worker_pid IS NULL AND worker_start_time IS NULL AND identifying_event_id IS NULL) OR (worker_pid IS NOT NULL AND worker_start_time IS NOT NULL AND worker_start_time <> '' AND identifying_event_id IS NOT NULL))
) STRICT;
CREATE TRIGGER service_dispatch_claims_identity BEFORE INSERT ON service_dispatch_claims
WHEN NEW.state <> 'claimed' OR NEW.revision <> 1 OR NEW.worker_pid IS NOT NULL
 OR NOT EXISTS (SELECT 1 FROM service_pending_dispatch p JOIN dispatch_outbox d USING (operation_id) JOIN attempts a ON a.attempt_id = p.attempt_id JOIN journal j ON j.event_id = NEW.snapshot_event_id
 WHERE p.operation_id = NEW.operation_id AND p.state = 'pending' AND p.generation = NEW.claim_generation
 AND d.preparation_id = p.preparation_id AND d.run_id = p.run_id AND d.attempt_id = p.attempt_id AND d.binding_sha256 = p.binding_sha256 AND d.generation = p.generation
 AND a.run_id = p.run_id AND a.launch_token_sha256 = NEW.launch_token_sha256 AND d.launch_token_sha256 = NEW.launch_token_sha256
 AND j.run_id = p.run_id AND j.type = 'workspace.snapshot_created')
BEGIN SELECT RAISE(ABORT, 'service dispatch claim mismatch'); END;
CREATE TRIGGER service_dispatch_claims_activation BEFORE UPDATE ON service_dispatch_claims
WHEN NEW.state IN ('spawn_intent','identified') AND NOT EXISTS
 (SELECT 1 FROM service_pending_dispatch WHERE operation_id = NEW.operation_id AND state IN ('armed','unconfirmed'))
BEGIN SELECT RAISE(ABORT, 'dispatch intent is not armed'); END;
CREATE TRIGGER service_dispatch_claims_evidence BEFORE UPDATE ON service_dispatch_claims
WHEN (NEW.state = 'identified' AND NOT EXISTS (SELECT 1 FROM service_pending_dispatch p JOIN attempts a ON a.attempt_id = p.attempt_id JOIN journal j ON j.event_id = NEW.identifying_event_id
 WHERE p.operation_id = NEW.operation_id AND a.run_id = p.run_id AND a.launch_token_sha256 = NEW.launch_token_sha256 AND a.worker_pid = NEW.worker_pid AND a.worker_start_time = NEW.worker_start_time
 AND j.run_id = p.run_id AND j.attempt_id = p.attempt_id AND j.type = 'attempt.launched' AND j.generation = NEW.claim_generation))
 OR ((NEW.worker_pid IS NOT OLD.worker_pid OR NEW.worker_start_time IS NOT OLD.worker_start_time OR NEW.identifying_event_id IS NOT OLD.identifying_event_id) AND (OLD.worker_pid IS NOT NULL OR NEW.state <> 'identified'))
BEGIN SELECT RAISE(ABORT, 'service dispatch identity evidence mismatch'); END;
CREATE TRIGGER service_pending_dispatch_armed BEFORE UPDATE ON service_pending_dispatch
WHEN NEW.state = 'armed' AND NOT EXISTS (SELECT 1 FROM service_dispatch_claims WHERE operation_id = NEW.operation_id)
BEGIN SELECT RAISE(ABORT, 'arming requires a durable dispatch claim'); END;

CREATE TRIGGER service_admission_bindings_no_replace BEFORE INSERT ON service_admission_bindings
WHEN EXISTS (SELECT 1 FROM service_admission_bindings WHERE rowid = NEW.rowid OR preparation_id = NEW.preparation_id)
BEGIN SELECT RAISE(ABORT, 'service_admission_bindings identity already exists'); END;

CREATE TRIGGER service_admission_bindings_immutable BEFORE UPDATE ON service_admission_bindings
WHEN 1
BEGIN SELECT RAISE(ABORT, 'service_admission_bindings identity is immutable'); END;

CREATE TRIGGER service_admission_bindings_no_delete BEFORE DELETE ON service_admission_bindings
BEGIN SELECT RAISE(ABORT, 'service_admission_bindings is retained'); END;

CREATE TRIGGER service_admission_bindings_revision_insert AFTER INSERT ON service_admission_bindings
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id = json_extract(NEW.canonical_binding, '$.RunID'); END;

CREATE TRIGGER service_admission_consumptions_no_replace BEFORE INSERT ON service_admission_consumptions
WHEN EXISTS (SELECT 1 FROM service_admission_consumptions WHERE rowid = NEW.rowid OR preparation_id = NEW.preparation_id OR operation_id = NEW.operation_id OR run_id = NEW.run_id OR attempt_id = NEW.attempt_id)
BEGIN SELECT RAISE(ABORT, 'service_admission_consumptions identity already exists'); END;

CREATE TRIGGER service_admission_consumptions_immutable BEFORE UPDATE ON service_admission_consumptions
WHEN 1
BEGIN SELECT RAISE(ABORT, 'service_admission_consumptions identity is immutable'); END;

CREATE TRIGGER service_admission_consumptions_no_delete BEFORE DELETE ON service_admission_consumptions
BEGIN SELECT RAISE(ABORT, 'service_admission_consumptions is retained'); END;

CREATE TRIGGER service_admission_consumptions_revision_insert AFTER INSERT ON service_admission_consumptions
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id = NEW.run_id; END;

CREATE TRIGGER service_pending_dispatch_no_replace BEFORE INSERT ON service_pending_dispatch
WHEN EXISTS (SELECT 1 FROM service_pending_dispatch WHERE rowid = NEW.rowid OR operation_id = NEW.operation_id OR preparation_id = NEW.preparation_id OR run_id = NEW.run_id OR attempt_id = NEW.attempt_id)
BEGIN SELECT RAISE(ABORT, 'service_pending_dispatch identity already exists'); END;

CREATE TRIGGER service_pending_dispatch_immutable BEFORE UPDATE ON service_pending_dispatch
WHEN NEW.rowid IS NOT OLD.rowid OR NEW.operation_id IS NOT OLD.operation_id OR NEW.preparation_id IS NOT OLD.preparation_id OR NEW.run_id IS NOT OLD.run_id OR NEW.attempt_id IS NOT OLD.attempt_id OR NEW.binding_sha256 IS NOT OLD.binding_sha256 OR NEW.generation IS NOT OLD.generation
BEGIN SELECT RAISE(ABORT, 'service_pending_dispatch identity is immutable'); END;

CREATE TRIGGER service_pending_dispatch_no_delete BEFORE DELETE ON service_pending_dispatch
BEGIN SELECT RAISE(ABORT, 'service_pending_dispatch is retained'); END;

CREATE TRIGGER service_pending_dispatch_revision_insert AFTER INSERT ON service_pending_dispatch
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id = NEW.run_id; END;

CREATE TRIGGER service_pending_dispatch_evolution BEFORE UPDATE ON service_pending_dispatch
WHEN (NEW.state IS NOT OLD.state AND NOT ((OLD.state = 'pending' AND NEW.state = 'armed') OR (OLD.state = 'pending' AND NEW.state = 'unconfirmed') OR (OLD.state = 'pending' AND NEW.state = 'settled') OR (OLD.state = 'armed' AND NEW.state = 'unconfirmed') OR (OLD.state = 'armed' AND NEW.state = 'settled') OR (OLD.state = 'unconfirmed' AND NEW.state = 'settled')))
 OR ((NEW.state IS NOT OLD.state) AND NEW.revision <> OLD.revision + 1)
 OR (NOT (NEW.state IS NOT OLD.state) AND NEW.revision <> OLD.revision)
BEGIN SELECT RAISE(ABORT, 'service_pending_dispatch invalid transition'); END;

CREATE TRIGGER service_pending_dispatch_revision_update AFTER UPDATE ON service_pending_dispatch
WHEN NEW.state IS NOT OLD.state
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id = NEW.run_id; END;

CREATE TRIGGER service_dispatch_claims_no_replace BEFORE INSERT ON service_dispatch_claims
WHEN EXISTS (SELECT 1 FROM service_dispatch_claims WHERE rowid = NEW.rowid OR operation_id = NEW.operation_id)
BEGIN SELECT RAISE(ABORT, 'service_dispatch_claims identity already exists'); END;

CREATE TRIGGER service_dispatch_claims_immutable BEFORE UPDATE ON service_dispatch_claims
WHEN NEW.rowid IS NOT OLD.rowid OR NEW.operation_id IS NOT OLD.operation_id OR NEW.claim_generation IS NOT OLD.claim_generation OR NEW.snapshot_event_id IS NOT OLD.snapshot_event_id OR NEW.launch_token_sha256 IS NOT OLD.launch_token_sha256 OR NEW.nonce_sha256 IS NOT OLD.nonce_sha256 OR NEW.ladder_version IS NOT OLD.ladder_version OR NEW.execution_deadline IS NOT OLD.execution_deadline
BEGIN SELECT RAISE(ABORT, 'service_dispatch_claims identity is immutable'); END;

CREATE TRIGGER service_dispatch_claims_no_delete BEFORE DELETE ON service_dispatch_claims
BEGIN SELECT RAISE(ABORT, 'service_dispatch_claims is retained'); END;

CREATE TRIGGER service_dispatch_claims_revision_insert AFTER INSERT ON service_dispatch_claims
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id = (SELECT run_id FROM service_pending_dispatch WHERE operation_id = NEW.operation_id); END;

CREATE TRIGGER service_dispatch_claims_evolution BEFORE UPDATE ON service_dispatch_claims
WHEN (NEW.state IS NOT OLD.state AND NOT ((OLD.state = 'claimed' AND NEW.state = 'spawn_intent') OR (OLD.state = 'claimed' AND NEW.state = 'unconfirmed') OR (OLD.state = 'claimed' AND NEW.state = 'settled') OR (OLD.state = 'spawn_intent' AND NEW.state = 'identified') OR (OLD.state = 'spawn_intent' AND NEW.state = 'unconfirmed') OR (OLD.state = 'spawn_intent' AND NEW.state = 'settled') OR (OLD.state = 'identified' AND NEW.state = 'unconfirmed') OR (OLD.state = 'identified' AND NEW.state = 'settled') OR (OLD.state = 'unconfirmed' AND NEW.state = 'identified') OR (OLD.state = 'unconfirmed' AND NEW.state = 'settled')))
 OR ((NEW.state IS NOT OLD.state OR NEW.worker_pid IS NOT OLD.worker_pid OR NEW.worker_start_time IS NOT OLD.worker_start_time OR NEW.identifying_event_id IS NOT OLD.identifying_event_id) AND NEW.revision <> OLD.revision + 1)
 OR (NOT (NEW.state IS NOT OLD.state OR NEW.worker_pid IS NOT OLD.worker_pid OR NEW.worker_start_time IS NOT OLD.worker_start_time OR NEW.identifying_event_id IS NOT OLD.identifying_event_id) AND NEW.revision <> OLD.revision)
BEGIN SELECT RAISE(ABORT, 'service_dispatch_claims invalid transition'); END;

CREATE TRIGGER service_dispatch_claims_revision_update AFTER UPDATE ON service_dispatch_claims
WHEN NEW.state IS NOT OLD.state OR NEW.worker_pid IS NOT OLD.worker_pid OR NEW.worker_start_time IS NOT OLD.worker_start_time OR NEW.identifying_event_id IS NOT OLD.identifying_event_id
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id = (SELECT run_id FROM service_pending_dispatch WHERE operation_id = NEW.operation_id); END;

UPDATE migration_state SET schema_version = 11 WHERE id = 1;
