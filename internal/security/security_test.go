package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// Secret-shaped fixtures are assembled at run time so the repository's
// public-hygiene scan never sees a token literal in this file.
var (
	anthropicKey = "sk-" + "ant-api03-" + strings.Repeat("Ab1_", 8)
	githubPAT    = "github_" + "pat_" + strings.Repeat("A1b2", 10)
	githubOAuth  = "gh" + "o_" + strings.Repeat("Z9y8", 9)
	githubServer = "gh" + "s_" + strings.Repeat("Q7r6", 9)
	awsKeyID     = "AK" + "IA" + strings.Repeat("ABCD2345", 2)
)

func TestRedactKnownSecretShapes(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"anthropic key", "key " + anthropicKey + " end", "key [REDACTED] end"},
		{"bearer", "Authorization: Bearer abc.def-ghi_jkl=", "Authorization: Bearer [REDACTED]"},
		{"bearer lower case", "authorization: bearer abc123", "authorization: bearer [REDACTED]"},
		{"github oauth token", "push with " + githubOAuth, "push with [REDACTED]"},
		{"github server token", "(" + githubServer + ")", "([REDACTED])"},
		{"github fine-grained PAT", "pat=" + githubPAT, "pat=[REDACTED]"},
		{"aws access key id", "id " + awsKeyID + ".", "id [REDACTED]."},
		{"api_key assignment", "api_key=hunter2 next", "api_key=[REDACTED] next"},
		{"apikey colon", "APIKey: hunter2", "APIKey: [REDACTED]"},
		{"token colon", "token : abc", "token : [REDACTED]"},
		{"secret equals", "client_secret=s3cr3t", "client_secret=[REDACTED]"},
		{"password", "PASSWORD=pw", "PASSWORD=[REDACTED]"},
		{"quoted json field", `{"token": "abc", "n": 1}`, `{"token": "[REDACTED]", "n": 1}`},
		{"no secret", "plain text, nothing here", "plain text, nothing here"},
		{"token counts are not secrets", "input_tokens: 42 max_tokens=8", "input_tokens: 42 max_tokens=8"},
		{"gh-like word inside another word", "laughs_out loud", "laughs_out loud"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Redact(tt.in); got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

type stringer string

func (s stringer) String() string { return string(s) }

func TestRedactingHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buf, nil)))
	logger.With("preset", "Bearer tok1", "ANTHROPIC_API_KEY", "raw-value").
		WithGroup("g").
		Info("using "+anthropicKey,
			"auth", "Bearer tok2",
			"err", errors.New("failed: password=pw"),
			"desc", stringer("token=abc"),
			slog.Group("nested", "secret", "raw", "note", "api_key=zzz"),
			"apiKeySource", "none",
			"input_tokens", 12,
			"accessToken", "raw",
		)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("decode %q: %v", buf.String(), err)
	}
	out := buf.String()
	for _, leak := range []string{anthropicKey, "tok1", "tok2", "pw", "abc", "zzz", "raw"} {
		if strings.Contains(out, leak) {
			t.Errorf("log line leaks %q: %s", leak, out)
		}
	}
	if rec["msg"] != "using [REDACTED]" {
		t.Errorf("msg = %v", rec["msg"])
	}
	if rec["preset"] != "Bearer [REDACTED]" {
		t.Errorf("WithAttrs value not redacted: %v", rec["preset"])
	}
	if _, ok := rec["ANTHROPIC_API_KEY"]; ok {
		t.Errorf("credential-named attribute not dropped: %s", out)
	}
	g, _ := rec["g"].(map[string]any)
	want := map[string]any{
		"auth":         "Bearer [REDACTED]",
		"err":          "failed: password=[REDACTED]",
		"desc":         "token=[REDACTED]",
		"apiKeySource": "none",
		"input_tokens": float64(12),
	}
	for k, v := range want {
		if g[k] != v {
			t.Errorf("g.%s = %v, want %v", k, g[k], v)
		}
	}
	if _, ok := g["accessToken"]; ok {
		t.Errorf("accessToken not dropped: %s", out)
	}
	nested, _ := g["nested"].(map[string]any)
	if _, ok := nested["secret"]; ok {
		t.Errorf("nested secret not dropped: %s", out)
	}
	if nested["note"] != "api_key=[REDACTED]" {
		t.Errorf("nested note = %v", nested["note"])
	}
}

