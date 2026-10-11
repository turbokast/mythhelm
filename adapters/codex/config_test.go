package codex

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureProject(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "config", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func inventoryBlobs(t *testing.T, blobs map[string][]byte) Manifest {
	t.Helper()
	m, err := InventoryAdmittedProject(t.TempDir(), t.TempDir(), blobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInventoryDigestsChangeWithSource(t *testing.T) { // AC-3.3
	t.Parallel()
	base := map[string][]byte{"project_instructions": []byte("be careful"), "project_config": []byte(`{}`)}
	ref := inventoryBlobs(t, base)
	for name := range ProjectSources {
		changed := map[string][]byte{}
		maps.Copy(changed, base)
		changed[name] = append(slices.Clone(base[name]), '\n')
		m := inventoryBlobs(t, changed)
		if m.Digests[name] == ref.Digests[name] || m.Digests[name] == "" {
			t.Errorf("%s: digest %q did not change from %q", name, m.Digests[name], ref.Digests[name])
		}
		if m.Digest == ref.Digest || m.Digest == "" {
			t.Errorf("%s: combined Digest %q did not change from %q", name, m.Digest, ref.Digest)
		}
	}
	if len(ProjectSources) == 0 {
		t.Fatal("ProjectSources is empty")
	}

	home, workdir := t.TempDir(), t.TempDir()
	user := filepath.Join(home, ".codex", "config.json")
	writeFile(t, user, `{}`)
	before, err := InventorySettingsForEnv(home, workdir, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, user, `{"model":"synthetic"}`)
	after, err := InventorySettingsForEnv(home, workdir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Digests["user_config"] == before.Digests["user_config"] || after.Digest == before.Digest || before.Digests["user_config"] == "" {
		t.Fatalf("user_config digest unchanged: before=%+v after=%+v", before, after)
	}
}

func TestInventoryOpensOnlyEnumeratedFiles(t *testing.T) { // AC-3.4, I19
	t.Parallel()
	home, workdir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".codex", "config.json"), `{}`)
	writeFile(t, filepath.Join(workdir, "AGENTS.md"), "rules")
	sentinels := []string{
		filepath.Join(home, ".codex", "auth.json"),
		filepath.Join(home, ".codex", "credentials.json"),
		filepath.Join(home, ".netrc"),
	}
	for _, s := range sentinels {
		writeFile(t, s, "SENTINEL-CREDENTIAL")
	}
	var opened []string
	read := func(path string) ([]byte, bool, error) {
		opened = append(opened, path)
		return readSource(path)
	}
	env := []string{"CODEX_HOME=" + filepath.Join(home, ".codex")}
	if _, err := inventorySettings(home, workdir, env, nil, read); err != nil {
		t.Fatal(err)
	}
	allowed := AdmittedConfigPathsForEnv(home, workdir, env)
	for _, p := range opened {
		if !slices.Contains(mapValues(allowed), p) {
			t.Errorf("opened %s, which is not an enumerated source", p)
		}
		for _, s := range sentinels {
			if p == s {
				t.Errorf("opened sentinel credential file %s", p)
			}
		}
	}
	if len(opened) == 0 {
		t.Fatal("no source was opened")
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func TestInventoryAndPathsAgree(t *testing.T) {
	t.Parallel()
	home, workdir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(home, ".codex", "config.json"), `{"model":"synthetic"}`)
	writeFile(t, filepath.Join(workdir, "AGENTS.md"), "rules")
	writeFile(t, filepath.Join(workdir, ".codex", "config.json"), string(fixtureProject(t)))
	m, err := InventorySettingsForEnv(home, workdir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Digests) != 3 {
		t.Fatalf("Digests = %v, want the three enumerated sources", m.Digests)
	}
	paths := AdmittedConfigPathsForEnv(home, workdir, nil)
	for key := range m.Digests {
		if paths[key] == "" || !filepath.IsAbs(paths[key]) {
			t.Errorf("manifest key %q has no absolute path entry: %v", key, paths)
		}
	}
	if got := paths["project_config"]; got != filepath.Join(workdir, ".codex", "config.json") {
		t.Errorf("project_config path = %q", got)
	}
	if paths["user_config"] != filepath.Join(home, ".codex", "config.json") {
		t.Errorf("user_config path = %q", paths["user_config"])
	}

	custom := filepath.Join(t.TempDir(), "elsewhere")
	env := []string{"CODEX_HOME=" + custom}
	if got := AdmittedConfigPathsForEnv(home, workdir, env)["user_config"]; got != filepath.Join(custom, "config.json") {
		t.Errorf("CODEX_HOME not honoured: user_config path = %q", got)
	}
	if got := AdmittedConfigPathsForEnv("relative", workdir, nil); len(got) != 0 {
		t.Errorf("non-absolute home yields %v, want empty", got)
	}
}

func TestGapsBlockStrict(t *testing.T) { // AC-3.5, I02
	t.Parallel()
	gaps := GapSources(UnresolvedSources)
	if len(gaps) == 0 || !slices.Equal(gaps, UnresolvedSources) {
		t.Fatalf("GapSources(UnresolvedSources) = %v, want all of %v", gaps, UnresolvedSources)
	}
	in := []string{"zeta", "alpha", "mid"}
	if got := GapSources(in); !slices.Equal(got, in) {
		t.Fatalf("GapSources(%v) = %v, want input order", in, got)
	}
	if got := GapSources(nil); got == nil || len(got) != 0 {
		t.Fatalf("GapSources(nil) = %#v, want empty non-nil", got)
	}
}

func TestHooksCountedAndTrustRequired(t *testing.T) { // AC-3.1
	t.Parallel()
	with := inventoryBlobs(t, map[string][]byte{"project_config": fixtureProject(t)})
	if with.Hooks != 3 || !with.RequiresTrust || !slices.Equal(with.MCPServers, []string{"project_config:synthetic-server"}) {
		t.Fatalf("fixture manifest = %+v, want 3 hooks, one MCP server, trust required", with)
	}
	without := inventoryBlobs(t, map[string][]byte{"project_config": []byte(`{"model":"synthetic"}`), "project_instructions": []byte("rules")})
	if without.Hooks != 0 || without.RequiresTrust || len(without.MCPServers) != 0 {
		t.Fatalf("plain manifest = %+v, want no hooks, MCP or trust", without)
	}
	mcpOnly := inventoryBlobs(t, map[string][]byte{"project_config": []byte(`{"mcp_servers":{"s":{}}}`)})
	if !mcpOnly.RequiresTrust {
		t.Fatal("an MCP server must require trust")
	}
}

func TestInventoryFailsClosed(t *testing.T) {
	t.Parallel()
	home, workdir := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(workdir, ".codex", "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workdir, ".codex", "real"), filepath.Join(workdir, ".codex", "config.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := InventorySettingsForEnv(home, workdir, nil); !errors.Is(err, ErrNonRegularSource) {
		t.Fatalf("symlinked project config: err = %v, want ErrNonRegularSource", err)
	}
	if _, err := InventorySettingsForEnv("relative-home", workdir, nil); !errors.Is(err, ErrCapability) {
		t.Fatalf("relative home: err = %v, want ErrCapability", err)
	}
	blockedHome := t.TempDir()
	writeFile(t, filepath.Join(blockedHome, ".codex"), "not a directory")
	if _, err := InventorySettingsForEnv(blockedHome, t.TempDir(), nil); !errors.Is(err, ErrCapability) {
		t.Fatalf("unreadable home: err = %v, want ErrCapability", err)
	}
	for name, blob := range map[string]string{ //nolint:gosec // Synthetic config fixtures, not credentials.
		"invalid json":      `{`,
		"credential key":    `{"api_key":"SENTINEL"}`,
		"auth method pin":   `{"preferred_auth_method":"apikey"}`,
		"duplicate hooks":   `{"hooks":{},"hooks":{}}`,
		"hooks not objects": `{"hooks":[]}`,
	} {
		if _, err := InventoryAdmittedProject(home, workdir, map[string][]byte{"project_config": []byte(blob)}, nil); err == nil {
			t.Errorf("%s: inventory accepted %s", name, blob)
		} else if strings.Contains(err.Error(), "SENTINEL") {
			t.Errorf("%s: error leaks credential value: %v", name, err)
		}
	}
}

// writesNativeStore reports os.WriteFile, os.Create and os.OpenFile calls.
func writesNativeStore(t *testing.T, name, src string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" && slices.Contains([]string{"WriteFile", "Create", "OpenFile"}, sel.Sel.Name) {
			found = append(found, "os."+sel.Sel.Name)
		}
		return true
	})
	return found
}

func TestNoNativeStoreWrites(t *testing.T) { // AC-6.2, I12
	t.Parallel()
	planted := "package p\nimport \"os\"\nfunc f() { _ = os.WriteFile(\"x\", nil, 0o600) }\n"
	if got := writesNativeStore(t, "planted.go", planted); !slices.Equal(got, []string{"os.WriteFile"}) {
		t.Fatalf("planted write not flagged: %v", got)
	}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no sources found: %v", err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f) //nolint:gosec // Globbed package source file.
		if err != nil {
			t.Fatal(err)
		}
		checked++
		if got := writesNativeStore(t, f, string(raw)); len(got) > 0 {
			t.Errorf("%s writes: %v", f, got)
		}
	}
	if checked == 0 {
		t.Fatal("no non-test source checked")
	}
}
