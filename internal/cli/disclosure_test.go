package cli

import (
	"context"
	"database/sql"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// disclosureSentence is the verbatim trusted-host sentence every honesty
// surface must carry: the residual host trust is never softened (I09).
const disclosureSentence = "runs with your host authority and is not adversarially contained"

// TestTrustedHostDisclosureAnchored pins the trusted-host wording on its three
// surfaces: the consented disclosure text, the run --help profile text and
// the receipt label. Each assertion anchors to the exact exported string or
// command output, never a file-wide match; restoring any weaker wording fails.
func TestTrustedHostDisclosureAnchored(t *testing.T) {
	t.Run("consent text", func(t *testing.T) {
		if !strings.Contains(admission.TrustedHostDisclosure, disclosureSentence) {
			t.Errorf("admission.TrustedHostDisclosure = %q, want it to contain %q",
				admission.TrustedHostDisclosure, disclosureSentence)
		}
	})
	t.Run("run help", func(t *testing.T) {
		code, stdout, stderr := runMain("run", "--help")
		if code != int(ExitOK) {
			t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr)
		}
		// The execution-profile flag's own help carries the sentence.
		_, usage, ok := strings.Cut(stdout, "-execution-profile")
		if !ok {
			t.Fatalf("run --help lacks -execution-profile:\n%s", stdout)
		}
		flagHelp, _, _ := strings.Cut(usage, "\n  -")
		if !strings.Contains(flagHelp, disclosureSentence) {
			t.Errorf("-execution-profile help = %q, want it to contain %q", flagHelp, disclosureSentence)
		}
	})
	t.Run("receipt label", func(t *testing.T) {
		ctx := context.Background()
		j, err := journal.Open(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = j.Close() }()
		now := time.Now().UTC()
		if err := j.Transact(ctx, func(tx *sql.Tx) error {
			return journal.InsertRun(ctx, tx, journal.RunRow{RunID: "run_disclosure", State: "ready_for_review",
				AdapterID: "fake", SourceRepo: "/repo", TaskSHA256: strings.Repeat("a", 64),
				BillingPosture: "local-scripted", ExecutionProfile: admission.ProfileTrustedHost,
				CreatedAt: now, UpdatedAt: now})
		}); err != nil {
			t.Fatal(err)
		}
		r, err := supervisor.BuildReceipt(ctx, j, "run_disclosure")
		if err != nil {
			t.Fatal(err)
		}
		label, ok := r["execution_bundle"].(map[string]any)["execution_profile"].(string)
		if !ok {
			t.Fatalf("execution_profile = %v, want a string label", r["execution_bundle"])
		}
		if !strings.Contains(label, disclosureSentence) {
			t.Errorf("receipt execution_profile = %q, want it to contain %q", label, disclosureSentence)
		}
	})
}

// TestNoMasquerade pins that only the contain provider substantiates a
// boundary claim (I05): the only Enforced: true literals in non-test sources
// live under internal/contain, and the owner lock, reservations and UI toggles
// appear in no recorded coverage.
func TestNoMasquerade(t *testing.T) {
	t.Run("enforced claims are provider-only", func(t *testing.T) {
		root := repoRoot(t)
		fset := token.NewFileSet()
		var offenders, hits []string
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Dot-directories hold tooling and nested checkouts, never
				// package sources of this tree.
				if path != root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			enforced := false
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok || key.Name != "Enforced" {
						continue
					}
					if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
						enforced = true
					}
				}
				return true
			})
			if !enforced {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			hits = append(hits, rel)
			if !strings.HasPrefix(rel, "internal/contain/") {
				offenders = append(offenders, rel)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
		if len(hits) == 0 {
			t.Fatal("no Enforced: true literal found anywhere; the provider must substantiate its boundary")
		}
		if len(offenders) > 0 {
			t.Errorf("Enforced: true outside internal/contain: %v (all hits: %v)", offenders, hits)
		}
	})
	t.Run("coordination is never coverage", func(t *testing.T) {
		seeds := contain.SeedV1()
		if len(seeds) == 0 {
			t.Fatal("SeedV1 is empty; nothing pins the boundary")
		}
		claims := 0
		for key, ev := range seeds {
			for _, c := range []contain.Claim{ev.Coverage.Filesystem, ev.Coverage.Process, ev.Coverage.Network, ev.Coverage.Credential} {
				claims++
				for _, text := range []string{c.Name, c.Detail} {
					for _, word := range []string{"owner lock", "ownerlock", "toggle", "reservation"} {
						if strings.Contains(strings.ToLower(text), word) {
							t.Errorf("%s claim %q mentions %q; coordination mechanisms are not containment evidence (I05)", key, text, word)
						}
					}
				}
			}
		}
		if claims == 0 {
			t.Fatal("no claims scanned")
		}
	})
}

// repoRoot returns the checkout root holding this test file.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
