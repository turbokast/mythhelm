package supervisor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
)

// schemaImage is the bytes of a freshly migrated, fully checkpointed journal
// database. It is never written to after capture.
type schemaImage struct {
	data []byte
	mode fs.FileMode
}

// pristineSchema captures the image once per test process from a real
// journal.Open, so the migrations run once instead of once per fixture.
var pristineSchema = sync.OnceValues(func() (schemaImage, error) {
	return captureSchemaImage(context.Background(), journal.Open)
})

// captureSchemaImage opens a journal with open in a new temporary directory,
// closes it, checkpoints the WAL away, verifies the schema and returns the
// database bytes. The directory is removed before returning.
func captureSchemaImage(ctx context.Context, open func(context.Context, string) (*journal.Journal, error)) (img schemaImage, err error) {
	dir, err := os.MkdirTemp("", "mythhelm-schema-image-")
	if err != nil {
		return schemaImage{}, err
	}
	defer func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil && err == nil {
			img, err = schemaImage{}, fmt.Errorf("removing schema image directory: %w", rmErr)
		}
	}()
	j, err := open(ctx, dir)
	if err != nil {
		return schemaImage{}, fmt.Errorf("opening template journal: %w", err)
	}
	if err := j.Close(); err != nil {
		return schemaImage{}, fmt.Errorf("closing template journal: %w", err)
	}
	path := filepath.Join(dir, journal.DBName)
	if err := checkpointAndVerify(ctx, path); err != nil {
		return schemaImage{}, err
	}
	for _, side := range []string{"-wal", "-shm", "-journal"} {
		fi, err := os.Stat(path + side)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return schemaImage{}, fmt.Errorf("inspecting %s: %w", side, err)
		case side == "-wal" && fi.Size() > 0:
			return schemaImage{}, fmt.Errorf("template WAL still holds %d bytes after checkpoint", fi.Size())
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return schemaImage{}, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // G304: the template's own path
	if err != nil {
		return schemaImage{}, fmt.Errorf("reading template database: %w", err)
	}
	return schemaImage{data: data, mode: fi.Mode().Perm()}, nil
}

func checkpointAndVerify(ctx context.Context, path string) error {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: p}).String())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var busy, logFrames, checkpointed int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("checkpointing template: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("template checkpoint busy (log %d, checkpointed %d)", logFrames, checkpointed)
	}
	var version, tables int
	var check string
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != journal.SchemaVersion {
		return fmt.Errorf("template user_version %d, want %d", version, journal.SchemaVersion)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables); err != nil {
		return err
	}
	if tables == 0 {
		return errors.New("template has no tables")
	}
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("template integrity_check: %s", check)
	}
	return db.Close()
}

