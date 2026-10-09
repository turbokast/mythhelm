package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/contain"
)

// projectConfigSources are inventoried from the admitted revision's committed
// blobs, never from disk: immutable, so the pre-exec re-hash skips them. A
// workdir file hashed against a blob digest would mismatch on identical
// content, so skipping is correctness, not leniency.
var projectConfigSources = map[string]bool{"project": true, "project_local": true, "project_mcp": true}

// userMCPSource carries no whole-file digest: its manifest digest covers the
// selected MCP subset, so it is re-verified by path presence plus
// re-inventory, never by hashing the file.
const userMCPSource = "user_mcp"

// spawnHooks are what a contained spawn needs around cmd.Start and cmd.Wait.
// The zero value, used for an uncontained native, does nothing.
type spawnHooks struct {
	afterStart func() // the worker drops its copy of the prompt pipe's read end
	cleanup    func() // stops the egress proxy and the prompt copier
}

func (h spawnHooks) started() {
	if h.afterStart != nil {
		h.afterStart()
	}
}

func (h spawnHooks) stop() {
	if h.cleanup != nil {
		h.cleanup()
	}
}

// command builds the process the worker starts for spec: the native itself, or
// `mythhelm __contain` wrapping it when the launch carries a containment
// policy. Either way the worker spawns exactly one process and owns it.
func (l Launch) command(spec adapter.ProcSpec) (*exec.Cmd, spawnHooks, error) {
	if l.Containment == nil {
		cmd := exec.Command(spec.Path, spec.Args...) //nolint:gosec // G204: the admitted argv, no shell
		// A nil Env would inherit the worker's environment; the child gets
		// exactly the admitted one.
		cmd.Dir, cmd.Env, cmd.Stdin = spec.Dir, append([]string{}, spec.Env...), spec.Stdin
		cmd.SysProcAttr = nativeAttr()
		return cmd, spawnHooks{}, nil
	}
	return l.containedCommand(spec)
}

// containedCommand starts the egress proxy, fills Policy.ProxyAddr with its
// address, pins the proxy variables into the child's environment and builds
// `__contain` with the spec on stdin and the prompt on fd 3 (design §2.2-2.3).
func (l Launch) containedCommand(spec adapter.ProcSpec) (*exec.Cmd, spawnHooks, error) {
	attr, err := containAttr()
	if err != nil {
		return nil, spawnHooks{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, spawnHooks{}, fmt.Errorf("locating mythhelm for __contain: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	addr, stopProxy, err := contain.ServeProxy(ctx, l.ProxyAllow)
	if err != nil {
		cancel()
		return nil, spawnHooks{}, err
	}
	policy := *l.Containment
	policy.ProxyAddr = addr
	body, err := json.Marshal(contain.ContainSpec{
		Path:   spec.Path,
		Args:   append([]string{spec.Path}, spec.Args...),
		Dir:    spec.Dir,
		Env:    withEnv(spec.Env, contain.ProxyEnv(addr)),
		Policy: policy,
	})
	if err != nil {
		stopProxy()
		cancel()
		return nil, spawnHooks{}, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		stopProxy()
		cancel()
		return nil, spawnHooks{}, err
	}
	// The copier owns a duplicate of the prompt file: the worker closes its
	// own handle as soon as Launch returns, while a slow native is still
	// reading.
	src := spec.Stdin
	if f, ok := src.(*os.File); ok {
		dup, err := dupFile(f)
		if err != nil {
			stopProxy()
			cancel()
			_ = r.Close()
			_ = w.Close()
			return nil, spawnHooks{}, fmt.Errorf("duplicating the prompt: %w", err)
		}
		src = dup
	}
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		defer func() { _ = w.Close() }()
		if c, ok := src.(io.Closer); ok && src != spec.Stdin {
			defer func() { _ = c.Close() }()
		}
		if src != nil {
			_, _ = io.Copy(w, src) // a short read surfaces as the native's own EOF
		}
	}()

	cmd := exec.Command(exe, contain.Command) //nolint:gosec // G204: re-executes this binary with a fixed argv
	cmd.Dir = string(filepath.Separator)
	cmd.Env = []string{}
	cmd.Stdin = bytes.NewReader(body)
	cmd.ExtraFiles = []*os.File{r}
	cmd.SysProcAttr = attr
	return cmd, spawnHooks{
		afterStart: func() { _ = r.Close() },
		cleanup: func() {
			stopProxy()
			cancel()
			_ = r.Close()
			_ = w.Close() // unblocks a copier still writing to a native that exited
			<-copied
		},
	}, nil
}

// verifyUserConfig re-verifies the mutable native-config files behind the
// admitted digests before exec, closing the admission→exec TOCTOU window
// (design §2.9, I20). Every digest key needs a path mapping — a missing
// mapping fails the launch, never skips — and every mapped mutable source
// must still match its admitted digest. It returns nil when the launch
// carries no digests. Errors name sources and paths, never file content.
func (l Launch) verifyUserConfig() error {
	if len(l.UserConfigDigests) == 0 {
		return nil
	}
	for _, source := range slices.Sorted(maps.Keys(l.UserConfigDigests)) {
		want := l.UserConfigDigests[source]
		if want == "" {
			return fmt.Errorf("native config %q carries no admitted digest", source)
		}
		path, ok := l.UserConfigPaths[source]
		if !ok || path == "" {
			return fmt.Errorf("native config %q has no inventoried path to re-verify", source)
		}
		if projectConfigSources[source] {
			continue
		}
		if source == userMCPSource {
			if err := l.verifyUserMCP(path, want); err != nil {
				return err
			}
			continue
		}
		digest, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("re-hashing native config %q: %w", source, err)
		}
		if digest != want {
			return fmt.Errorf("native config %q changed after admission", source)
		}
	}
	return nil
}

// verifyUserMCP re-verifies the user MCP selection: the inventoried file
// must still be a regular file, and a fresh inventory over the launch's own
// workdir and child env must select the admitted digest.
func (l Launch) verifyUserMCP(path, want string) error {
	st, err := os.Stat(path)
	switch {
	case err != nil:
		return fmt.Errorf("re-verifying native config %q: %w", userMCPSource, err)
	case !st.Mode().IsRegular():
		return fmt.Errorf("re-verifying native config %q: not a regular file", userMCPSource)
	}
	home := launchHome(l.Env)
	if home == "" {
		return fmt.Errorf("re-verifying native config %q needs HOME in the launch env", userMCPSource)
	}
	manifest, err := claudecode.InventorySettingsForEnv(home, l.Dir, l.Env)
	if err != nil {
		return fmt.Errorf("re-inventorying native config %q: %w", userMCPSource, err)
	}
	if manifest.Digests[userMCPSource] != want {
		return fmt.Errorf("native config %q changed after admission", userMCPSource)
	}
	return nil
}

// launchHome is the HOME entry of the admitted child env, or "" when the
// launch carries none. Later duplicates win, matching security.BuildEnv.
func launchHome(env []string) string {
	home := ""
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, "HOME="); ok {
			home = value
		}
	}
	return home
}

// withEnv returns env with each pin set, replacing any existing entry for the
// same key, in sorted key order.
func withEnv(env []string, pins map[string]string) []string {
	out := make([]string, 0, len(env)+len(pins))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if _, pinned := pins[key]; !pinned {
			out = append(out, kv)
		}
	}
	keys := make([]string, 0, len(pins))
	for k := range pins {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		out = append(out, k+"="+pins[k])
	}
	return out
}
