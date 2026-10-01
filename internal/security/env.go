package security

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
)

// ErrDeniedPassthrough reports a passthrough (or MYTHHELM-set) variable that
// the credential denylist forbids. It is a configuration error: exit 2.
var ErrDeniedPassthrough = errors.New("environment variable is on the credential denylist")

// allowedEnv is the child environment allowlist for every platform (design §6.3).
var allowedEnv = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "TMPDIR", "TERM",
	"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR",
	"CLAUDE_CONFIG_DIR",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
}

// allowedEnvWindows is added to allowedEnv on Windows.
var allowedEnvWindows = []string{
	"USERPROFILE", "APPDATA", "LOCALAPPDATA", "SystemRoot", "SystemDrive", "ComSpec", "PATHEXT", "TEMP", "TMP",
}

// deniedEnvPrefixes always win over the allowlist, passthrough and set.
// They are matched case-insensitively on every platform.
var deniedEnvPrefixes = []string{"ANTHROPIC_", "CLAUDE_CODE_USE_", "AWS_", "GOOGLE_", "AZURE_", "OPENAI_"}

var deniedEnvNames = []string{"CLAUDE_CODE_OAUTH_TOKEN"}

func deniedEnv(name string) bool {
	upper := strings.ToUpper(name)
	return slices.Contains(deniedEnvNames, upper) || slices.ContainsFunc(deniedEnvPrefixes, func(p string) bool {
		return strings.HasPrefix(upper, p)
	})
}

// CredentialEnvNames reports the names (never the values) of the entries of
// env that the credential denylist covers, sorted and deduplicated. Doctor
// uses it to name the credential routes without printing secrets.
func CredentialEnvNames(env []string) []string {
	var names []string
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "" || !deniedEnv(name) {
			continue
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// BuildEnv returns the child environment: the variables from parent ("K=V"
// entries, later duplicates winning) that are on the platform allowlist or
// named in passthrough, overlaid with set, sorted by name. A passthrough or set
// name on the credential denylist is an ErrDeniedPassthrough error.
//
// CLAUDE_CODE_OAUTH_TOKEN is always denied here. The AC-4.7 user-level opt-in
// is the Claude Code adapter's to apply, by copying that one variable from the
// parent environment after BuildEnv.
func BuildEnv(parent []string, passthrough []string, set map[string]string) ([]string, error) {
	return buildEnv(runtime.GOOS, parent, passthrough, set)
}

func buildEnv(goos string, parent []string, passthrough []string, set map[string]string) ([]string, error) {
	for _, name := range passthrough {
		if deniedEnv(name) {
			return nil, fmt.Errorf("passthrough %s: %w", name, ErrDeniedPassthrough)
		}
	}
	for name := range set {
		if deniedEnv(name) {
			return nil, fmt.Errorf("set %s: %w", name, ErrDeniedPassthrough)
		}
	}

	// Windows variable names are case-insensitive, so names are compared by
	// their canonical (upper-case) form there; the parent's spelling is kept.
	canon := func(name string) string { return name }
	if goos == "windows" {
		canon = strings.ToUpper
	}
	allowed := map[string]bool{}
	for _, lists := range [][]string{allowedEnv, passthrough} {
		for _, name := range lists {
			allowed[canon(name)] = true
		}
	}
	if goos == "windows" {
		for _, name := range allowedEnvWindows {
			allowed[canon(name)] = true
		}
	}

	type entry struct{ name, value string }
	vars := map[string]entry{}
	for _, kv := range parent {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || name == "" || !allowed[canon(name)] || deniedEnv(name) {
			continue
		}
		vars[canon(name)] = entry{name, value}
	}
	for name, value := range set {
		vars[canon(name)] = entry{name, value}
	}

	env := make([]string, 0, len(vars))
	for _, e := range vars {
		env = append(env, e.name+"="+e.value)
	}
	slices.Sort(env)
	return env, nil
}
