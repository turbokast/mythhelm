package qualify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
)

var (
	// ErrNotFound reports a lookup with no current record for the key.
	// Absence is not a zero record (I09).
	ErrNotFound = errors.New("qualify: no record for key")

	// ErrAmbiguousMatch reports a consult with several stable-identity
	// matches. It is a sentinel (not a message to substring-match) because
	// stored-row errors interpolate free-form digest text that could name
	// the ambiguity report.
	ErrAmbiguousMatch = errors.New("qualify: ambiguous stable match")

	// ErrSchemaMismatch marks a registry database whose schema this binary
	// cannot read (older or newer user_version, or a missing table).
	ErrSchemaMismatch = errors.New("qualify: schema mismatch")

	// errMissing marks a missing state dir or database, as opposed to an
	// unreadable or version-mismatched one.
	errMissing = errors.New("state dir or database is missing")

	// errReadOnly reports a write against a read-only handle, including
	// the empty dir-without-database form, which has no journal.
	errReadOnly = errors.New("qualify: registry is read-only")
)

// Registry is a versioned qualification record store over the state dir's
// mythhelm.db: one immutable revision chain per qualification key (I20).
type Registry struct {
	journal  *journal.Journal // nil for an existing dir with no database: reads empty
	dir      string
	readOnly bool
}

// Open opens the registry over the state dir's mythhelm.db, read-write,
// running pending migrations. It never creates qualification evidence:
// only EnsureSeeded adds records. dir must already exist.
func Open(ctx context.Context, dir string) (*Registry, error) {
	j, err := journal.Open(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("qualify: opening registry in %s: %w", dir, err)
	}
	return &Registry{journal: j, dir: dir}, nil
}

// OpenReadOnly opens the registry without migrating or seeding; used by
// doctor. It stats the dir and the database file itself because
// journal.OpenReadOnly reports ErrNoDatabase for a missing dir, a missing
// file and a version-0 file alike. An existing dir with no database file
// yields an empty registry whose List returns []; a present but version-0
// file errors, never mistaken for empty. A path that exists but is not a
// directory errors likewise, on every platform.
func OpenReadOnly(ctx context.Context, dir string) (*Registry, error) {
	if fi, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("qualify: state dir %s: %w", dir, errMissing)
		}
		return nil, fmt.Errorf("qualify: state dir %s: %w", dir, err)
	} else if !fi.IsDir() {
		// A file as dir must fail loudly on every platform: without this,
		// the database stat below yields ENOTDIR on Linux but ENOENT on
		// Windows, which would read as a missing (empty) registry there.
		return nil, fmt.Errorf("qualify: state dir %s is not a directory", dir)
	}
	path := filepath.Join(dir, journal.DBName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Registry{dir: dir, readOnly: true}, nil
		}
		return nil, fmt.Errorf("qualify: state database %s: %w", path, err)
	}
	j, err := journal.OpenReadOnly(ctx, dir)
	if err != nil {
		if errors.Is(err, journal.ErrNoDatabase) {
			// The file exists (statted above), so this is a version-0
			// database, never a missing one.
			return nil, fmt.Errorf("qualify: state database %s has schema version 0: %w", path, ErrSchemaMismatch)
		}
		if errors.Is(err, journal.ErrSchemaTooNew) || isOlderVersion(err) {
			return nil, fmt.Errorf("qualify: opening registry in %s: %w: %w", dir, ErrSchemaMismatch, err)
		}
		return nil, fmt.Errorf("qualify: opening registry in %s: %w", dir, err)
	}
	return &Registry{journal: j, dir: dir, readOnly: true}, nil
}

// isOlderVersion reports journal.OpenReadOnly's older-schema error, which
// carries no sentinel: its in-repo message names the direction
// (internal/journal/projections.go: "older than"). Callers pin this through
// the v1-database test, so a rewording fails loudly.
func isOlderVersion(err error) bool {
	return strings.Contains(err.Error(), "older than")
}

// Close closes the registry; errors are returned, never hidden.
func (r *Registry) Close() error {
	if r.journal == nil {
		return nil
	}
	return r.journal.Close()
}

// Lookup returns the current record for the exact key, or ErrNotFound.
// Stored JSON decodes exclusively via DecodeRecord, and the decoded digest
// must recompute: content-addressed reads fail closed (I20, NFR-2).
func (r *Registry) Lookup(ctx context.Context, k Key) (Record, error) {
	if r.journal == nil {
		return Record{}, ErrNotFound
	}
	stored, _, err := r.journal.CurrentQualificationRecord(ctx, KeyHash(k))
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return Record{}, ErrNotFound
		}
		return Record{}, r.mapReadError(err)
	}
	return decodeStored(stored)
}

