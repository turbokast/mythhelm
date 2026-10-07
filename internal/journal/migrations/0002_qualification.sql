-- Schema v2 (qualification-registry, v2 §7.1): versioned qualification
-- records. A released migration is never edited; later changes go in a new
-- numbered file.
CREATE TABLE qualification_records (
    key_hash TEXT NOT NULL,      -- sha256 over canonical Key JSON
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    record_json TEXT NOT NULL,   -- canonical Record JSON, schema_version 2
    recorded_at TEXT NOT NULL,   -- RFC3339Nano
    superseded_at TEXT,          -- NULL while current
    PRIMARY KEY (key_hash, revision)
) STRICT;
CREATE UNIQUE INDEX idx_qualification_current ON qualification_records(key_hash) WHERE superseded_at IS NULL;
