package claudecode_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
)

type syntheticPolicyReader func(context.Context, claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error)

func (f syntheticPolicyReader) Observe(ctx context.Context, kind claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
	return f(ctx, kind)
}
func policyKinds() []claudecode.PolicySourceKind {
	return []claudecode.PolicySourceKind{"remote-cache", "macos-managed-preferences", "windows-machine-policy", "windows-user-policy", "managed-files", "managed-mcp", "policy-helper", "host-parent-settings"}
}

func TestPolicyObservationStates(t *testing.T) {
	t.Parallel()
	for _, state := range []claudecode.PolicySourceState{"present", "absent", "unreadable", "unsupported", "unknown"} {
		t.Run(string(state), func(t *testing.T) {
			kinds := policyKinds()
			want := make([]claudecode.PolicySourceObservation, len(kinds))
			for i, kind := range kinds {
				want[i] = claudecode.PolicySourceObservation{Kind: kind, State: state}
			}
			got, err := claudecode.ObservePolicySources(context.Background(), syntheticPolicyReader(func(_ context.Context, kind claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
				return claudecode.PolicySourceObservation{Kind: kind, State: state}, nil
			}), kinds)
			// I09 (v2 §2): preserve the exact source identity and observation state.
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("observations = %#v, %v; want %#v", got, err, want)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		kinds []claudecode.PolicySourceKind
		kind  claudecode.PolicySourceKind
		state claudecode.PolicySourceState
	}{
		{name: "unknown request", kinds: []claudecode.PolicySourceKind{"unexpected"}},
		{name: "duplicate request", kinds: []claudecode.PolicySourceKind{"remote-cache", "remote-cache"}},
		{name: "reverse request", kinds: []claudecode.PolicySourceKind{"managed-files", "remote-cache"}},
		{name: "invalid returned kind", kinds: []claudecode.PolicySourceKind{"remote-cache"}, kind: "unexpected", state: "present"},
		{name: "swapped valid kind", kinds: []claudecode.PolicySourceKind{"remote-cache"}, kind: "managed-files", state: "present"},
		{name: "invalid state", kinds: []claudecode.PolicySourceKind{"remote-cache"}, kind: "remote-cache", state: "unexpected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, err := claudecode.ObservePolicySources(context.Background(), syntheticPolicyReader(func(_ context.Context, requested claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
				calls++
				if tc.state == "" {
					return claudecode.PolicySourceObservation{Kind: requested, State: "present"}, nil
				}
				return claudecode.PolicySourceObservation{Kind: tc.kind, State: tc.state}, nil
			}), tc.kinds)
			if tc.state == "" && calls != 0 {
				t.Fatalf("invalid request reached reader %d times", calls)
			}
			if len(got) != 0 || !errors.Is(err, claudecode.ErrPolicyObservation) {
				t.Fatalf("invalid observation = %#v, %v", got, err)
			}
		})
	}
}

func TestPolicyObservationRequestSnapshot(t *testing.T) {
	t.Parallel()
	kinds := []claudecode.PolicySourceKind{"remote-cache", "managed-files"}
	want := []claudecode.PolicySourceObservation{{Kind: "remote-cache", State: "present"}, {Kind: "managed-files", State: "present"}}
	reader := syntheticPolicyReader(func(_ context.Context, kind claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
		kinds[1] = "unreviewed"
		return claudecode.PolicySourceObservation{Kind: kind, State: "present"}, nil
	})
	got, err := claudecode.ObservePolicySources(context.Background(), reader, kinds)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("mutated request result = %#v, %v; want %#v", got, err, want)
	}
}

