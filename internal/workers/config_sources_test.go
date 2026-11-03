package workers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// identityLaunch is a serialised Codex launch whose admitted manifest holds
// two distinct synthetic files with identical bytes, hence identical digests:
// the user config under HOME and the project config under the workdir.
type identityLaunch struct {
	t                  *testing.T
	a                  attempt
	launch             Launch
	counter            string
	userPath, projPath string
}

func newIdentityLaunch(t *testing.T) identityLaunch {
	t.Helper()
	a := newAttempt(t)
	l := a.codexLaunch(t, "success")
	home := launchHome(l.Env)
	c := identityLaunch{
		t:        t,
		a:        a,
		counter:  filepath.Join(t.TempDir(), "native-starts"),
		userPath: filepath.Join(home, ".codex", "config.json"),
		projPath: filepath.Join(l.Dir, ".codex", "config.json"),
	}
	body := "{}"
	writeConfigFile(t, c.userPath, body)
	writeConfigFile(t, c.projPath, body)
	sum := sha256.Sum256([]byte(body))
	digest := hex.EncodeToString(sum[:])
	l.Env = append(l.Env, nativeCounterEnv+"="+c.counter)
	l.UserConfigDigests = map[string]string{"user_config": digest, "project_config": digest}
	l.UserConfigPaths = map[string]string{"user_config": c.userPath, "project_config": c.projPath}
	c.launch = l
	return c
}

// starts is how many times the fixture native actually started.
func (c identityLaunch) starts(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(c.counter)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "started\n")
}

func TestWorkerConfigSourceIdentity(t *testing.T) { // I20 (v2 §8: a digest is not a source identity)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the Codex fixture native is exercised by the unix worker tests only")
	}
	tests := []struct {
		name       string
		mutate     func(c identityLaunch, paths map[string]string)
		wantStarts int
		wantErr    string
	}{
		{name: "the unchanged exact mapping launches the native once", mutate: func(identityLaunch, map[string]string) {}, wantStarts: 1},
		{name: "swapped paths of equal-digest files refuse before launch", mutate: func(c identityLaunch, p map[string]string) {
			p["user_config"], p["project_config"] = c.projPath, c.userPath
		}, wantErr: `native config "project_config" payload path is not its inventoried path`},
		{name: "a missing admitted source refuses before launch", mutate: func(_ identityLaunch, p map[string]string) {
			delete(p, "project_config")
		}, wantErr: `native config "project_config" has no payload path`},
		{name: "an extra inventoried but unadmitted source refuses before launch", mutate: func(c identityLaunch, p map[string]string) {
			p["project_instructions"] = filepath.Join(c.launch.Dir, "AGENTS.md")
		}, wantErr: `native config payload carries unadmitted source "project_instructions"`},
		{name: "an extra unknown source refuses before launch", mutate: func(c identityLaunch, p map[string]string) {
			p["plugin_cache"] = c.userPath
		}, wantErr: `native config payload carries unadmitted source "plugin_cache"`},
		{name: "the exact mapping still re-hashes a changed admitted file", mutate: func(c identityLaunch, _ map[string]string) {
			writeConfigFile(c.t, c.userPath, `{"model":"changed"}`)
		}, wantErr: `native config "user_config" changed after admission`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newIdentityLaunch(t)
			tc.mutate(c, c.launch.UserConfigPaths)
			proc := c.a.spawn(t, c.launch)
			if tc.wantStarts == 1 {
				c.a.waitDone(t)
			}
			if !waitExit(proc, 30*time.Second) {
				t.Fatal("worker did not exit")
			}
			if got := c.starts(t); got != tc.wantStarts {
				t.Fatalf("native started %d times, want %d (worker log %q)", got, tc.wantStarts, c.a.workerLog())
			}
			if got := launchFailedErr(t, c.a.workerLog()); got != tc.wantErr {
				t.Fatalf("native launch refusal = %q, want %q (worker log %q)", got, tc.wantErr, c.a.workerLog())
			}
		})
	}
}

// launchFailedErr decodes the quoted err attribute of the worker's slog text
// record "native launch failed", or returns "" when the worker logged none.
func launchFailedErr(t *testing.T, log string) string {
	t.Helper()
	for line := range strings.Lines(log) {
		if !strings.Contains(line, ` msg="native launch failed" `) {
			continue
		}
		_, rest, ok := strings.Cut(line, " err=")
		if !ok {
			t.Fatalf("native launch failed record has no err attribute: %q", line)
		}
		quoted, err := strconv.QuotedPrefix(rest)
		if err != nil {
			t.Fatalf("native launch failed err attribute %q is not quoted: %v", rest, err)
		}
		v, err := strconv.Unquote(quoted)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	return ""
}

// TestManagedDigestRehash isolates the digest layer below exact source-identity
// validation, using only a synthetic temporary file rather than host policy.
func TestManagedDigestRehash(t *testing.T) { // I20 (v2 §8): mutable managed bytes remain pinned.
	t.Parallel()
	path := filepath.Join(t.TempDir(), "managed-settings.json")
	const original = "{}\n"
	writeConfigFile(t, path, original)
	sum := sha256.Sum256([]byte(original))
	l := Launch{AdapterID: "builtin/fake", UserConfigPaths: map[string]string{"managed": path}, UserConfigDigests: map[string]string{"managed": hex.EncodeToString(sum[:])}}
	if err := l.verifyUserConfig(); err != nil {
		t.Fatalf("unchanged managed digest: %v", err)
	}
	writeConfigFile(t, path, "{\"env\":{\"X\":\"1\"}}\n")
	if err := l.verifyUserConfig(); err == nil || !strings.Contains(err.Error(), `native config "managed" changed after admission`) {
		t.Fatalf("managed rehash error = %v", err)
	}
}
