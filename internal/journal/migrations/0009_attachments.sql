-- Schema v9 (supervisor-attachment-control design section 5). Additive only.
-- Raw bearer tokens and caller environments have no storage column.
CREATE TABLE attachment_grants (
 attachment_id TEXT PRIMARY KEY,
 token_sha256 TEXT NOT NULL UNIQUE,
 client_id TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 repository_id TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK (generation > 0),
 revocation_revision INTEGER NOT NULL DEFAULT 0 CHECK (revocation_revision >= 0),
 revoked INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0,1))
) STRICT;
CREATE TABLE run_control_revisions (
 run_id TEXT PRIMARY KEY REFERENCES runs(run_id) ON UPDATE CASCADE ON DELETE CASCADE,
 revision INTEGER NOT NULL CHECK (revision > 0)
) STRICT;
-- Legacy runs start with a known revision, without changing any legacy value.
INSERT INTO run_control_revisions SELECT run_id, 1 FROM runs;
CREATE TABLE attachment_projections (
 attachment_id TEXT NOT NULL REFERENCES attachment_grants(attachment_id),
 generation INTEGER NOT NULL CHECK (generation > 0),
 projection_revision INTEGER NOT NULL CHECK (projection_revision > 0),
 source_run_revision INTEGER NOT NULL CHECK (source_run_revision > 0),
 content_sha256 TEXT NOT NULL,
 content TEXT NOT NULL CHECK (json_valid(content)),
 PRIMARY KEY (attachment_id, generation)
) STRICT;
CREATE TABLE stop_outbox (
 operation_id TEXT PRIMARY KEY REFERENCES operations(operation_id) DEFERRABLE INITIALLY DEFERRED,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 attempt_id TEXT NOT NULL REFERENCES attempts(attempt_id),
 launch_token_sha256 TEXT NOT NULL,
 worker_pid INTEGER NOT NULL CHECK (worker_pid > 0),
 worker_start_time TEXT NOT NULL CHECK (worker_start_time <> ''),
 accepted_generation INTEGER NOT NULL CHECK (accepted_generation > 0),
 delivery_state TEXT NOT NULL DEFAULT 'pending' CHECK (delivery_state IN ('pending','delivered','coalesced','unconfirmed'))
) STRICT;
CREATE TRIGGER stop_outbox_identity BEFORE INSERT ON stop_outbox
WHEN NOT EXISTS (SELECT 1 FROM attempts a WHERE a.attempt_id = NEW.attempt_id AND a.run_id = NEW.run_id
 AND a.launch_token_sha256 = NEW.launch_token_sha256 AND a.worker_pid = NEW.worker_pid
 AND a.worker_start_time = NEW.worker_start_time)
BEGIN SELECT RAISE(ABORT, 'stop outbox identity mismatch'); END;
CREATE TRIGGER stop_outbox_no_replace BEFORE INSERT ON stop_outbox
WHEN EXISTS (SELECT 1 FROM stop_outbox WHERE operation_id = NEW.operation_id)
BEGIN SELECT RAISE(ABORT, 'stop outbox identity is immutable'); END;
CREATE TRIGGER stop_outbox_identity_no_update BEFORE UPDATE ON stop_outbox
WHEN NEW.operation_id IS NOT OLD.operation_id OR NEW.run_id IS NOT OLD.run_id
 OR NEW.attempt_id IS NOT OLD.attempt_id OR NEW.launch_token_sha256 IS NOT OLD.launch_token_sha256
 OR NEW.worker_pid IS NOT OLD.worker_pid OR NEW.worker_start_time IS NOT OLD.worker_start_time
 OR NEW.accepted_generation IS NOT OLD.accepted_generation