func TestCredentialName(t *testing.T) {
	for _, name := range []string{"token", "api_key", "apiKey", "ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_AUTH_TOKEN", "AWS_SECRET_ACCESS_KEY", "password", "client-secret", "credentials", "private_key"} {
		if !credentialName(name) {
			t.Errorf("credentialName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"apiKeySource", "input_tokens", "maxTokens", "token_count", "path", "session_id", "ANTHROPIC_BASE_URL"} {
		if credentialName(name) {
			t.Errorf("credentialName(%q) = true, want false", name)
		}
	}
}

func TestTermSafeStripsEscapesAndC1(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"OSC 8 hyperlink with ST", "see \x1b]8;;https://example.com\x1b\\click\x1b]8;;\x1b\\ now", "see click now"},
		{"OSC 8 hyperlink with BEL", "\x1b]8;;https://example.com\aclick\x1b]8;;\a", "click"},
		{"OSC title", "a\x1b]0;forged title\x07b", "ab"},
		{"OSC holding a rune with a 0x9c byte", "a\x1b]0;✓\ab", "ab"},
		{"CSI colour", "\x1b[1;31mred\x1b[0m", "red"},
		{"CSI cursor movement", "x\x1b[2J\x1b[H\x1b[?25ly", "xy"},
		{"ESC with a final byte", "a\x1bcb", "ab"},
		{"bare ESC before newline", "a\x1b\nb", "a\nb"},
		{"bare ESC before unicode", "a\x1bé", "aé"},
		{"trailing bare ESC", "abc\x1b", "abc"},
		{"ESC with intermediate", "a\x1b(Bb", "ab"},
		{"8-bit C1 CSI byte", "a\x9b31mb", "ab"},
		{"encoded C1 CSI", "a\u009b31mb", "ab"},
		{"encoded C1 OSC", "a\u009d0;t\u009cb", "ab"},
		{"other encoded C1", "a\u0085\u0080b", "ab"},
		{"C0 controls", "a\x00\x07\x08\r\x7fb", "ab"},
		{"newline and tab kept", "a\tb\nc", "a\tb\nc"},
		{"unterminated OSC stops at newline", "a\x1b]0;hidden\nvisible", "a\nvisible"},
		{"aborted CSI keeps following text", "a\x1b[12\nb", "a\nb"},
		{"unicode kept", "résumé ✓ 日本", "résumé ✓ 日本"},
		{"invalid utf-8 replaced", "a\xffb", "a\uFFFDb"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TermSafe(tt.in); got != tt.want {
				t.Errorf("TermSafe(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzTermSafe(f *testing.F) {
	for _, s := range []string{
		"", "plain", "\x1b]8;;https://example.com\x1b\\x\x1b]8;;\x1b\\", "\x1b[31m", "\x1b", "\x9b",
		"\u009b1m", "\u009dx\u009c", "a\tb\nc", "\x1b]0;t\a", "\x1bP1$r\x1b\\", "\xff\xfe", "\x1b(B",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := TermSafe(in)
		if !utf8.ValidString(out) {
			t.Fatalf("TermSafe(%q) = %q is not valid UTF-8", in, out)
		}
		for _, r := range out {
			if (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r <= 0x9f) {
				t.Fatalf("TermSafe(%q) = %q keeps control %U", in, out, r)
			}
		}
		if again := TermSafe(out); again != out {
			t.Fatalf("TermSafe is not idempotent: %q -> %q -> %q", in, out, again)
		}
	})
}

func TestBuildEnvAllowlistOnly(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"HOME=/tmp/home",
		"LANG=C.UTF-8",
		"https_proxy=http://proxy.example.com",
		"EDITOR=vi",
		"GOPATH=/tmp/go",
		"GOFLAGS=-mod=mod",
		"ANTHROPIC_API_KEY=x",
		"CLAUDE_CODE_OAUTH_TOKEN=x",
		"AWS_REGION=x",
		"DISABLE_AUTOUPDATER=0",
		"HOME=/tmp/home2",
		"=C:=C:\\",
		"MALFORMED",
	}
	got, err := BuildEnv(parent, []string{"GOPATH", "GOCACHE"}, map[string]string{
		"DISABLE_AUTOUPDATER": "1",
		"MYTHHELM_ATTEMPT_ID": "att_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DISABLE_AUTOUPDATER=1",
		"GOPATH=/tmp/go",
		"HOME=/tmp/home2",
		"LANG=C.UTF-8",
		"MYTHHELM_ATTEMPT_ID=att_1",
		"PATH=/usr/bin",
		"https_proxy=http://proxy.example.com",
	}
	if !slices.Equal(got, want) {
		t.Errorf("BuildEnv =\n%q\nwant\n%q", got, want)
	}
}

func TestBuildEnvWindowsNamesAreCaseInsensitive(t *testing.T) {
	parent := []string{"Path=C:\\Windows", "SystemRoot=C:\\Windows", "userprofile=C:\\Users\\u", "Anthropic_Api_Key=x", "gopath=C:\\go"}
	got, err := buildEnv("windows", parent, []string{"GOPATH"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Path=C:\\Windows", "SystemRoot=C:\\Windows", "gopath=C:\\go", "userprofile=C:\\Users\\u"}
	if !slices.Equal(got, want) {
		t.Errorf("buildEnv(windows) =\n%q\nwant\n%q", got, want)
	}

	got, err = buildEnv("linux", parent, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("buildEnv(linux) = %q; Windows-only and mixed-case names must not pass", got)
	}
}

func TestBuildEnvDenylistWinsOverPassthrough(t *testing.T) {
	parent := []string{"ANTHROPIC_API_KEY=x", "PATH=/usr/bin"}
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_OAUTH_TOKEN", "AWS_PROFILE", "GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CLIENT_ID", "OPENAI_API_KEY", "anthropic_api_key"} {
		t.Run(name, func(t *testing.T) {
			env, err := BuildEnv(parent, []string{"GOPATH", name}, nil)
			if !errors.Is(err, ErrDeniedPassthrough) {
				t.Fatalf("err = %v, want ErrDeniedPassthrough", err)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name %s", err, name)
			}
			if env != nil {
				t.Errorf("env = %q, want nil on error", env)
			}
		})
	}
	if _, err := BuildEnv(parent, nil, map[string]string{"ANTHROPIC_API_KEY": "x"}); !errors.Is(err, ErrDeniedPassthrough) {
		t.Errorf("denied name in set: err = %v, want ErrDeniedPassthrough", err)
	}
}

func TestResolvesOutsideSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(root, "sub"), outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(root, "file"), filepath.Join(root, "sub", "inner"), filepath.Join(outside, "secret")} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"abs-out":        filepath.Join(outside, "secret"),
		"rel-out":        filepath.Join("..", "outside", "secret"),
		"sub/rel-out":    filepath.Join("..", "..", "outside"),
		"rel-in":         filepath.Join("sub", "inner"),
		"sub/up-in":      filepath.Join("..", "file"),
		"abs-in":         filepath.Join(root, "file"),
		"dangling-out":   filepath.Join(outside, "missing"),
		"dangling-in":    "missing",
		"chain":          "abs-out",
		"dir-out":        outside,
		"loop-a":         "loop-b",
		"loop-b":         "loop-a",
		"sub/to-sub-dir": ".",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	rootLink := filepath.Join(base, "root-link")
	if err := os.Symlink(root, rootLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	tests := []struct {
		name, root, path string
		want             bool
	}{
		{"plain file", root, "file", false},
		{"root itself", root, ".", false},
		{"missing file inside", root, "no/such/file", false},
		{"absolute symlink out", root, "abs-out", true},
		{"relative symlink out", root, "rel-out", true},
		{"nested relative symlink out", root, "sub/rel-out/secret", true},
		{"relative symlink in", root, "rel-in", false},
		{"symlink up but still in", root, "sub/up-in", false},
		{"absolute symlink in", root, "abs-in", false},
		{"dangling symlink out", root, "dangling-out", true},
		{"dangling symlink in", root, "dangling-in", false},
		{"symlink chain out", root, "chain", true},
		{"through a directory symlink", root, "dir-out/secret", true},
		{"missing component then symlink", root, "missing/../abs-out", true},
		{"lexical dot-dot out", root, "../outside/secret", true},
		{"dot-dot after a directory symlink", root, "dir-out/../file", true},
		{"absolute path argument", root, filepath.Join(root, "sub", "inner"), false},
		{"self-referencing dir link", root, "sub/to-sub-dir/to-sub-dir/inner", false},
		{"root given through a symlink", rootLink, "abs-in", false},
		{"root given through a symlink, escape", rootLink, "abs-out", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvesOutside(tt.root, filepath.FromSlash(tt.path))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ResolvesOutside(%s, %s) = %v, want %v", tt.root, tt.path, got, tt.want)
			}
		})
	}

	t.Run("symlink loop is an error", func(t *testing.T) {
		if _, err := ResolvesOutside(root, "loop-a"); err == nil {
			t.Error("want an error for a symlink loop")
		}
	})
	t.Run("missing root is an error", func(t *testing.T) {
		if _, err := ResolvesOutside(filepath.Join(base, "nope"), "x"); err == nil {
			t.Error("want an error for a missing root")
		}
	})
}
