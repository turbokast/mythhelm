package contain

import (
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