BEGIN SELECT RAISE(ABORT, 'stop outbox identity is immutable'); END;
CREATE TRIGGER stop_outbox_no_delete BEFORE DELETE ON stop_outbox
BEGIN SELECT RAISE(ABORT, 'stop outbox identity is immutable'); END;
CREATE TABLE stop_results (
 operation_id TEXT PRIMARY KEY REFERENCES stop_outbox(operation_id),
 state TEXT NOT NULL CHECK (state IN ('requested','unconfirmed','confirmed')),
 reason TEXT NOT NULL DEFAULT '' CHECK (reason IN ('', 'stop_unconfirmed', 'invalid_contract', 'revision_conflict', 'dependency_stale', 'auth_unavailable', 'entitlement_unknown', 'entitlement_ineligible', 'allowance_exhausted', 'capability_unsupported', 'permission_denied', 'provider_throttled', 'protocol_mismatch', 'process_lost', 'ownership_unresolved', 'tool_failed', 'candidate_rejected', 'integration_conflict', 'verification_failed', 'verification_unavailable', 'persistence_unavailable', 'cancel_incomplete', 'external_effect_uncertain', 'budget_exhausted', 'policy_ineligible', 'schema_too_new')),
 confirming_event_id TEXT REFERENCES journal(event_id),
 result_revision INTEGER NOT NULL CHECK (result_revision > 0),
 CHECK ((state = 'confirmed') = (confirming_event_id IS NOT NULL))
) STRICT;
CREATE TRIGGER stop_results_initial BEFORE INSERT ON stop_results
WHEN NEW.state <> 'requested' OR NEW.result_revision <> 1
BEGIN SELECT RAISE(ABORT, 'stop result must begin requested at revision 1'); END;
CREATE TRIGGER stop_results_no_replace BEFORE INSERT ON stop_results
WHEN EXISTS (SELECT 1 FROM stop_results WHERE operation_id = NEW.operation_id)
BEGIN SELECT RAISE(ABORT, 'stop result already exists'); END;
CREATE TRIGGER stop_results_evolution BEFORE UPDATE ON stop_results
WHEN NEW.operation_id IS NOT OLD.operation_id
 OR (NEW.state IS NOT OLD.state AND NOT (
   (OLD.state = 'requested' AND NEW.state IN ('unconfirmed','confirmed'))
   OR (OLD.state = 'unconfirmed' AND NEW.state = 'confirmed')))
 OR (OLD.state = 'confirmed' AND (NEW.reason IS NOT OLD.reason OR NEW.confirming_event_id IS NOT OLD.confirming_event_id))
 OR ((NEW.state IS NOT OLD.state OR NEW.reason IS NOT OLD.reason OR NEW.confirming_event_id IS NOT OLD.confirming_event_id)
     AND NEW.result_revision <> OLD.result_revision + 1)
 OR ((NEW.state IS OLD.state AND NEW.reason IS OLD.reason AND NEW.confirming_event_id IS OLD.confirming_event_id)
     AND NEW.result_revision <> OLD.result_revision)
BEGIN SELECT RAISE(ABORT, 'invalid stop result evolution'); END;
CREATE TRIGGER stop_results_confirm BEFORE UPDATE ON stop_results
WHEN NEW.state = 'confirmed' AND NOT EXISTS (
 SELECT 1 FROM journal j JOIN stop_outbox o ON o.operation_id = NEW.operation_id
 WHERE j.event_id = NEW.confirming_event_id AND j.run_id = o.run_id AND j.attempt_id = o.attempt_id
 AND j.type = 'attempt.stopped' AND json_type(j.payload, '$.confirmed') = 'true'
 AND json_type(j.payload, '$.unresolved_pids') = 'array' AND json_array_length(j.payload, '$.unresolved_pids') = 0)
