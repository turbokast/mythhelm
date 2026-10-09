package contain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPolicyRejectsWholeHomeBind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	secret := filepath.Join(home, ".config", "tool", "token.json")
	work := filepath.Join(t.TempDir(), "work")
	root := filepath.VolumeName(home) + string(filepath.Separator)

	tests := []struct {
		name    string
		source  string
		wantErr bool
	}{
		{"home itself", home, true},
		{"home with trailing separator", home + string(filepath.Separator), true},
		{"root", root, true},
		{"ancestor of home", filepath.Dir(home), true},
		{"relative source", "token.json", true},
		{"inside the workdir", filepath.Join(work, "secret"), true},
		{"single file under home", secret, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binds := []AuthBind{{Source: tc.source, Target: filepath.Join(home, "token")}}
			p, err := PolicyFor("restricted", work, false, binds, "")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("PolicyFor accepted bind source %q, got %+v", tc.source, p)
				}
				return
			}
			if err != nil {
				t.Fatalf("PolicyFor rejected single-file bind: %v", err)
			}
			if len(p.AuthBinds) != 1 || p.AuthBinds[0].Source != secret {
				t.Fatalf("AuthBinds = %+v, want the one admitted bind", p.AuthBinds)
			}
		})
	}
}

func TestPolicyForCarriesInputs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := filepath.Join(t.TempDir(), "work", "tree")
	p, err := PolicyFor("inspect", work, true, nil, "127.0.0.1:9")
	if err != nil {
		t.Fatalf("PolicyFor: %v", err)
	}
	want := Policy{Profile: "inspect", Workdir: work, ReadOnly: true, ProxyAddr: "127.0.0.1:9"}
	if p.Profile != want.Profile || p.Workdir != want.Workdir || p.ReadOnly != want.ReadOnly || p.ProxyAddr != want.ProxyAddr || len(p.AuthBinds) != 0 {
		t.Fatalf("PolicyFor = %+v, want %+v", p, want)
	}
}

func TestPolicyForRejectsMalformedInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := filepath.Join(t.TempDir(), "work")
	src := filepath.Join(t.TempDir(), "src")
	tests := []struct {
		name, profile, workdir string
		binds                  []AuthBind
	}{
		{"empty profile", "", "/work", nil},
		{"empty workdir", "restricted", "", nil},
		{"relative workdir", "restricted", "work", nil},
		{"relative bind target", "restricted", work, []AuthBind{{Source: src, Target: "token"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if p, err := PolicyFor(tc.profile, tc.workdir, false, tc.binds, ""); err == nil {
				t.Fatalf("PolicyFor accepted %s: %+v", tc.name, p)
			}
		})
	}
}

func TestPolicyRejectsWorkdirEnclosingHomeOrTmp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.VolumeName(home) + string(filepath.Separator)
	tests := []struct {
		name, workdir string
		wantErr       bool
	}{
		{"workdir is HOME", home, true},
		{"workdir is an ancestor of HOME", filepath.Dir(home), true},
		{"HOME beneath the workdir", filepath.Join(home, "work"), false},
		{"workdir is the filesystem root", root, true},
		{"sibling of HOME", filepath.Join(filepath.Dir(home), "elsewhere"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := PolicyFor("restricted", tc.workdir, false, nil, "")
			if tc.wantErr && err == nil {
				t.Fatalf("PolicyFor accepted workdir %q: %+v", tc.workdir, p)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("PolicyFor rejected workdir %q: %v", tc.workdir, err)
			}
		})
	}
}

func TestPolicyResolvesSymlinksBeforeChecks(t *testing.T) {
	parent := t.TempDir()
	realHome := filepath.Join(parent, "user")
	if err := os.Mkdir(realHome, 0o750); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	t.Setenv("HOME", realHome)

	if p, err := PolicyFor("restricted", filepath.Join(alias, "user"), false, nil, ""); err == nil {
		t.Fatalf("PolicyFor accepted a workdir that resolves to HOME: %+v", p)
	}

	sibling := filepath.Join(parent, "work")
	if err := os.Mkdir(sibling, 0o750); err != nil {
		t.Fatal(err)
	}
	p, err := PolicyFor("restricted", filepath.Join(alias, "work"), false, nil, "")
	if err != nil {
		t.Fatalf("PolicyFor rejected a workdir behind a symlinked ancestor: %v", err)
	}
	want, err := filepath.EvalSymlinks(sibling)
	if err != nil {
		t.Fatal(err)
	}
	if p.Workdir != want {
		t.Fatalf("Workdir = %q, want the resolved %q", p.Workdir, want)
	}
}
