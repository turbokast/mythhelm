package qualify

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
)

// harnessIDsV72 is the v2 §7.2 candidate-harness order the seed follows.
var harnessIDsV72 = []string{"claude-code", "codex", "opencode", "muse", "kimi", "cursor", "antigravity"}

func TestSeedHasSevenHarnesses(t *testing.T) {
	seed := SeedV1()
	if len(seed) != 7 {
		t.Fatalf("SeedV1 returns %d records, want exactly the seven v2 §7.2 harnesses", len(seed))
	}
	for i, rec := range seed {
		if rec.Key.Harness != harnessIDsV72[i] {
			t.Fatalf("SeedV1[%d].Harness = %q, want %q (v2 §7.2 order)", i, rec.Key.Harness, harnessIDsV72[i])
		}
	}

	blocked := 0
	hashes := map[string]bool{}
	for _, rec := range seed {
		if rec.Progress != ProgressBlocked && rec.Progress != ProgressPlanned {
			t.Errorf("%s progress = %q, want blocked or planned (never fixture-tested or better)", rec.Key.Harness, rec.Progress)
		}
		if rec.Progress == ProgressBlocked {
			blocked++
			parts := strings.SplitN(rec.NextTest, "; authority:", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				t.Errorf("%s NextTest = %q, want <step>; authority: <grant> with both halves non-empty", rec.Key.Harness, rec.NextTest)
			}
		}
		if rec.Key.OS != runtime.GOOS || rec.Key.Arch != runtime.GOARCH {
			t.Errorf("%s key = %s/%s, want the local platform %s/%s",
				rec.Key.Harness, rec.Key.OS, rec.Key.Arch, runtime.GOOS, runtime.GOARCH)
		}
		if rec.SchemaVersion != 2 {
			t.Errorf("%s SchemaVersion = %d, want 2", rec.Key.Harness, rec.SchemaVersion)
		}
		if rec.Revision != 1 {
			t.Errorf("%s Revision = %d, want 1", rec.Key.Harness, rec.Revision)
		}
		digest, err := CanonicalDigest(rec)
		if err != nil {
			t.Errorf("%s CanonicalDigest: %v", rec.Key.Harness, err)
		} else if digest != rec.Digest {
			t.Errorf("%s Digest = %q, want the recomputed %q", rec.Key.Harness, rec.Digest, digest)
		}
		hashes[KeyHash(rec.Key)] = true
	}
	if blocked == 0 {
		t.Error("no seed record is blocked, want at least one")
	}
	if len(hashes) != 7 {
		t.Fatalf("%d unique seed key hashes, want 7 (keys must not collide)", len(hashes))
	}

	if !reflect.DeepEqual(seed, SeedV1()) {
		t.Fatal("SeedV1 is not deterministic across calls")
	}
}