BEGIN SELECT RAISE(ABORT, 'stop confirmation requires matching evidence'); END;
CREATE TRIGGER stop_results_no_delete BEFORE DELETE ON stop_results
BEGIN SELECT RAISE(ABORT, 'stop result is retained'); END;
-- Typed service admission producers own validation of these JSON records.
-- Only the closed RunSpec and challenge/confirmation records belong here.
CREATE TABLE prepared_runs (
 preparation_id TEXT PRIMARY KEY,
 peer_id TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK (generation > 0),
 binding_sha256 TEXT NOT NULL,
 revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
 spec TEXT NOT NULL CHECK (json_valid(spec)),
 challenges TEXT NOT NULL CHECK (json_valid(challenges)),
 confirmations TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(confirmations)),
 ready INTEGER NOT NULL DEFAULT 0 CHECK (ready IN (0,1)),
 consumed INTEGER NOT NULL DEFAULT 0 CHECK (consumed IN (0,1)),
 expires_at TEXT NOT NULL
) STRICT;
CREATE TABLE dispatch_outbox (
 operation_id TEXT PRIMARY KEY REFERENCES operations(operation_id) DEFERRABLE INITIALLY DEFERRED,
 preparation_id TEXT NOT NULL UNIQUE REFERENCES prepared_runs(preparation_id),
 peer_id TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK (generation > 0),
 binding_sha256 TEXT NOT NULL,
 admitted_evidence_sha256 TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 attempt_id TEXT NOT NULL UNIQUE REFERENCES attempts(attempt_id),
 launch_token_sha256 TEXT NOT NULL,
 delivery_state TEXT NOT NULL DEFAULT 'pending' CHECK (delivery_state IN ('pending','dispatched','unconfirmed'))
) STRICT;
CREATE TRIGGER dispatch_outbox_identity BEFORE INSERT ON dispatch_outbox
WHEN NOT EXISTS (SELECT 1 FROM attempts a WHERE a.attempt_id = NEW.attempt_id AND a.run_id = NEW.run_id
 AND a.launch_token_sha256 = NEW.launch_token_sha256)
 OR NOT EXISTS (SELECT 1 FROM prepared_runs p WHERE p.preparation_id = NEW.preparation_id
 AND p.peer_id = NEW.peer_id AND p.generation = NEW.generation AND p.binding_sha256 = NEW.binding_sha256)
BEGIN SELECT RAISE(ABORT, 'dispatch binding mismatch'); END;
CREATE TRIGGER dispatch_outbox_no_replace BEFORE INSERT ON dispatch_outbox
WHEN EXISTS (SELECT 1 FROM dispatch_outbox WHERE operation_id = NEW.operation_id
 OR preparation_id = NEW.preparation_id OR attempt_id = NEW.attempt_id)
BEGIN SELECT RAISE(ABORT, 'dispatch identity is immutable'); END;
CREATE TRIGGER dispatch_outbox_identity_no_update BEFORE UPDATE ON dispatch_outbox
WHEN NEW.operation_id IS NOT OLD.operation_id OR NEW.preparation_id IS NOT OLD.preparation_id
 OR NEW.peer_id IS NOT OLD.peer_id OR NEW.generation IS NOT OLD.generation
 OR NEW.binding_sha256 IS NOT OLD.binding_sha256 OR NEW.admitted_evidence_sha256 IS NOT OLD.admitted_evidence_sha256
 OR NEW.run_id IS NOT OLD.run_id OR NEW.attempt_id IS NOT OLD.attempt_id
 OR NEW.launch_token_sha256 IS NOT OLD.launch_token_sha256
BEGIN SELECT RAISE(ABORT, 'dispatch identity is immutable'); END;
CREATE TRIGGER dispatch_outbox_no_delete BEFORE DELETE ON dispatch_outbox
BEGIN SELECT RAISE(ABORT, 'dispatch identity is immutable'); END;

-- A concurrent long-operation claimant may INSERT ... ON CONFLICT DO NOTHING
-- after a winner completed. Ignore every duplicate insertion of a completed
-- operation (including OR REPLACE), preserving the entire immutable row and
-- RowsAffected=0. Existing completed UPDATE/DELETE guards remain unchanged.
CREATE TRIGGER operations_done_no_replace BEFORE INSERT ON operations
WHEN EXISTS (SELECT 1 FROM operations WHERE (operation_id = NEW.operation_id OR rowid = NEW.rowid) AND state = 'done')
BEGIN SELECT RAISE(IGNORE); END;

