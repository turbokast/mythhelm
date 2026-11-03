-- Schema v10 (design section 3.2). Additive only: released run_assignments
-- keeps its four columns and deferred operation FK. No epoch is synthesized
-- for historical assignments; a missing epoch means unknown, never 1.
-- 0011 is reserved for the binding migration.
CREATE TABLE service_assignment_epochs (
 run_id TEXT NOT NULL REFERENCES run_assignments(run_id) DEFERRABLE INITIALLY DEFERRED,
 control_generation INTEGER NOT NULL CHECK (control_generation > 0),
 attempt_id TEXT NOT NULL REFERENCES attempts(attempt_id),
 adoption_event_id TEXT NOT NULL UNIQUE REFERENCES journal(event_id),
 operation_id TEXT NOT NULL UNIQUE REFERENCES operations(operation_id),
 PRIMARY KEY (run_id, control_generation)
) STRICT;

-- Links: the attempt belongs to the run, the operation is a claimed internal
-- service.adopt for that run (never an event ID), and the event is the
-- schema-2 service.adopted naming this run, attempt, generation and operation.
CREATE TRIGGER service_assignment_epochs_identity BEFORE INSERT ON service_assignment_epochs
WHEN NEW.operation_id = NEW.adoption_event_id
 OR NOT EXISTS (SELECT 1 FROM attempts WHERE attempt_id = NEW.attempt_id AND run_id = NEW.run_id)
 OR NOT EXISTS (SELECT 1 FROM operations WHERE operation_id = NEW.operation_id AND method = 'service.adopt' AND object = NEW.run_id AND state = 'claimed')
 OR NOT EXISTS (SELECT 1 FROM journal WHERE event_id = NEW.adoption_event_id AND schema_version = 2 AND type = 'service.adopted'
  AND run_id = NEW.run_id AND attempt_id = NEW.attempt_id AND generation = NEW.control_generation
  AND json_type(payload, '$.control_generation') = 'integer' AND json_extract(payload, '$.control_generation') = NEW.control_generation
  AND json_type(payload, '$.operation_id') = 'text' AND json_extract(payload, '$.operation_id') = NEW.operation_id)
BEGIN SELECT RAISE(ABORT, 'service_assignment_epochs link mismatch'); END;
CREATE TRIGGER service_assignment_epochs_stale BEFORE INSERT ON service_assignment_epochs
WHEN EXISTS (SELECT 1 FROM service_assignment_epochs WHERE run_id = NEW.run_id AND control_generation >= NEW.control_generation)
BEGIN SELECT RAISE(ABORT, 'service_assignment_epochs generation is stale'); END;
-- A replacing conflict resolution would delete a victim through any key or
-- the hidden rowid; refuse every collision before insertion.
CREATE TRIGGER service_assignment_epochs_no_replace BEFORE INSERT ON service_assignment_epochs
WHEN EXISTS (SELECT 1 FROM service_assignment_epochs WHERE rowid = NEW.rowid
 OR (run_id = NEW.run_id AND control_generation = NEW.control_generation)
 OR adoption_event_id = NEW.adoption_event_id OR operation_id = NEW.operation_id)
BEGIN SELECT RAISE(ABORT, 'service_assignment_epochs identity already exists'); END;
CREATE TRIGGER service_assignment_epochs_no_update BEFORE UPDATE ON service_assignment_epochs
BEGIN SELECT RAISE(ABORT, 'service_assignment_epochs is append-only'); END;
CREATE TRIGGER service_assignment_epochs_no_delete BEFORE DELETE ON service_assignment_epochs
BEGIN SELECT RAISE(ABORT, 'service_assignment_epochs is append-only'); END;

-- The sole epoch-to-assignment projector: advance or create the released row.
CREATE TRIGGER service_assignment_epochs_project AFTER INSERT ON service_assignment_epochs
BEGIN
 UPDATE run_assignments SET owner = 'supervisor',
  assigned_at = (SELECT observed_at FROM journal WHERE event_id = NEW.adoption_event_id),
  operation_id = NEW.operation_id
 WHERE run_id = NEW.run_id;
 INSERT INTO run_assignments (run_id, owner, assigned_at, operation_id)
 SELECT NEW.run_id, 'supervisor', (SELECT observed_at FROM journal WHERE event_id = NEW.adoption_event_id), NEW.operation_id
 WHERE NOT EXISTS (SELECT 1 FROM run_assignments WHERE run_id = NEW.run_id);
END;

-- Once epoch history exists, the assignment must equal the latest epoch.
CREATE TRIGGER run_assignments_epoch_update BEFORE UPDATE ON run_assignments
WHEN EXISTS (SELECT 1 FROM service_assignment_epochs WHERE run_id IN (OLD.run_id, NEW.run_id))
 AND NOT EXISTS (
  SELECT 1 FROM service_assignment_epochs e JOIN journal j ON j.event_id = e.adoption_event_id
  WHERE NEW.run_id = OLD.run_id AND e.run_id = NEW.run_id
  AND e.control_generation = (SELECT MAX(control_generation) FROM service_assignment_epochs WHERE run_id = NEW.run_id)
  AND NEW.owner = 'supervisor' AND NEW.operation_id = e.operation_id AND NEW.assigned_at = j.observed_at)
BEGIN SELECT RAISE(ABORT, 'run_assignments diverges from its latest assignment epoch'); END;
CREATE TRIGGER run_assignments_epoch_delete BEFORE DELETE ON run_assignments
WHEN EXISTS (SELECT 1 FROM service_assignment_epochs WHERE run_id = OLD.run_id)
BEGIN SELECT RAISE(ABORT, 'run_assignments with epoch history is immutable'); END;

-- Selected-data revision inventory for epoch history.
CREATE TRIGGER service_assignment_epochs_revision_insert AFTER INSERT ON service_assignment_epochs
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (NEW.run_id); END;
CREATE TRIGGER service_assignment_epochs_revision_update AFTER UPDATE ON service_assignment_epochs
WHEN NEW.run_id IS NOT OLD.run_id OR NEW.control_generation IS NOT OLD.control_generation OR NEW.attempt_id IS NOT OLD.attempt_id OR NEW.adoption_event_id IS NOT OLD.adoption_event_id OR NEW.operation_id IS NOT OLD.operation_id
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id, NEW.run_id); END;
CREATE TRIGGER service_assignment_epochs_revision_delete BEFORE DELETE ON service_assignment_epochs
BEGIN UPDATE run_control_revisions SET revision = revision + 1 WHERE run_id IN (OLD.run_id); END;
UPDATE migration_state SET schema_version = 10 WHERE id = 1;