// List returns every current record, ordered by harness then surface (with
// a key-hash tie-break so the order is total). A database without the
// table wraps ErrSchemaMismatch; one corrupt row fails the whole List —
// never a silently partial truth.
func (r *Registry) List(ctx context.Context) ([]Record, error) {
	if r.journal == nil {
		return []Record{}, nil
	}
	rows, err := r.journal.ListCurrentQualificationRecords(ctx)
	if err != nil {
		return nil, r.mapReadError(err)
	}
	out := make([]Record, 0, len(rows))
	for _, stored := range rows {
		rec, err := decodeStored(stored)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	slices.SortFunc(out, func(a, b Record) int {
		if c := strings.Compare(a.Key.Harness, b.Key.Harness); c != 0 {
			return c
		}
		if c := strings.Compare(a.Key.Surface, b.Key.Surface); c != 0 {
			return c
		}
		return strings.Compare(KeyHash(a.Key), KeyHash(b.Key))
	})
	return out, nil
}

// mapReadError maps a journal read failure to its registry error: a missing
// table (foreign or corrupt: migration 2 always creates it) wraps
// ErrSchemaMismatch, anything else is wrapped as-is.
func (r *Registry) mapReadError(err error) error {
	if strings.Contains(err.Error(), "no such table") {
		return fmt.Errorf("qualify: qualification table missing in %s: %w: %w", r.dir, ErrSchemaMismatch, err)
	}
	return fmt.Errorf("qualify: reading qualification records in %s: %w", r.dir, err)
}

// decodeStored decodes one stored row via DecodeRecord and verifies its
// digest recomputes.
func decodeStored(stored string) (Record, error) {
	rec, err := DecodeRecord([]byte(stored))
	if err != nil {
		return Record{}, err
	}
	recomputed, err := CanonicalDigest(rec)
	if err != nil {
		return Record{}, err
	}
	if rec.Digest != recomputed {
		return Record{}, fmt.Errorf("qualify: stored record digest mismatch: %q does not recompute (content hashes to %q)",
			rec.Digest, recomputed)
	}
	return rec, nil
}

// Record stores rec as revision current+1 (1 for a new key), ignoring
// rec.Revision; callers never pick revisions. Zero-valued fields default to
// the unknown markers (the DecodeRecord rule), SupersededAt is cleared — a
// Record call always stores a current revision — and the digest is derived:
// an empty Digest is filled in, while a non-empty Digest that does not
// recompute over the stored content is refused without writing anything.
// Single-writer: concurrent same-key Records collide on the primary key and
// fail closed; callers must not retry blindly.
func (r *Registry) Record(ctx context.Context, rec Record) error {
	if r.readOnly || r.journal == nil {
		return errReadOnly
	}
	applyDefaults(&rec)
	if column := evidenceFreeProven(rec); column != "" {
		return fmt.Errorf("qualify: proven verdict without evidence: %s", column)
	}
	keyHash := KeyHash(rec.Key)
	next := 1
	if _, current, err := r.journal.CurrentQualificationRecord(ctx, keyHash); err != nil {
		if !errors.Is(err, journal.ErrNotFound) {
			return r.mapReadError(err)
		}
	} else {
		next = current + 1
	}
	rec.Revision = next
	rec.SupersededAt = nil
	recomputed, err := CanonicalDigest(rec)
	if err != nil {
		return err
	}
	if rec.Digest != "" && rec.Digest != recomputed {
		return fmt.Errorf("qualify: record digest mismatch: presented %q, recomputed %q", rec.Digest, recomputed)
	}
	rec.Digest = recomputed
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("qualify: encoding record: %w", err)
	}
	if err := r.journal.InsertQualificationRecord(ctx, keyHash, next, rec.Digest, string(raw)); err != nil {
		return fmt.Errorf("qualify: recording qualification record: %w", err)
	}
	return nil
}

// evidenceFreeProven names the first column valued Proven with empty
// Evidence, or "" when every proven column carries evidence.
func evidenceFreeProven(rec Record) string {
	for _, column := range []struct {
		name string
		col  Column
	}{
		{"fidelity", rec.Fidelity},
		{"entitlement", rec.Entitlement},
		{"lifecycle", rec.Lifecycle},
	} {
		if column.col.Verdict == Proven && len(column.col.Evidence) == 0 {
			return column.name
		}
	}
	return ""
}