-- SQLite OR REPLACE can delete a conflicting row without running DELETE
-- triggers when recursive_triggers is off. Refuse duplicate identities before
-- insertion instead; normal writers use INSERT/UPDATE and journal dedup checks
-- happen before INSERT. Cover every UNIQUE key, including journal aliases.
CREATE TRIGGER attempts_no_replace BEFORE INSERT ON attempts
WHEN EXISTS (SELECT 1 FROM attempts WHERE attempt_id = NEW.attempt_id OR (run_id = NEW.run_id AND task_id = NEW.task_id AND attempt_number = NEW.attempt_number))
BEGIN SELECT RAISE(ABORT, 'attempts identity already exists'); END;
CREATE TRIGGER journal_no_replace BEFORE INSERT ON journal
WHEN EXISTS (SELECT 1 FROM journal WHERE seq = NEW.seq OR event_id = NEW.event_id OR (producer_id = NEW.producer_id AND producer_sequence = NEW.producer_sequence) OR (run_id = NEW.run_id AND run_sequence = NEW.run_sequence))
BEGIN SELECT RAISE(ABORT, 'journal identity already exists'); END;
CREATE TRIGGER candidates_no_replace BEFORE INSERT ON candidates
WHEN EXISTS (SELECT 1 FROM candidates WHERE attempt_id = NEW.attempt_id)
BEGIN SELECT RAISE(ABORT, 'candidates identity already exists'); END;
CREATE TRIGGER verifications_no_replace BEFORE INSERT ON verifications
WHEN EXISTS (SELECT 1 FROM verifications WHERE verification_id = NEW.verification_id)
BEGIN SELECT RAISE(ABORT, 'verifications identity already exists'); END;
CREATE TRIGGER check_results_no_replace BEFORE INSERT ON check_results
WHEN EXISTS (SELECT 1 FROM check_results WHERE verification_id = NEW.verification_id AND name = NEW.name)
BEGIN SELECT RAISE(ABORT, 'check_results identity already exists'); END;
CREATE TRIGGER applies_no_replace BEFORE INSERT ON applies
WHEN EXISTS (SELECT 1 FROM applies WHERE apply_id = NEW.apply_id)
BEGIN SELECT RAISE(ABORT, 'applies identity already exists'); END;
CREATE TRIGGER run_assignments_no_replace BEFORE INSERT ON run_assignments
WHEN EXISTS (SELECT 1 FROM run_assignments WHERE run_id = NEW.run_id)
BEGIN SELECT RAISE(ABORT, 'run_assignments identity already exists'); END;

-- UPDATE OR REPLACE has the same implicit-delete behavior. Never allow a
-- mutable source row to collide with another identity; otherwise the victim's
-- run could disappear from revision maintenance. Exclude only the OLD row.
CREATE TRIGGER operations_done_no_update_collision BEFORE UPDATE ON operations
WHEN EXISTS (
 SELECT 1 FROM operations WHERE rowid IS NOT OLD.rowid
 AND (operation_id = NEW.operation_id OR rowid = NEW.rowid) AND state = 'done')