func TestPolicyObservationLateFailureDiscardsResults(t *testing.T) {
	t.Parallel()
	injected := errors.New("synthetic late failure")
	for _, mode := range []string{"reader error", "invalid state", "mismatched kind"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			reader := syntheticPolicyReader(func(_ context.Context, kind claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
				calls++
				out := claudecode.PolicySourceObservation{Kind: kind, State: "present"}
				if calls == 2 {
					switch mode {
					case "reader error":
						return out, injected
					case "invalid state":
						out.State = "unreviewed"
					case "mismatched kind":
						out.Kind = "policy-helper"
					}
				}
				return out, nil
			})
			got, err := claudecode.ObservePolicySources(context.Background(), reader, policyKinds()[:3])
			if len(got) != 0 || !errors.Is(err, claudecode.ErrPolicyObservation) || calls != 2 {
				t.Fatalf("late failure result = %#v, %v, calls=%d", got, err, calls)
			}
			if mode == "reader error" && !errors.Is(err, injected) {
				t.Fatalf("reader error identity lost: %v", err)
			}
		})
	}
}

func TestPolicyObservationUnknownDefault(t *testing.T) {
	t.Parallel()
	kinds := policyKinds()
	want := make([]claudecode.PolicySourceObservation, len(kinds))
	for i, kind := range kinds {
		want[i] = claudecode.PolicySourceObservation{Kind: kind, State: "unknown"}
	}
	got, err := claudecode.ObservePolicySources(context.Background(), nil, kinds)
	// I02 (v2 §2): no reader is unknown, never evidence of absence.
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("nil reader = %#v, %v; want %#v", got, err, want)
	}
	for _, reader := range []claudecode.PolicySourceReader{nil, syntheticPolicyReader(func(context.Context, claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
		t.Fatal("empty request invoked reader")
		return claudecode.PolicySourceObservation{}, nil
	})} {
		got, err = claudecode.ObservePolicySources(context.Background(), reader, nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("empty request = %#v, %v", got, err)
		}
	}
}

func TestPolicyObservationCancellation(t *testing.T) {
	t.Parallel()
	for _, cancelAt := range []int{0, 1, 3} {
		t.Run(strconv.Itoa(cancelAt), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			if cancelAt == 0 {
				cancel()
			}
			reader := syntheticPolicyReader(func(_ context.Context, kind claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
				calls++
				if calls == cancelAt {
					cancel()
				}
				return claudecode.PolicySourceObservation{Kind: kind, State: "present"}, nil
			})
			got, err := claudecode.ObservePolicySources(ctx, reader, policyKinds()[:3])
			if len(got) != 0 || !errors.Is(err, claudecode.ErrPolicyObservation) || !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel result = %#v, %v", got, err)
			}
			if calls != cancelAt {
				t.Fatalf("reader calls = %d; want %d", calls, cancelAt)
			}
		})
	}
	readerErr := errors.New("synthetic reader error")
	got, err := claudecode.ObservePolicySources(context.Background(), syntheticPolicyReader(func(context.Context, claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
		return claudecode.PolicySourceObservation{}, readerErr
	}), policyKinds())
	if len(got) != 0 || !errors.Is(err, claudecode.ErrPolicyObservation) || !errors.Is(err, readerErr) {
		t.Fatalf("reader failure = %#v, %v", got, err)
	}
}

