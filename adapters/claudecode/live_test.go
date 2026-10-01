//go:build live

package claudecode

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/security"
)

// TestLiveClaudeCanary runs one trivial task against the real native Claude
// Code for Task 20's review. It needs MYTHHELM_LIVE_CLAUDE=1, a real `claude`
// on PATH with a working subscription login, and the maintainer's explicit
// invocation: it consumes the maintainer's allowance, so CI never sets the
// tag or the variable, and the invocation is never automated.
func TestLiveClaudeCanary(t *testing.T) {
	if os.Getenv("MYTHHELM_LIVE_CLAUDE") != "1" {
		t.Skip("live canary needs MYTHHELM_LIVE_CLAUDE=1 and a real subscription login")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	a := New()
	probe, err := a.Probe(ctx, adapter.ProbeInput{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	env, err := security.BuildEnv(os.Environ(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	const prompt = "Reply with exactly this line and nothing else: live canary ok. Do not use any tools."
	lp, err := a.Prepare(ctx, adapter.PrepareInput{
		Workdir:   t.TempDir(),
		AttemptID: "att_live_canary",
		Env:       env,
		Prompt:    strings.NewReader(prompt),
		Probe:     probe,
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	sess, err := a.Start(ctx, lp, &stubLauncher{t: t})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var sawSession, sawResult bool
	for ob := range sess.Observations() {
		switch ob.(type) {
		case adapter.SessionStarted:
			sawSession = true
		case adapter.Result:
			sawResult = true
		}
	}
	exit := <-sess.Done()
	if !sawSession || !sawResult {
		t.Fatalf("canary observed session=%v result=%v exit=%+v", sawSession, sawResult, exit)
	}
	// The sanitised recording is written by hand from this run's observed
	// stream after the maintainer reviews it for secrets (Task 20):
	// automation must never write native text into the repository.
	t.Logf("live canary exit=%+v; record the stream manually after review", exit)
}