BEGIN SELECT RAISE(ABORT, 'a recorded operation identity is immutable'); END;
CREATE TRIGGER runs_no_update_collision BEFORE UPDATE ON runs
WHEN EXISTS (SELECT 1 FROM runs WHERE (run_id IS NOT OLD.run_id) AND (run_id = NEW.run_id))
BEGIN SELECT RAISE(ABORT, 'runs identity collision'); END;
CREATE TRIGGER attempts_no_update_collision BEFORE UPDATE ON attempts
WHEN EXISTS (SELECT 1 FROM attempts WHERE (attempt_id IS NOT OLD.attempt_id) AND (attempt_id = NEW.attempt_id OR (run_id = NEW.run_id AND task_id = NEW.task_id AND attempt_number = NEW.attempt_number)))
BEGIN SELECT RAISE(ABORT, 'attempts identity collision'); END;
CREATE TRIGGER candidates_no_update_collision BEFORE UPDATE ON candidates
WHEN EXISTS (SELECT 1 FROM candidates WHERE (attempt_id IS NOT OLD.attempt_id) AND (attempt_id = NEW.attempt_id))
BEGIN SELECT RAISE(ABORT, 'candidates identity collision'); END;
CREATE TRIGGER verifications_no_update_collision BEFORE UPDATE ON verifications
WHEN EXISTS (SELECT 1 FROM verifications WHERE (verification_id IS NOT OLD.verification_id) AND (verification_id = NEW.verification_id))
BEGIN SELECT RAISE(ABORT, 'verifications identity collision'); END;
CREATE TRIGGER check_results_no_update_collision BEFORE UPDATE ON check_results
WHEN EXISTS (SELECT 1 FROM check_results WHERE (NOT (verification_id = OLD.verification_id AND name = OLD.name)) AND (verification_id = NEW.verification_id AND name = NEW.name))
BEGIN SELECT RAISE(ABORT, 'check_results identity collision'); END;
CREATE TRIGGER applies_no_update_collision BEFORE UPDATE ON applies
WHEN EXISTS (SELECT 1 FROM applies WHERE (apply_id IS NOT OLD.apply_id) AND (apply_id = NEW.apply_id))
BEGIN SELECT RAISE(ABORT, 'applies identity collision'); END;
CREATE TRIGGER run_assignments_no_update_collision BEFORE UPDATE ON run_assignments
WHEN EXISTS (SELECT 1 FROM run_assignments WHERE (run_id IS NOT OLD.run_id) AND (run_id = NEW.run_id))
BEGIN SELECT RAISE(ABORT, 'run_assignments identity collision'); END;