func TestPolicyObservationDoesNotAlterAdmission(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name, path, source string
		reject             bool
	}{
		{"clean helper", "internal/helper/clean.go", "package helper; func Value() int { return 1 }", false},
		{"direct caller", "internal/admission/caller.go", `package admission; import c "github.com/turbokast/mythhelm/adapters/claudecode"; var Run = c.ObservePolicySources`, true},
		{"indirect helper", "internal/helper/caller.go", `package helper; import c "github.com/turbokast/mythhelm/adapters/claudecode"; var Run = c.ObservePolicySources`, true},
		{"dot import", "internal/helper/caller.go", `package helper; import . "github.com/turbokast/mythhelm/adapters/claudecode"`, true},
		{"adapter reader", "adapters/claudecode/reader.go", `package claudecode; var Read = ObservePolicySources`, true},
		{"implementation source read", "adapters/claudecode/policy_observation.go", `package claudecode; import "os"; var Data, Err = os.ReadFile("synthetic")`, true},
		{"implementation concrete reader", "adapters/claudecode/policy_observation.go", `package claudecode; type nativeReader struct{}; func (nativeReader) Observe() {}`, true},
		{"implementation helper source read", "adapters/claudecode/policy_observation.go", `package claudecode; func ObservePolicySources() { readHostSource() }`, true},
		{"implementation shadowed context", "adapters/claudecode/policy_observation.go", `package claudecode; func ObservePolicySources() { ctx := hostContext; ctx.Err() }`, true},
		{"implementation shadowed injection", "adapters/claudecode/policy_observation.go", `package claudecode; func ObservePolicySources() { reader := hostReader; reader.Observe() }`, true},
		{"implementation directive", "adapters/claudecode/policy_observation.go", "package claudecode\n//go:linkname read synthetic.read\nfunc read()", true},
		{"implementation injection", "adapters/claudecode/policy_observation.go", `package claudecode; import "context"; type PolicySourceReader interface { Observe(context.Context) }; func ObservePolicySources(ctx context.Context,reader PolicySourceReader) { reader.Observe(ctx) }`, false},
	}
	fixtureBytes := 0
	if len(fixtures) > 64 {
		t.Fatal("source fixture count limit")
	}
	for _, fixture := range fixtures {
		fixtureBytes += len(fixture.source)
	}
	if fixtureBytes > 1024*1024 {
		t.Fatal("source fixture parse bytes limit")
	}
	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			err := checkPolicyObservationSource(tc.path, []byte(tc.source))
			// I02/I15 (v2 §2): proposed observations cannot become admission proof.
			if (err != nil) != tc.reject {
				t.Fatalf("guard result = %v; rejection wanted %v", err, tc.reject)
			}
		})
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := policyObservationSourceRoot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := scanPolicyObservationTree(ctx, root, defaultPolicyScanLimits()); err != nil {
		t.Fatal(err)
	}
}

func checkPolicyObservationSource(path string, source []byte) error {
	if len(source) > 512*1024 {
		return errors.New("file bytes limit")
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ParseComments)
	if err != nil {
		return err
	}
	implementation := path == "adapters/claudecode/policy_observation.go"
	api := map[string]bool{"ErrPolicyObservation": true, "PolicySourceKind": true, "PolicySourceState": true, "PolicySourceObservation": true, "PolicySourceReader": true, "ObservePolicySources": true}
	aliases := map[string]bool{}
	for _, item := range file.Imports {
		name, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			return err
		}
		if implementation {
			if (name != "context" && name != "errors" && name != "fmt") || item.Name != nil {
				return fmt.Errorf("%s: observation implementation import %s", path, name)
			}
		}
		if name == "github.com/turbokast/mythhelm/adapters/claudecode" {
			alias := "claudecode"
			if item.Name != nil {
				alias = item.Name.Name
			}
			if alias == "." {
				return fmt.Errorf("%s: adapter dot import", path)
			}
			aliases[alias] = true
		}
	}
	if implementation {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.HasPrefix(comment.Text, "//go:") || strings.HasPrefix(comment.Text, "//line ") {
					return fmt.Errorf("%s: compiler directive", path)
				}
			}
		}
		allowedCalls := map[string]bool{"make": true, "len": true, "append": true}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Recv != nil || fn.Name.Name == "init" {
					return fmt.Errorf("%s: concrete reader or initializer", path)
				}
				allowedCalls[fn.Name.Name] = true
			}
		}
		var violation error
		ast.Inspect(file, func(node ast.Node) bool {
			if violation != nil {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			allowed := false
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				allowed = allowedCalls[fun.Name] && (fun.Obj == nil || fun.Obj.Kind == ast.Fun)
			case *ast.SelectorExpr:
				if receiver, ok := fun.X.(*ast.Ident); ok {
					allowed = (policyReaderParameter(receiver) && fun.Sel.Name == "Observe") || (policyContextParameter(receiver) && fun.Sel.Name == "Err") || (receiver.Obj == nil && receiver.Name == "fmt" && fun.Sel.Name == "Errorf") || (receiver.Obj == nil && receiver.Name == "errors" && fun.Sel.Name == "New")
				}
			}
			if !allowed {
				violation = fmt.Errorf("%s: call outside synthetic observation boundary", path)
			}
			return true
		})
		return violation
	}
	var violation error
	samePackage := filepath.ToSlash(filepath.Dir(path)) == "adapters/claudecode"
	ast.Inspect(file, func(node ast.Node) bool {
		if violation != nil {
			return false
		}
		if samePackage {
			if id, ok := node.(*ast.Ident); ok && api[id.Name] {
				violation = fmt.Errorf("%s: production observation reference", path)
				return false
			}
		}
		if selector, ok := node.(*ast.SelectorExpr); ok && api[selector.Sel.Name] {
			if receiver, ok := selector.X.(*ast.Ident); ok && aliases[receiver.Name] {
				violation = fmt.Errorf("%s: production observation reference", path)
				return false
			}
		}
		return true
	})
	return violation
}

