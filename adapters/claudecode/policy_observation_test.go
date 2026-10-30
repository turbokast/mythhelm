package claudecode_test

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

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
			got, err := claudecode.ObservePolicySources(context.Background(), syntheticPolicyReader(func(_ context.Context, _ claudecode.PolicySourceKind) (claudecode.PolicySourceObservation, error) {
				return claudecode.PolicySourceObservation{Kind: tc.kind, State: tc.state}, nil
			}), tc.kinds)
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
	for _, tc := range []struct {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkPolicyObservationSource(tc.path, []byte(tc.source))
			// I02/I15 (v2 §2): proposed observations cannot become admission proof.
			if (err != nil) != tc.reject {
				t.Fatalf("guard result = %v; rejection wanted %v", err, tc.reject)
			}
		})
	}
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	sourceRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sourceRoot.Close(); err != nil {
			t.Error(err)
		}
	})
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || rel == ".claude/data" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source guard refuses symlink: %s", rel)
		}
		source, err := sourceRoot.ReadFile(rel)
		if err != nil {
			return err
		}
		return checkPolicyObservationSource(rel, source)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkPolicyObservationSource(path string, source []byte) error {
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