-- Hidden rowid aliases can also cause implicit deletion independently of
-- declared keys. BEFORE INSERT exposes -1 for ordinary auto-allocation; do
-- not skip that value, since an explicit negative collision must fail closed.
-- Production writers use positive auto-allocation (or journal's positive seq).
CREATE TRIGGER runs_no_rowid_replace BEFORE INSERT ON runs
WHEN EXISTS (SELECT 1 FROM runs WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'runs physical identity collision'); END;
CREATE TRIGGER runs_no_rowid_update_collision BEFORE UPDATE ON runs
WHEN EXISTS (SELECT 1 FROM runs WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'runs physical identity collision'); END;
CREATE TRIGGER attempts_no_rowid_replace BEFORE INSERT ON attempts
WHEN EXISTS (SELECT 1 FROM attempts WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'attempts physical identity collision'); END;
CREATE TRIGGER attempts_no_rowid_update_collision BEFORE UPDATE ON attempts
WHEN EXISTS (SELECT 1 FROM attempts WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'attempts physical identity collision'); END;
CREATE TRIGGER candidates_no_rowid_replace BEFORE INSERT ON candidates
WHEN EXISTS (SELECT 1 FROM candidates WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'candidates physical identity collision'); END;
CREATE TRIGGER candidates_no_rowid_update_collision BEFORE UPDATE ON candidates
WHEN EXISTS (SELECT 1 FROM candidates WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'candidates physical identity collision'); END;
CREATE TRIGGER verifications_no_rowid_replace BEFORE INSERT ON verifications
WHEN EXISTS (SELECT 1 FROM verifications WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'verifications physical identity collision'); END;
CREATE TRIGGER verifications_no_rowid_update_collision BEFORE UPDATE ON verifications
WHEN EXISTS (SELECT 1 FROM verifications WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'verifications physical identity collision'); END;
CREATE TRIGGER verification_bindings_no_rowid_replace BEFORE INSERT ON verification_bindings
WHEN EXISTS (SELECT 1 FROM verification_bindings WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'verification_bindings physical identity collision'); END;
CREATE TRIGGER check_results_no_rowid_replace BEFORE INSERT ON check_results
WHEN EXISTS (SELECT 1 FROM check_results WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'check_results physical identity collision'); END;
CREATE TRIGGER check_results_no_rowid_update_collision BEFORE UPDATE ON check_results
WHEN EXISTS (SELECT 1 FROM check_results WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'check_results physical identity collision'); END;
CREATE TRIGGER applies_no_rowid_replace BEFORE INSERT ON applies
WHEN EXISTS (SELECT 1 FROM applies WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'applies physical identity collision'); END;
CREATE TRIGGER applies_no_rowid_update_collision BEFORE UPDATE ON applies
WHEN EXISTS (SELECT 1 FROM applies WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'applies physical identity collision'); END;
CREATE TRIGGER run_assignments_no_rowid_replace BEFORE INSERT ON run_assignments
WHEN EXISTS (SELECT 1 FROM run_assignments WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'run_assignments physical identity collision'); END;
CREATE TRIGGER run_assignments_no_rowid_update_collision BEFORE UPDATE ON run_assignments
WHEN EXISTS (SELECT 1 FROM run_assignments WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'run_assignments physical identity collision'); END;
CREATE TRIGGER stop_results_no_rowid_replace BEFORE INSERT ON stop_results
WHEN EXISTS (SELECT 1 FROM stop_results WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'stop_results physical identity collision'); END;
CREATE TRIGGER stop_results_no_rowid_update_collision BEFORE UPDATE ON stop_results
WHEN EXISTS (SELECT 1 FROM stop_results WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'stop_results physical identity collision'); END;
CREATE TRIGGER stop_outbox_no_rowid_replace BEFORE INSERT ON stop_outbox
WHEN EXISTS (SELECT 1 FROM stop_outbox WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'stop_outbox physical identity collision'); END;
CREATE TRIGGER stop_outbox_no_rowid_update_collision BEFORE UPDATE ON stop_outbox
WHEN EXISTS (SELECT 1 FROM stop_outbox WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'stop_outbox physical identity collision'); END;
CREATE TRIGGER dispatch_outbox_no_rowid_replace BEFORE INSERT ON dispatch_outbox
WHEN EXISTS (SELECT 1 FROM dispatch_outbox WHERE rowid = NEW.rowid)
BEGIN SELECT RAISE(ABORT, 'dispatch_outbox physical identity collision'); END;
CREATE TRIGGER dispatch_outbox_no_rowid_update_collision BEFORE UPDATE ON dispatch_outbox
WHEN EXISTS (SELECT 1 FROM dispatch_outbox WHERE rowid = NEW.rowid AND rowid IS NOT OLD.rowid)
BEGIN SELECT RAISE(ABORT, 'dispatch_outbox physical identity collision'); END;

-- Revision inventory: all identity/state/evidence columns of the closed source
-- set below. runs timestamps and attempts.spool_offset are bookkeeping, not
-- selected data. In particular deduplicated spool acknowledgments are not a
-- canonical advance. BEFORE DELETE preserves indirect bindings until resolved.
CREATE TRIGGER runs_revision_no_replace BEFORE INSERT ON runs
WHEN EXISTS (SELECT 1 FROM runs WHERE run_id = NEW.run_id)
BEGIN SELECT RAISE(ABORT, 'run identity already exists'); END;
CREATE TRIGGER runs_revision_insert AFTER INSERT ON runs
BEGIN INSERT INTO run_control_revisions VALUES (NEW.run_id, 1); END;
CREATE TRIGGER runs_revision_update AFTER UPDATE ON runs
WHEN NEW.run_id IS NOT OLD.run_id OR NEW.state IS NOT OLD.state OR NEW.reason IS NOT OLD.reason OR NEW.adapter_id IS NOT OLD.adapter_id OR NEW.source_repo IS NOT OLD.source_repo OR NEW.source_branch IS NOT OLD.source_branch OR NEW.base_rev IS NOT OLD.base_rev OR NEW.task_sha256 IS NOT OLD.task_sha256 OR NEW.billing_posture IS NOT OLD.billing_posture OR NEW.execution_profile IS NOT OLD.execution_profile
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER runs_revision_delete BEFORE DELETE ON runs
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
CREATE TRIGGER attempts_revision_insert AFTER INSERT ON attempts
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (NEW.run_id); END;
CREATE TRIGGER attempts_revision_update AFTER UPDATE ON attempts
WHEN NEW.attempt_id IS NOT OLD.attempt_id OR NEW.run_id IS NOT OLD.run_id OR NEW.task_id IS NOT OLD.task_id OR NEW.attempt_number IS NOT OLD.attempt_number OR NEW.state IS NOT OLD.state OR NEW.reason IS NOT OLD.reason OR NEW.launch_token_sha256 IS NOT OLD.launch_token_sha256 OR NEW.worker_pid IS NOT OLD.worker_pid OR NEW.worker_start_time IS NOT OLD.worker_start_time OR NEW.native_pid IS NOT OLD.native_pid OR NEW.native_pgid IS NOT OLD.native_pgid OR NEW.native_session_id IS NOT OLD.native_session_id OR NEW.workspace_path IS NOT OLD.workspace_path
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER attempts_revision_delete BEFORE DELETE ON attempts
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
CREATE TRIGGER journal_revision_insert AFTER INSERT ON journal
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (NEW.run_id); END;
CREATE TRIGGER journal_revision_update AFTER UPDATE ON journal
WHEN NEW.seq IS NOT OLD.seq OR NEW.event_id IS NOT OLD.event_id OR NEW.schema_version IS NOT OLD.schema_version OR NEW.run_id IS NOT OLD.run_id OR NEW.task_id IS NOT OLD.task_id OR NEW.attempt_id IS NOT OLD.attempt_id OR NEW.producer_id IS NOT OLD.producer_id OR NEW.producer_sequence IS NOT OLD.producer_sequence OR NEW.run_sequence IS NOT OLD.run_sequence OR NEW.generation IS NOT OLD.generation OR NEW.caused_by IS NOT OLD.caused_by OR NEW.observed_at IS NOT OLD.observed_at OR NEW.type IS NOT OLD.type OR NEW.payload IS NOT OLD.payload
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER journal_revision_delete BEFORE DELETE ON journal
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
CREATE TRIGGER candidates_revision_insert AFTER INSERT ON candidates
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM attempts WHERE attempt_id = NEW.attempt_id)); END;
CREATE TRIGGER candidates_revision_update AFTER UPDATE ON candidates
WHEN NEW.attempt_id IS NOT OLD.attempt_id OR NEW.base_rev IS NOT OLD.base_rev OR NEW.candidate_commit IS NOT OLD.candidate_commit OR NEW.tree_id IS NOT OLD.tree_id OR NEW.patch_sha256 IS NOT OLD.patch_sha256 OR NEW.changed_paths IS NOT OLD.changed_paths OR NEW.flags IS NOT OLD.flags OR NEW.partial IS NOT OLD.partial
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM attempts WHERE attempt_id = OLD.attempt_id), (SELECT run_id FROM attempts WHERE attempt_id = NEW.attempt_id)); END;
CREATE TRIGGER candidates_revision_delete BEFORE DELETE ON candidates
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM attempts WHERE attempt_id = OLD.attempt_id)); END;
CREATE TRIGGER verifications_revision_insert AFTER INSERT ON verifications
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (NEW.run_id); END;
CREATE TRIGGER verifications_revision_update AFTER UPDATE ON verifications
WHEN NEW.verification_id IS NOT OLD.verification_id OR NEW.run_id IS NOT OLD.run_id OR NEW.candidate_commit IS NOT OLD.candidate_commit OR NEW.config_sha256 IS NOT OLD.config_sha256 OR NEW.result IS NOT OLD.result OR NEW.started_at IS NOT OLD.started_at OR NEW.finished_at IS NOT OLD.finished_at
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER verifications_revision_delete BEFORE DELETE ON verifications
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
CREATE TRIGGER verification_bindings_revision_insert AFTER INSERT ON verification_bindings
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM verifications WHERE verification_id = NEW.verification_id)); END;
CREATE TRIGGER verification_bindings_revision_update AFTER UPDATE ON verification_bindings
WHEN NEW.verification_id IS NOT OLD.verification_id OR NEW.reason IS NOT OLD.reason OR NEW.attestation IS NOT OLD.attestation OR NEW.digest IS NOT OLD.digest OR NEW.overhead_ms IS NOT OLD.overhead_ms
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM verifications WHERE verification_id = OLD.verification_id), (SELECT run_id FROM verifications WHERE verification_id = NEW.verification_id)); END;
CREATE TRIGGER verification_bindings_revision_delete BEFORE DELETE ON verification_bindings
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM verifications WHERE verification_id = OLD.verification_id)); END;
CREATE TRIGGER check_results_revision_insert AFTER INSERT ON check_results
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM verifications WHERE verification_id = NEW.verification_id)); END;
CREATE TRIGGER check_results_revision_update AFTER UPDATE ON check_results
WHEN NEW.verification_id IS NOT OLD.verification_id OR NEW.name IS NOT OLD.name OR NEW.argv IS NOT OLD.argv OR NEW.status IS NOT OLD.status OR NEW.exit_code IS NOT OLD.exit_code OR NEW.duration_ms IS NOT OLD.duration_ms OR NEW.evidence_path IS NOT OLD.evidence_path OR NEW.evidence_sha256 IS NOT OLD.evidence_sha256
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM verifications WHERE verification_id = OLD.verification_id), (SELECT run_id FROM verifications WHERE verification_id = NEW.verification_id)); END;
CREATE TRIGGER check_results_revision_delete BEFORE DELETE ON check_results
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM verifications WHERE verification_id = OLD.verification_id)); END;
CREATE TRIGGER applies_revision_insert AFTER INSERT ON applies
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (NEW.run_id); END;
CREATE TRIGGER applies_revision_update AFTER UPDATE ON applies
WHEN NEW.apply_id IS NOT OLD.apply_id OR NEW.run_id IS NOT OLD.run_id OR NEW.target_repo IS NOT OLD.target_repo OR NEW.branch IS NOT OLD.branch OR NEW.candidate_commit IS NOT OLD.candidate_commit OR NEW.state IS NOT OLD.state OR NEW.reason IS NOT OLD.reason OR NEW.created_at IS NOT OLD.created_at
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER applies_revision_delete BEFORE DELETE ON applies
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
CREATE TRIGGER run_assignments_revision_insert AFTER INSERT ON run_assignments
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (NEW.run_id); END;
CREATE TRIGGER run_assignments_revision_update AFTER UPDATE ON run_assignments
WHEN NEW.run_id IS NOT OLD.run_id OR NEW.owner IS NOT OLD.owner OR NEW.assigned_at IS NOT OLD.assigned_at OR NEW.operation_id IS NOT OLD.operation_id
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER run_assignments_revision_delete BEFORE DELETE ON run_assignments
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
CREATE TRIGGER stop_results_revision_insert AFTER INSERT ON stop_results
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM stop_outbox WHERE operation_id = NEW.operation_id)); END;
CREATE TRIGGER stop_results_revision_update AFTER UPDATE ON stop_results
WHEN NEW.operation_id IS NOT OLD.operation_id OR NEW.state IS NOT OLD.state OR NEW.reason IS NOT OLD.reason OR NEW.confirming_event_id IS NOT OLD.confirming_event_id OR NEW.result_revision IS NOT OLD.result_revision
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM stop_outbox WHERE operation_id = OLD.operation_id), (SELECT run_id FROM stop_outbox WHERE operation_id = NEW.operation_id)); END;
CREATE TRIGGER stop_results_revision_delete BEFORE DELETE ON stop_results
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN ((SELECT run_id FROM stop_outbox WHERE operation_id = OLD.operation_id)); END;
UPDATE migration_state SET schema_version = 9 WHERE id = 1;