// policyReaderParameter excludes a local variable that merely reuses the name.
func policyReaderParameter(id *ast.Ident) bool {
	if id.Name != "reader" || id.Obj == nil {
		return false
	}
	field, ok := id.Obj.Decl.(*ast.Field)
	if !ok {
		return false
	}
	kind, ok := field.Type.(*ast.Ident)
	return ok && kind.Name == "PolicySourceReader"
}

func policyContextParameter(id *ast.Ident) bool {
	if id.Name != "ctx" || id.Obj == nil {
		return false
	}
	field, ok := id.Obj.Decl.(*ast.Field)
	if !ok {
		return false
	}
	kind, ok := field.Type.(*ast.SelectorExpr)
	if !ok || kind.Sel.Name != "Context" {
		return false
	}
	pkg, ok := kind.X.(*ast.Ident)
	return ok && pkg.Name == "context" && pkg.Obj == nil
}

type policyScanLimits struct{ entries, sources, depth, fileBytes, totalBytes int }

func defaultPolicyScanLimits() policyScanLimits {
	return policyScanLimits{entries: 40000, sources: 2048, depth: 64, fileBytes: 512 * 1024, totalBytes: 16 * 1024 * 1024}
}
func policyObservationSourceRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for range 64 {
		root, err := os.OpenRoot(dir)
		if err != nil {
			return "", err
		}
		data, readErr := readPolicySource(root, "go.mod", 64*1024)
		closeErr := root.Close()
		if closeErr != nil {
			return "", closeErr
		}
		if readErr == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == "module" && fields[1] == "github.com/turbokast/mythhelm" {
					return dir, nil
				}
			}
		} else if !errors.Is(readErr, fs.ErrNotExist) {
			return "", readErr
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("MYTHHELM source root not found within ancestor bound")
}

func readPolicySource(root *os.Root, name string, maxBytes int) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxBytes {
		return nil, errors.New("file bytes limit")
	}
	return data, nil
}

func policySourceEntries(ctx context.Context, root *os.Root, dir string, remaining int) (entries []fs.DirEntry, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := root.Open(dir)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return readPolicyEntries(ctx, file, remaining)
}

type policyDirectoryBatchReader interface {
	ReadDir(int) ([]fs.DirEntry, error)
}

