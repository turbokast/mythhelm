package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/contain"
)

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
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		defer func() { _ = w.Close() }()
		if spec.Stdin != nil {
			_, _ = io.Copy(w, spec.Stdin) // a short read surfaces as the native's own EOF
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
