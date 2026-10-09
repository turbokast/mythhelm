package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
)

func parseEnvelopeFlags(t *testing.T, args ...string) (*billing.Ceilings, error) {
	t.Helper()
	fs := newFlagSet("run")
	addEnvelopeFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return envelopeFlagCeilings(fs)
}

func TestEnvelopeFlagPrecedence(t *testing.T) {
	t.Parallel()
	flags, err := parseEnvelopeFlags(t, "--envelope-repairs", "5")
	want := billing.Ceilings{Execution: -1, Repairs: 5, Replans: -1, TransportRetries: -1}
	if err != nil || flags == nil || *flags != want {
		t.Fatalf("flag layer = %+v, %v; want %+v with the other fields unset", flags, err, want)
	}

	// The file layer decodes separately from the flag layer.
	cfg, _, err := admission.ParseProjectConfig([]byte("schema_version = 1\n[envelopes]\nrepairs = 1\n[[checks]]\nname = \"t\"\nargv = [\"true\"]\ntimeout = \"1s\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := cfg.Envelopes.ToCeilings()
	if err != nil || file == nil || file.Repairs != 1 {
		t.Fatalf("file layer = %+v, %v; want repairs 1", file, err)
	}
	if flags.Repairs != 5 {
		t.Fatalf("flag layer repairs = %d after decoding the file layer, want 5", flags.Repairs)
	}
	// Precedence itself is billing.ResolveCeilings, applied at admission.
	if got := billing.ResolveCeilings(flags, file); got.Repairs != 5 {
		t.Fatalf("resolved repairs = %d, want the flag's 5", got.Repairs)
	}

	all, err := parseEnvelopeFlags(t, "--envelope-execution", "45m", "--envelope-repairs", "0", "--envelope-replans", "1", "--envelope-transport-retries", "9")
	if err != nil || all == nil || *all != (billing.Ceilings{Execution: 45 * time.Minute, Repairs: 0, Replans: 1, TransportRetries: 9}) {
		t.Fatalf("all flags = %+v, %v (a zero count is a set value)", all, err)
	}

	none, err := parseEnvelopeFlags(t)
	if err != nil || none != nil {
		t.Fatalf("no flags = %+v, %v; want a nil layer", none, err)
	}
}

func TestEnvelopeFlagsRejectInvalid(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--envelope-repairs", "-1"},
		{"--envelope-replans", "-3"},
		{"--envelope-transport-retries", "-1"},
		{"--envelope-execution", "0s"},
		{"--envelope-execution", "-5m"},
		{"--envelope-execution", "soon"},
	} {
		if _, err := parseEnvelopeFlags(t, args...); err == nil {
			t.Errorf("%v accepted, want an error", args)
		}
	}
}

func TestRunRejectsInvalidEnvelopeFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"run", "--task-file", "t.md", "--adapter", "fake", "--billing", "local-scripted", "--non-interactive", "--envelope-repairs", "-1"},
		Stdio{In: strings.NewReader(""), Out: &out, Err: &errb})
	if code != int(ExitInvalid) {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, ExitInvalid, errb.String())
	}
	if !strings.Contains(out.String()+errb.String(), "envelope-repairs") {
		t.Fatalf("output does not name the flag: %q %q", out.String(), errb.String())
	}
}