func readPolicyEntries(ctx context.Context, file policyDirectoryBatchReader, remaining int) (entries []fs.DirEntry, err error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, readErr := file.ReadDir(64)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(entries)+len(batch) > remaining {
			return nil, errors.New("traversal entries limit")
		}
		entries = append(entries, batch...)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

func scanPolicyObservationTree(ctx context.Context, path string, limits policyScanLimits) (err error) {
	if limits.entries <= 0 || limits.sources <= 0 || limits.depth <= 0 || limits.fileBytes <= 0 || limits.totalBytes <= 0 {
		return errors.New("invalid scan limits")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	type directory struct {
		path  string
		depth int
	}
	pending := []directory{{path: ".", depth: 0}}
	entriesSeen, sourcesSeen, bytesParsed := 0, 0, 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		entries, err := policySourceEntries(ctx, root, current.path, limits.entries-entriesSeen)
		if err != nil {
			return err
		}
		entriesSeen += len(entries)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			rel := filepath.Join(current.path, entry.Name())
			slash := filepath.ToSlash(rel)
			if entry.IsDir() {
				if entry.Name() == ".git" || entry.Name() == "node_modules" || slash == ".claude/data" {
					continue
				}
				if current.depth+1 > limits.depth {
					return errors.New("directory depth limit")
				}
				pending = append(pending, directory{path: rel, depth: current.depth + 1})
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("source guard refuses symlink: %s", slash)
			}
			sourcesSeen++
			if sourcesSeen > limits.sources {
				return errors.New("source count limit")
			}
			source, err := readPolicySource(root, rel, limits.fileBytes)
			if err != nil {
				return err
			}
			bytesParsed += len(source)
			if bytesParsed > limits.totalBytes {
				return errors.New("total parse bytes limit")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := checkPolicyObservationSource(slash, source); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

func TestPolicyObservationSourceRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "adapters", "claudecode")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/turbokast/mythhelm\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := policyObservationSourceRoot(nested)
	if err != nil || got != root {
		t.Fatalf("cwd source root = %q, %v; want %q", got, err, root)
	}
	if _, err := policyObservationSourceRoot(t.TempDir()); err == nil {
		t.Fatal("missing module accepted")
	}
}

func TestPolicyObservationScanBounds(t *testing.T) {
	t.Parallel()
	source := "package fixture\n"
	for _, tc := range []struct {
		name   string
		files  map[string]string
		alter  func(*policyScanLimits)
		reason string
	}{
		{"clean", map[string]string{"one.go": source}, func(*policyScanLimits) {}, ""},
		{"entry count", map[string]string{"one.txt": "", "two.txt": ""}, func(l *policyScanLimits) { l.entries = 1 }, "traversal entries limit"},
		{"source count", map[string]string{"one.go": source, "two.go": source}, func(l *policyScanLimits) { l.sources = 1 }, "source count limit"},
		{"file bytes", map[string]string{"one.go": source + strings.Repeat(" ", 64)}, func(l *policyScanLimits) { l.fileBytes = 64 }, "file bytes limit"},
		{"total bytes", map[string]string{"one.go": source, "two.go": source}, func(l *policyScanLimits) { l.totalBytes = len(source)*2 - 1 }, "total parse bytes limit"},
		{"depth", map[string]string{"one/two/file.go": source}, func(l *policyScanLimits) { l.depth = 1 }, "directory depth limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, data := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			limits := defaultPolicyScanLimits()
			tc.alter(&limits)
			err := scanPolicyObservationTree(context.Background(), root, limits)
			if tc.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("scan = %v; want %s", err, tc.reason)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := scanPolicyObservationTree(ctx, t.TempDir(), defaultPolicyScanLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan = %v", err)
	}
}

func TestPolicyObservationParserBounds(t *testing.T) {
	t.Parallel()
	source := []byte("package fixture\n" + strings.Repeat(" ", 512*1024))
	if err := checkPolicyObservationSource("fixture.go", source); err == nil || !strings.Contains(err.Error(), "file bytes limit") {
		t.Fatalf("oversized fixture = %v", err)
	}
}

type syntheticPolicyDirectoryBatch func(int) ([]fs.DirEntry, error)

func (f syntheticPolicyDirectoryBatch) ReadDir(n int) ([]fs.DirEntry, error) { return f(n) }
func TestPolicyObservationDirectoryBatchCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	reader := syntheticPolicyDirectoryBatch(func(int) ([]fs.DirEntry, error) {
		calls++
		if calls == 1 {
			cancel()
			return nil, nil
		}
		return nil, io.EOF
	})
	entries, err := readPolicyEntries(ctx, reader, 64)
	if len(entries) != 0 || !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancelled batch = %v, %v; reads=%d", entries, err, calls)
	}
}