func TestEnsureSeededIdempotent(t *testing.T) {
	ctx := t.Context()

	t.Run("empty table records seven", func(t *testing.T) {
		j := openSeeded(t)
		if got := countCurrent(t, j); got != 7 {
			t.Fatalf("current rows after EnsureSeeded = %d, want 7", got)
		}
		for _, rec := range SeedV1() {
			stored, revision, err := j.CurrentQualificationRecord(ctx, KeyHash(rec.Key))
			if err != nil {
				t.Fatalf("Current %s: %v", rec.Key.Harness, err)
			}
			if revision != 1 {
				t.Fatalf("Current %s revision = %d, want 1", rec.Key.Harness, revision)
			}
			decoded, err := DecodeRecord([]byte(stored))
			if err != nil {
				t.Fatalf("DecodeRecord %s: %v", rec.Key.Harness, err)
			}
			recomputed, err := CanonicalDigest(decoded)
			if err != nil {
				t.Fatalf("CanonicalDigest %s: %v", rec.Key.Harness, err)
			}
			if recomputed != rec.Digest {
				t.Fatalf("%s stored digest does not recompute", rec.Key.Harness)
			}
		}
	})

	t.Run("complete table untouched", func(t *testing.T) {
		j := openSeeded(t)
		before := currentHash(t, j)
		if err := EnsureSeeded(ctx, j); err != nil {
			t.Fatalf("EnsureSeeded on a complete table: %v", err)
		}
		if after := currentHash(t, j); after != before {
			t.Fatal("EnsureSeeded on a complete table changed a row")
		}
		if got := countCurrent(t, j); got != 7 {
			t.Fatalf("current rows = %d, want 7", got)
		}
	})

	t.Run("partial seed completes", func(t *testing.T) {
		seed := SeedV1()
		j := openPartial(t, seed[:3])
		if got := countCurrent(t, j); got != 3 {
			t.Fatalf("partial setup holds %d rows, want 3", got)
		}
		before := map[string]string{}
		for _, rec := range seed[:3] {
			stored, _, err := j.CurrentQualificationRecord(ctx, KeyHash(rec.Key))
			if err != nil {
				t.Fatal(err)
			}
			before[rec.Key.Harness] = stored
		}
		if err := EnsureSeeded(ctx, j); err != nil {
			t.Fatalf("EnsureSeeded on a partial seed: %v", err)
		}
		if got := countCurrent(t, j); got != 7 {
			t.Fatalf("current rows = %d, want 7", got)
		}
		for _, rec := range seed[:3] {
			stored, _, err := j.CurrentQualificationRecord(ctx, KeyHash(rec.Key))
			if err != nil {
				t.Fatal(err)
			}
			if stored != before[rec.Key.Harness] {
				t.Fatalf("%s row changed while completing the seed", rec.Key.Harness)
			}
		}
	})

	t.Run("concurrent seeds leave seven", func(t *testing.T) {
		dir := t.TempDir()
		first, err := journal.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = first.Close() })
		second, err := journal.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = second.Close() })
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, j := range []*journal.Journal{first, second} {
			wg.Go(func() {
				errs <- EnsureSeeded(ctx, j)
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent EnsureSeeded: %v", err)
			}
		}
		if got := countCurrent(t, first); got != 7 {
			t.Fatalf("current rows after concurrent seeds = %d, want exactly 7", got)
		}
	})
}

func TestEnsureSeededFailureWrapsJournalError(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	err = EnsureSeeded(t.Context(), j)
	if err == nil {
		t.Fatal("EnsureSeeded on a closed journal returned nil, want the journal error wrapped")
	}
	if !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("EnsureSeeded error = %q, want the journal error wrapped", err)
	}
	if !strings.HasPrefix(err.Error(), "qualify:") {
		t.Fatalf("EnsureSeeded error = %q, want the qualify prefix", err)
	}
}

// openSeeded opens a fresh journal and runs EnsureSeeded on it; the caller
// asserts the outcome.
func openSeeded(t *testing.T) *journal.Journal {
	t.Helper()
	j := openPartial(t, nil)
	if err := EnsureSeeded(t.Context(), j); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	return j
}

// openPartial opens a fresh journal and pre-inserts pre (nil for none)
// without seeding.
func openPartial(t *testing.T, pre []Record) *journal.Journal {
	t.Helper()
	ctx := t.Context()
	j, err := journal.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	for _, rec := range pre {
		raw, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := j.InsertQualificationRecordIfAbsent(ctx, KeyHash(rec.Key), 1, rec.Digest, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	return j
}

func countCurrent(t *testing.T, j *journal.Journal) int {
	t.Helper()
	recs, err := j.ListCurrentQualificationRecords(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return len(recs)
}

// currentHash hashes every current (key hash, revision, bytes) triple, so
// any row change — content, revision or set — changes it.
func currentHash(t *testing.T, j *journal.Journal) [32]byte {
	t.Helper()
	ctx := t.Context()
	h := sha256.New()
	for _, rec := range SeedV1() {
		stored, revision, err := j.CurrentQualificationRecord(ctx, KeyHash(rec.Key))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(h).Encode([]any{KeyHash(rec.Key), revision, stored}); err != nil {
			t.Fatal(err)
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
