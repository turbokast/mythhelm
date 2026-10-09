package v2contract_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// loadGolden reads testdata/<rel>; later tasks' tests reuse it.
func loadGolden(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read golden %s: %v", rel, err)
	}
	return b
}

// I09 (v2 §2): an unknown key is an error naming the key, never dropped.
func TestDecodeRejectsUnknownField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"unknown key named", `{"kind":"artifact","id":"a","revision":1,"bogus":true}`, "bogus"},
		{"truncated names offset", `{"kind":"artifact","id":`, "offset 24"},
		{"syntax error names offset", `{"kind":"artifact",,}`, "offset 20"},
		{"trailing data rejected", `{"kind":"artifact","id":"a","revision":1} {}`, "trailing data"},
		{"validation error propagates", `{"kind":"other","id":"a","revision":1}`, "kind"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := v2contract.Decode[v2contract.RevisionRef]([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Decode(%s) error = %v, want containing %q", tc.in, err, tc.wantErr)
			}
		})
	}

	got, err := v2contract.Decode[v2contract.RevisionRef]([]byte(`{"kind":"contract","id":"c1","revision":2}`))
	if err != nil {
		t.Fatalf("Decode(valid) error = %v", err)
	}
	if want := (v2contract.RevisionRef{Kind: "contract", ID: "c1", Revision: 2}); got != want {
		t.Errorf("Decode(valid) = %+v, want %+v", got, want)
	}
}

// I20 (v2 §2): digests are content-addressed. The golden equals
// `printf '%s' '{"format":"sha1","value":"abc"}' | sha256sum`.
func TestDigestGolden(t *testing.T) {
	t.Parallel()
	const golden = "sha256:132acd35377fa231649f9681586be0c41a3e72e6d027ed0184a9afb524e56eb5"
	if got := v2contract.Digest(v2contract.GitObject{Format: "sha1", Value: "abc"}); got != golden {
		t.Errorf("Digest = %q, want %q", got, golden)
	}
	if got := v2contract.Digest(v2contract.GitObject{Format: "sha1", Value: "abd"}); got == golden {
		t.Errorf("Digest unchanged after flipping one byte: %q", got)
	}
	if got := v2contract.Digest(map[string]float64{"x": math.NaN()}); got != "" {
		t.Errorf("Digest(non-finite) = %q, want empty", got)
	}
}
