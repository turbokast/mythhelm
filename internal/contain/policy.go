package contain

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProbeEnv marks a process as the probe child. ProbeLinux starts the current
// executable with it set inside fresh user and mount namespaces; the
// executable's entry point must call RunProbeChild when it is set.
const ProbeEnv = "MYTHHELM_CONTAIN_PROBE"

const boundaryName = "mythhelm-restricted/1"

// ContainSpec is what __contain receives on stdin: the native to exec, with
// Args as its full argv, and the Policy it runs under.
type ContainSpec struct { //nolint:revive // the name is part of the design §4 contract {
	Path   string   `json:"path"`
	Args   []string `json:"args"`
	Dir    string   `json:"dir"`
	Env    []string `json:"env"`
	Policy Policy   `json:"policy"`
}

// PolicyFor builds the Policy for one worker. An AuthBind must name a single
// absolute source path that is neither $HOME nor an ancestor of it, so a whole
// credential store never enters the boundary.
func PolicyFor(profile, workdir string, readonly bool, binds []AuthBind, proxy string) (Policy, error) {
	if profile == "" {
		return Policy{}, errors.New("contain: empty profile")
	}
	if !filepath.IsAbs(workdir) {
		return Policy{}, fmt.Errorf("contain: workdir %q is not absolute", workdir)
	}
	home, err := resolveExisting(os.Getenv("HOME"))
	if err != nil {
		return Policy{}, fmt.Errorf("contain: resolve HOME: %w", err)
	}
	if workdir, err = resolveExisting(workdir); err != nil {
		return Policy{}, fmt.Errorf("contain: resolve workdir: %w", err)
	}
	if (home != "." && within(home, workdir)) || within("/tmp", workdir) {
		return Policy{}, fmt.Errorf("contain: workdir %q encloses $HOME or /tmp", workdir)
	}
	admitted := make([]AuthBind, 0, len(binds))
	for _, b := range binds {
		if !filepath.IsAbs(b.Source) || !filepath.IsAbs(b.Target) {
			return Policy{}, fmt.Errorf("contain: auth bind %q -> %q needs absolute paths", b.Source, b.Target)
		}
		source := filepath.Clean(b.Source)
		if source == filepath.VolumeName(source)+string(filepath.Separator) || (home != "." && within(home, source)) {
			return Policy{}, fmt.Errorf("contain: auth bind source %q would expose $HOME; bind single files", source)
		}
		if within(source, workdir) {
			return Policy{}, fmt.Errorf("contain: auth bind source %q lies inside the workdir", source)
		}
		admitted = append(admitted, AuthBind{Source: source, Target: filepath.Clean(b.Target)})
	}
	return Policy{
		Profile:   profile,
		Workdir:   workdir,
		ReadOnly:  readonly,
		AuthBinds: admitted,
		ProxyAddr: proxy,
	}, nil
}

// resolveExisting follows symlinks in path so overlap checks compare real
// locations. A path that does not exist yet is kept as written; EnterLinux
// requires it to exist and resolves it again.
func resolveExisting(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return filepath.Clean(path), nil
	}
	return resolved, err
}

// within reports whether path equals dir or lies beneath it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// mountOps is the privileged surface of the boundary setup, so a test can
// deny one step. Linux implements it with mount(2) and mount_setattr(2).
type mountOps interface {
	Private() error
	ReadOnlyTree() error
	Tmpfs(dir string, mode uint32) error
	Release() (string, error)
}

// isolateRoot stops mount events reaching the host and makes every existing
// mount read-only.
func isolateRoot(ops mountOps) error {
	if err := ops.Private(); err != nil {
		return fmt.Errorf("propagation: %w", err)
	}
	if err := ops.ReadOnlyTree(); err != nil {
		return fmt.Errorf("readonly: %w", err)
	}
	return nil
}

// probeChild runs the setup steps the boundary depends on against a scratch
// /tmp and returns the kernel release.
func probeChild(ops mountOps) (string, error) {
	if err := isolateRoot(ops); err != nil {
		return "", err
	}
	if err := ops.Tmpfs("/tmp", 0o1777); err != nil {
		return "", fmt.Errorf("tmpfs: %w", err)
	}
	return ops.Release()
}

// probeResult turns the probe child's output and exit into an Availability.
func probeResult(out []byte, runErr error) Availability {
	msg := strings.TrimSpace(string(out))
	if runErr == nil {
		return Availability{Supported: true, Version: boundaryName + " linux " + msg}
	}
	if _, ok := errors.AsType[*exec.ExitError](runErr); !ok {
		return Availability{Reason: fmt.Sprintf("user namespaces unavailable: %v: %s", runErr, msg)}
	}
	return Availability{Reason: "boundary setup denied: " + msg}
}
