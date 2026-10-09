package admission_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
)

func envelopeRepo(t *testing.T, configTable string) (repo, task string) {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed test argv only
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	cfg := envelopesConfig(configTable)
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "mythhelm.toml")
	git("commit", "--quiet", "-m", "config")
	task = filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(task, []byte("# Demo task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo, task
}

func decideEnvelopes(t *testing.T, configTable string, flags *billing.Ceilings) admission.Decision {
	t.Helper()
	repo, task := envelopeRepo(t, configTable)
	_, digest, err := admission.ParseProjectConfig(envelopesConfig(configTable))
	if err != nil {
		t.Fatal(err)
	}
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: t.TempDir(), Repo: repo, TaskFile: task, Adapter: admission.AdapterFake,
		Billing: admission.BillingLocalScripted, ExecutionProfile: "trusted-host",
		TrustProjectConfig: "sha256:" + digest, Env: os.Environ(), EnvelopeFlags: flags,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDecideCarriesEnvelopeLayers(t *testing.T) {
	flags := &billing.Ceilings{Execution: -1, Repairs: 5, Replans: -1, TransportRetries: -1}
	d := decideEnvelopes(t, "[envelopes]\nexecution = \"10m\"\nrepairs = 1", flags)

	// The flag layer is carried exactly as given: not merged with the file
	// layer and not filled from the built-ins.
	if d.EnvelopeFlags == nil || *d.EnvelopeFlags != *flags {
		t.Fatalf("Decision.EnvelopeFlags = %+v, want %+v unresolved", d.EnvelopeFlags, flags)
	}
	file, err := d.ProjectConfig.Envelopes.ToCeilings()
	want := billing.Ceilings{Execution: 10 * time.Minute, Repairs: 1, Replans: -1, TransportRetries: -1}
	if err != nil || file == nil || *file != want {
		t.Fatalf("file layer = %+v, %v; want %+v unresolved", file, err, want)
	}

	// No flags given: the flag layer stays nil, never a resolved default.
	d = decideEnvelopes(t, "[envelopes]\nrepairs = 1", nil)
	if d.EnvelopeFlags != nil {
		t.Fatalf("Decision.EnvelopeFlags = %+v, want nil when no flag was given", d.EnvelopeFlags)
	}
}