// Consult finds the record for an observed identity: exact key first, else
// the unique stable-identity match (Harness, Surface, OS, Arch,
// TrustProfile, EntitlementClass, AdapterProtocol — pinned digests and
// snapshots ignored) for drift comparison. Zero matches read ErrNotFound;
// several stable matches refuse as ambiguous. drifted reports CheckDrift on
// the matched record.
func (r *Registry) Consult(ctx context.Context, observed Key) (rec Record, drifted bool, reason string, err error) {
	if found, err := r.Lookup(ctx, observed); err == nil {
		drifted, reason := CheckDrift(DriftInput{Record: found, ExecutableDigest: observed.ExecutableDigest, ConfigDigest: observed.ConfigDigest})
		return found, drifted, reason, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Record{}, false, "", err
	}
	all, err := r.List(ctx)
	if err != nil {
		return Record{}, false, "", err
	}
	norm := normalizeKeyForHash(observed)
	var matches []Record
	for _, candidate := range all {
		if stableMatch(norm, candidate.Key) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return Record{}, false, "", ErrNotFound
	case 1:
		found := matches[0]
		drifted, reason := CheckDrift(DriftInput{Record: found, ExecutableDigest: observed.ExecutableDigest, ConfigDigest: observed.ConfigDigest})
		return found, drifted, reason, nil
	default:
		return Record{}, false, "", fmt.Errorf("%w: %d records for %s/%s",
			ErrAmbiguousMatch, len(matches), norm.Harness, norm.Surface)
	}
}

// stableMatch reports whether stored carries the same stable identity as the
// normalised observed key: the seven non-drift dimensions equal, pinned
// digests and snapshots ignored.
func stableMatch(observed, stored Key) bool {
	return observed.Harness == stored.Harness &&
		observed.Surface == stored.Surface &&
		observed.OS == stored.OS &&
		observed.Arch == stored.Arch &&
		observed.TrustProfile == stored.TrustProfile &&
		observed.EntitlementClass == stored.EntitlementClass &&
		observed.AdapterProtocol == stored.AdapterProtocol
}

// invalidationEvidenceID is the marker entry replacing invalidated column
// evidence; its Uncertainty text preserves the reason and the prior ids.
const invalidationEvidenceID = "ev_drift_invalidation"

// InvalidateOnDrift stores a new revision with affected columns reset to
// Unknown/NotProven and Progress to blocked, preserving the reason and the
// old evidence ids in the new record's Uncertainty text for audit (I20).
// Only rec.Key is read: the new revision always derives from the current
// row, so a stale argument can never fork the chain. An unknown key reads
// ErrNotFound; an empty reason is refused. It ships as tested API with no
// production caller in this spec: production databases hold no proven
// records to invalidate, and admission writes nothing.
func (r *Registry) InvalidateOnDrift(ctx context.Context, rec Record, reason string) error {
	if r.readOnly || r.journal == nil {
		return errReadOnly
	}
	if reason == "" {
		return errors.New("qualify: invalidate without reason")
	}
	current, err := r.Lookup(ctx, rec.Key)
	if err != nil {
		return err
	}
	next := current
	next.Digest = ""
	next.Progress = ProgressBlocked
	next.Capabilities = maps.Clone(current.Capabilities)
	next.Fidelity = invalidateColumn(current.Fidelity, reason)
	next.Entitlement = invalidateColumn(current.Entitlement, reason)
	next.Lifecycle = invalidateColumn(current.Lifecycle, reason)
	return r.Record(ctx, next)
}

// invalidateColumn resets one column to its post-drift value: a column
// with nothing claimed (Unknown, no evidence) is untouched, otherwise the
// verdict drops to NotProven — prior proof no longer establishes anything —
// and the evidence is replaced by an audit marker preserving the reason and
// the prior ids. The old revision keeps the full evidence (I20).
func invalidateColumn(col Column, reason string) Column {
	if col.Verdict == Unknown && len(col.Evidence) == 0 {
		return col
	}
	ids := make([]string, 0, len(col.Evidence))
	for _, e := range col.Evidence {
		ids = append(ids, e.ID)
	}
	prior := strings.Join(ids, ", ")
	if prior == "" {
		prior = "none"
	}
	verdict := NotProven
	if col.Verdict == Unknown {
		verdict = Unknown
	}
	return Column{
		Verdict: verdict,
		Evidence: []Evidence{{
			ID:          invalidationEvidenceID,
			Method:      "documented-mechanism",
			Suite:       unknownString,
			Result:      "blocked",
			Uncertainty: "invalidated: " + reason + "; prior evidence: " + prior,
			At:          time.Now().UTC(),
			Label:       Observed,
			Source:      "drift-check",
		}},
	}
}

// IsMissing reports whether err is a missing-dir-or-database condition (as
// opposed to unreadable or version-mismatched state).
func IsMissing(err error) bool { return errors.Is(err, errMissing) }

// IsSchemaMismatch reports errors.Is(err, ErrSchemaMismatch).
func IsSchemaMismatch(err error) bool { return errors.Is(err, ErrSchemaMismatch) }