// writeSchemaImage writes img as a new regular database file in state. It
// refuses to replace an existing database or its WAL companions.
func writeSchemaImage(state string, img schemaImage) error {
	if len(img.data) == 0 {
		return errors.New("empty schema image")
	}
	path := filepath.Join(state, journal.DBName)
	for _, side := range []string{"-wal", "-shm", "-journal"} {
		_, err := os.Lstat(path + side)
		if err == nil {
			return fmt.Errorf("refusing to seed %s: %s exists", path, side)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("refusing to seed %s: stat %s: %w", path, side, err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, img.mode) //nolint:gosec // G304: the fixture's own state root
	if err != nil {
		return fmt.Errorf("refusing to seed %s: %w", path, err)
	}
	if _, err := f.Write(img.data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// SeedJournalState writes the pristine schema image into state, an empty
// fixture state root, so a later journal.Open skips the migrations.
func SeedJournalState(t *testing.T, state string) {
	t.Helper()
	img, err := pristineSchema()
	if err != nil {
		t.Fatalf("pristine schema image: %v", err)
	}
	if err := writeSchemaImage(state, img); err != nil {
		t.Fatal(err)
	}
}

// openSeededJournal is journal.Open on a fresh state root seeded with the
// pristine schema image.
func openSeededJournal(t *testing.T) *journal.Journal {
	t.Helper()
	state := t.TempDir()
	SeedJournalState(t, state)
	j, err := journal.Open(t.Context(), state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

// dumpDB renders the schema objects and every table's rows of the database
// at path, in a stable order.
func dumpDB(t *testing.T, path string) (version int, schema, rows []string) {
	t.Helper()
	db := rawOpen(t, path)
	if err := db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	r, err := db.QueryContext(t.Context(), "SELECT type, name, tbl_name, coalesce(sql, '') FROM sqlite_master ORDER BY type, name")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for r.Next() {
		var typ, name, tbl, ddl string
		if err := r.Scan(&typ, &name, &tbl, &ddl); err != nil {
			t.Fatal(err)
		}
		schema = append(schema, strings.Join([]string{typ, name, tbl, ddl}, "|"))
		if typ == "table" {
			tables = append(tables, name)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	for _, table := range tables {
		tr, err := db.QueryContext(t.Context(), fmt.Sprintf("SELECT * FROM %q ORDER BY rowid", table))
		if err != nil {
			t.Fatal(err)
		}
		cols, err := tr.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for tr.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := tr.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			rows = append(rows, fmt.Sprintf("%s:%v", table, vals))
		}
		if err := tr.Err(); err != nil {
			t.Fatal(err)
		}
		_ = tr.Close()
	}
	return version, schema, rows
}

func TestSeededJournalMatchesFreshJournal(t *testing.T) {
	t.Parallel()
	fresh, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	seeded := openSeededJournal(t)
	fv, fschema, fr := dumpDB(t, filepath.Join(fresh.StateDir(), journal.DBName))
	sv, ss, sr := dumpDB(t, filepath.Join(seeded.StateDir(), journal.DBName))
	if fv != journal.SchemaVersion || sv != fv {
		t.Fatalf("user_version fresh %d seeded %d, want %d", fv, sv, journal.SchemaVersion)
	}
	if len(fschema) == 0 || !slices.Equal(fschema, ss) {
		t.Fatalf("schema objects differ:\nfresh  %q\nseeded %q", fschema, ss)
	}
	if !slices.Equal(fr, sr) {
		t.Fatalf("initial rows differ (only schema-seeded rows may exist):\nfresh  %q\nseeded %q", fr, sr)
	}
}

func TestSeededJournalsAreIsolated(t *testing.T) {
	t.Parallel()
	img, err := pristineSchema()
	if err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(img.data)
	a, b := openSeededJournal(t), openSeededJournal(t)
	pathA := filepath.Join(a.StateDir(), journal.DBName)
	pathB := filepath.Join(b.StateDir(), journal.DBName)
	fiA, err := os.Lstat(pathA)
	if err != nil {
		t.Fatal(err)
	}
	fiB, err := os.Lstat(pathB)
	if err != nil {
		t.Fatal(err)
	}
	if !fiA.Mode().IsRegular() || !fiB.Mode().IsRegular() || os.SameFile(fiA, fiB) {
		t.Fatalf("fixtures share a database file: %s and %s", pathA, pathB)
	}
	_, _, emptyRows := dumpDB(t, pathB)

	createRuns := func(j *journal.Journal, prefix string, n int) error {
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() {
				errs[i] = CreateRun(t.Context(), j, journal.RunRow{RunID: fmt.Sprintf("run_%s_%d", prefix, i), AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
					TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, NewProducer(fmt.Sprintf("sup_%s_%d", prefix, i), 1))
			})
		}
		wg.Wait()
		return errors.Join(errs...)
	}
	var wg sync.WaitGroup
	var errA, errB error
	wg.Go(func() { errA = createRuns(a, "a", 4) })
	wg.Go(func() { errB = createRuns(b, "b", 1) })
	wg.Wait()
	if err := errors.Join(errA, errB); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		j    *journal.Journal
		want int
	}{{a, 4}, {b, 1}} {
		runs, err := c.j.ListRuns(t.Context(), 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(runs) != c.want {
			t.Fatalf("%s holds %d runs, want %d", c.j.StateDir(), len(runs), c.want)
		}
	}
	if sha256.Sum256(img.data) != before {
		t.Fatal("writing a seeded journal changed the template image")
	}

	// Closing and deleting A leaves B working.
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(a.StateDir()); err != nil {
		t.Fatal(err)
	}
	if err := createRuns(b, "b2", 2); err != nil {
		t.Fatal(err)
	}
	runs, err := b.ListRuns(t.Context(), 100)
	if err != nil || len(runs) != 3 {
		t.Fatalf("B after deleting A: %d runs, %v; want 3", len(runs), err)
	}
	if _, _, rows := dumpDB(t, pathB); slices.Equal(rows, emptyRows) {
		t.Fatal("B's writes are not visible in B's own file")
	}

	// A fresh seed after all of that is still pristine.
	c := openSeededJournal(t)
	if runs, err := c.ListRuns(t.Context(), 100); err != nil || len(runs) != 0 {
		t.Fatalf("new seeded journal holds %d runs, %v; want 0", len(runs), err)
	}
}

func TestSeedRefusesExistingState(t *testing.T) {
	t.Parallel()
	img, err := pristineSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{journal.DBName, journal.DBName + "-wal", journal.DBName + "-shm"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state := t.TempDir()
			path := filepath.Join(state, name)
			if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := writeSchemaImage(state, img); err == nil || !strings.Contains(err.Error(), "refusing to seed") {
				t.Fatalf("seeding over %s: err = %v, want a refusal", name, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "keep me" { //nolint:gosec // G304: the test's own TempDir
				t.Fatalf("%s after refusal = %q, %v; want the original bytes", name, got, err)
			}
		})
	}
}

func TestSchemaImageCaptureFailsClosed(t *testing.T) {
	t.Parallel()
	openErr := errors.New("open failed")
	for _, c := range []struct {
		name string
		open func(context.Context, string) (*journal.Journal, error)
		want string
	}{
		{"open error", func(context.Context, string) (*journal.Journal, error) { return nil, openErr }, "open failed"},
		{"stale schema version", func(ctx context.Context, dir string) (*journal.Journal, error) {
			j, err := journal.Open(ctx, dir)
			if err != nil {
				return nil, err
			}
			db := rawOpen(t, filepath.Join(dir, journal.DBName))
			if _, err := db.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
				return nil, err
			}
			return j, db.Close()
		}, "user_version 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			img, err := captureSchemaImage(t.Context(), c.open)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
			if err := writeSchemaImage(t.TempDir(), img); err == nil {
				t.Fatal("a failed capture yielded a usable seed")
			}
		})
	}
}
