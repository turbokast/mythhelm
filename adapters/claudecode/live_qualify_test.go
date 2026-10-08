//go:build live

package claudecode

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/security"
)

// liveGranted reports the maintainer's explicit grant: exactly "1".
func liveGranted() bool { return os.Getenv("MYTHHELM_LIVE_QUALIFY") == "1" }

// TestLiveQualifySkipsWithoutGrant pins the grant gate: an unset or any other
// value never grants, so the live proof cannot run by accident.
func TestLiveQualifySkipsWithoutGrant(t *testing.T) {
	for _, value := range []string{"", "0", "true", "bogus", "1 "} {
		t.Setenv("MYTHHELM_LIVE_QUALIFY", value)
		if liveGranted() {
			t.Errorf("MYTHHELM_LIVE_QUALIFY=%q granted the live proof", value)
		}
	}
	t.Setenv("MYTHHELM_LIVE_QUALIFY", "1")
	if !liveGranted() {
		t.Error("MYTHHELM_LIVE_QUALIFY=1 did not grant")
	}
}

// TestLiveQualifyEntitlement runs the authorised entitlement proof against the
// real native Claude Code and records it through Registry.Record. It needs
// MYTHHELM_LIVE_QUALIFY=1, a real `claude` on PATH with a working subscription
// login, MYTHHELM_LIVE_STATE_DIR naming an existing state directory, and the
// maintainer's account observations in MYTHHELM_LIVE_EXTRA_USAGE and
// MYTHHELM_LIVE_PURCHASED_CREDITS. It consumes the maintainer's allowance, so
// CI never sets the tag or the variable, and the invocation is never automated
// (spec claude-strict-subscription Task 7).
func TestLiveQualifyEntitlement(t *testing.T) {
	if !liveGranted() {
		t.Skip("live qualification needs MYTHHELM_LIVE_QUALIFY=1, a real subscription login and the maintainer's grant")
	}
	stateDir := os.Getenv("MYTHHELM_LIVE_STATE_DIR")
	if stateDir == "" {
		t.Fatal("MYTHHELM_LIVE_STATE_DIR must name the state directory to record into")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	a := New()
	workdir := t.TempDir()
	probe, err := a.Probe(ctx, adapter.ProbeInput{Workdir: workdir})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	env, err := security.BuildEnv(os.Environ(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := AuthStatus(ctx, probe, env, workdir)
	if err != nil {
		t.Fatalf("auth status: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := InventorySettings(home, workdir)
	if err != nil {
		t.Fatalf("settings inventory: %v", err)
	}
	cfg, err := InventoryEffective(probe, manifest, auth)
	if err != nil {
		t.Fatalf("effective inventory: %v", err)
	}
	cfg.ExtraUsage = os.Getenv("MYTHHELM_LIVE_EXTRA_USAGE")
	cfg.PurchasedCredits = os.Getenv("MYTHHELM_LIVE_PURCHASED_CREDITS")

	lp, err := a.Prepare(ctx, adapter.PrepareInput{
		Workdir:   workdir,
		AttemptID: "att_live_qualify",
		Env:       env,
		Prompt:    strings.NewReader("Reply with exactly this line and nothing else: live qualify ok. Do not use any tools."),
		Probe:     probe,
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	sess, err := a.Start(ctx, lp, &stubLauncher{t: t})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	scan := scanObservations(sess.Observations())
	<-sess.Done()
	if !scan.sawResult || scan.sawRateLimit {
		t.Fatalf("session did not complete on the included plan: result=%v rate_limit=%v", scan.sawResult, scan.sawRateLimit)
	}

	if !scan.sawStarted {
		t.Fatal("no session-start observation: the plugin and MCP inventory is unassessed")
	}
	routes := InventoryAuxiliary(manifest, scan.started)
	for i := range routes {
		if strings.HasPrefix(routes[i].Name, "plugin:") || strings.HasPrefix(routes[i].Name, "mcp:") {
			continue
		}
		routes[i].Funding = "included"
		routes[i].Evidence = "live-qualify:" + probe.Version
	}
	key := firstRouteFixtureKey()
	key.ExecutableDigest = "sha256:" + probe.SHA256
	key.ConfigDigest = qualify.ConfigDigestOf(map[string]string{"manifest": manifest.Digest})
	key.AuthCategory = auth.AuthMethod + "/" + auth.SubscriptionType
	rec, err := LiveRecord(key, cfg, routes)
	if err != nil {
		t.Fatalf("live record: %v", err)
	}
	reg, err := qualify.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer reg.Close()
	if err := reg.Record(ctx, rec); err != nil {
		t.Fatalf("record: %v", err)
	}
	t.Logf("recorded live-qualified first-route entitlement for native %s", probe.Version)
}
