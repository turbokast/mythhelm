package supervisor_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestStreamEventSetPinned(t *testing.T) {
	t.Parallel()
	want := []string{
		"admission.decided",
		"apply.completed",
		"apply.intent_recorded",
		"attempt.admission_pinned",
		"attempt.launch_intent_recorded",
		"attempt.launched",
		"attempt.native_error",
		"attempt.native_result",
		"attempt.native_session",
		"attempt.permission_denied",
		"attempt.progress",
		"attempt.protocol_counters",
		"attempt.recovery_decided",
		"attempt.state_changed",
		"attempt.stop_requested",
		"attempt.stopped",
		"candidate.frozen",
		"check.completed",
		"migration.imported",
		"migration.quarantined",
		"receipt.written",
		"recovery.ownership_resolved",
		"run.created",
		"run.extension_granted",
		"run.result",
		"run.state_changed",
		"verification.completed",
		"verification.started",
		"workspace.snapshot_created",
	}
	got := supervisor.StreamEventTypes()
	if !slices.Equal(got, want) {
		t.Fatalf("StreamEventTypes() = %v, want %v", got, want)
	}
	for _, typ := range want {
		if !supervisor.IsStreamEvent(typ) {
			t.Errorf("IsStreamEvent(%q) = false", typ)
		}
	}
	if supervisor.IsStreamEvent("attempt.surprise") {
		t.Error("IsStreamEvent accepted attempt.surprise")
	}
	got[0] = "tampered"
	if again := supervisor.StreamEventTypes(); again[0] != want[0] {
		t.Errorf("StreamEventTypes returned its backing slice: first entry now %q", again[0])
	}
}

// emittedTypes collects the event types a Go source declares or emits
// (design §2.10): the string value of every constant or variable named
// Event*, ev*, *Event or *EventType, and the string literal at the event-type
// position of newEvent (4th argument), a method append (2nd) and applyEvent
// (5th). A type built by an expression is not collected.
// emission keeps source domain/schema with the type; a string-only allowlist
// cannot distinguish an internal record from a public schema1 emitter.
type emission struct {
	typ, form string
	schema    int
}

func internalCanonicalType(typ string) bool {
	// Adopted attachment-control design §§3.2/7; never public stream additions.
	return typ == "service.diagnostic" || typ == "service.adopted"
}

func emissionProblems(records []emission) []string {
	var out []string
	for _, r := range records {
		known := supervisor.IsStreamEvent(r.typ)
		internal := internalCanonicalType(r.typ)
		valid := false
		switch r.form {
		case "declaration":
			valid = known || internal
		case "public":
			valid = known && r.schema == 1
		case "canonical-v2":
			valid = (known || internal) && r.schema == 2
		case "canonical":
			valid = (known && (r.schema == 1 || r.schema == 2)) || (internal && r.schema == 2)
		}
		if !valid {
			out = append(out, fmt.Sprintf("%s type=%q schema=%d", r.form, r.typ, r.schema))
		}
	}
	return out
}

// sourceEmissions inventories concrete declarations, public helpers and direct
// canonical composites. Parameter forwarding is not a new type declaration;
// unresolved non-parameter expressions and unrecognised Event forms fail closed.
func sourceEmissions(t *testing.T, filename string, src any, catalog map[string]string) []emission {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}
	values := map[string]ast.Expr{}
	ast.Inspect(file, func(n ast.Node) bool {
		if v, ok := n.(*ast.ValueSpec); ok {
			for i, name := range v.Names {
				if i < len(v.Values) {
					values[name.Name] = v.Values[i]
				}
			}
		}
		return true
	})
	var stringValue func(ast.Expr, int) (string, bool)
	stringValue = func(e ast.Expr, depth int) (string, bool) {
		if depth > 16 {
			return "", false
		}
		switch e := e.(type) {
		case *ast.BasicLit:
			if e.Kind == token.STRING {
				v, err := strconv.Unquote(e.Value)
				return v, err == nil
			}
		case *ast.Ident:
			if value, ok := values[e.Name]; ok {
				return stringValue(value, depth+1)
			}
			v, ok := catalog[file.Name.Name+"."+e.Name]
			return v, ok
		case *ast.SelectorExpr:
			if pkg, ok := e.X.(*ast.Ident); ok {
				v, ok := catalog[filepath.Base(imports[pkg.Name])+"."+e.Sel.Name]
				return v, ok
			}
		case *ast.ParenExpr:
			return stringValue(e.X, depth+1)
		}
		return "", false
	}
	var schemaValue func(ast.Expr, int) int
	schemaValue = func(e ast.Expr, depth int) int {
		if depth > 16 {
			return -1
		}
		switch e := e.(type) {
		case *ast.BasicLit:
			if e.Kind == token.INT {
				n, err := strconv.Atoi(e.Value)
				if err == nil {
					return n
				}
			}
		case *ast.Ident:
			if value, ok := values[e.Name]; ok {
				return schemaValue(value, depth+1)
			}
		case *ast.SelectorExpr:
			if pkg, ok := e.X.(*ast.Ident); ok {
				switch imports[pkg.Name] + "." + e.Sel.Name {
				case "github.com/turbokast/mythhelm/internal/journal.EnvelopeVersion":
					return 1
				case "github.com/turbokast/mythhelm/internal/v2contract.SchemaVersion":
					return 2
				}
			}
		case *ast.ParenExpr:
			return schemaValue(e.X, depth+1)
		}
		return -1
	}
	var found []emission
	// Preserve deterministic source order for historical public collector fixtures.
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if v, ok := spec.(*ast.ValueSpec); ok {
					for i, name := range v.Names {
						if eventTypeName(name.Name) && i < len(v.Values) {
							typ, ok := stringValue(v.Values[i], 0)
							if !ok {
								typ = "<unresolved>"
							}
							found = append(found, emission{typ, "declaration", 0})
						}
					}
				}
			}
		}
	}
	scan := func(node ast.Node, parameters map[string]bool) {
		ast.Inspect(node, func(n ast.Node) bool {
			forwarded := func(e ast.Expr) bool {
				if id, ok := e.(*ast.Ident); ok {
					return parameters[id.Name]
				}
				if sel, ok := e.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok {
						return parameters[id.Name]
					}
				}
				return false
			}
			switch n := n.(type) {
			case *ast.CallExpr:
				position := -1
				switch fn := n.Fun.(type) {
				case *ast.Ident:
					switch fn.Name {
					case "newEvent":
						position = 3
					case "applyEvent":
						position = 4
					}
				case *ast.SelectorExpr:
					switch fn.Sel.Name {
					case "newEvent":
						position = 3
					case "applyEvent":
						position = 4
					case "append":
						position = 1
					}
				}
				if position >= 0 && position < len(n.Args) {
					typ, ok := stringValue(n.Args[position], 0)
					if ok {
						found = append(found, emission{typ, "public", 1})
					} else if !forwarded(n.Args[position]) {
						// append(ctx,journal,event,projection) is a journal transport, not
						// a type-string emitter. Other unresolved public helper inputs fail.
						if fn, ok := n.Fun.(*ast.SelectorExpr); !ok || fn.Sel.Name != "append" || len(n.Args) != 4 {
							found = append(found, emission{"<unresolved>", "public", 1})
						}
					}
				}
			case *ast.CompositeLit:
				form := "unrecognised"
				eventForm := false
				if sel, ok := n.Type.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Event" || sel.Sel.Name == "Envelope") {
					eventForm = true
					if pkg, ok := sel.X.(*ast.Ident); ok {
						path := imports[pkg.Name]
						if (path == "github.com/turbokast/mythhelm/internal/journal" && sel.Sel.Name == "Event") || (path == "github.com/turbokast/mythhelm/internal/v2contract" && sel.Sel.Name == "Envelope") {
							form = "canonical"
							if sel.Sel.Name == "Envelope" {
								form = "canonical-v2"
							}
						}
					}
				}
				if len(n.Elts) == 0 {
					break
				}
				var typExpr, schemaExpr ast.Expr
				for _, elt := range n.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok {
							switch key.Name {
							case "Type":
								typExpr = kv.Value
							case "SchemaVersion":
								schemaExpr = kv.Value
							}
						}
					}
				}
				if !eventForm {
					name, known := stringValue(typExpr, 0)
					if typExpr == nil || (schemaExpr == nil && (!known || !internalCanonicalType(name))) {
						break
					}
				}
				typ, ok := stringValue(typExpr, 0)
				if !ok && forwarded(typExpr) {
					break
				}
				if !ok {
					typ = "<unresolved>"
				}
				found = append(found, emission{typ, form, schemaValue(schemaExpr, 0)})
			}
			return true
		})
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			params := map[string]bool{}
			if fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					for _, name := range field.Names {
						params[name.Name] = true
					}
				}
			}
			if fn.Body != nil {
				scan(fn.Body, params)
			}
		} else {
			scan(decl, map[string]bool{})
		}
	}
	return found
}

func emittedTypes(t *testing.T, filename string, src any) []string {
	t.Helper()
	var out []string
	for _, r := range sourceEmissions(t, filename, src, nil) {
		out = append(out, r.typ)
	}
	return out
}

func eventTypeName(name string) bool {
	upperAfter := func(prefix string) bool {
		rest, ok := strings.CutPrefix(name, prefix)
		return ok && rest != "" && unicode.IsUpper(rune(rest[0]))
	}
	return upperAfter("Event") || upperAfter("ev") || strings.HasSuffix(name, "Event") || strings.HasSuffix(name, "EventType")
}

func undocumented(types []string) []string {
	var out []string
	for _, typ := range types {
		if !supervisor.IsStreamEvent(typ) {
			out = append(out, typ)
		}
	}
	return out
}

// streamedTypes decodes every stdout line strictly and returns the event types
// in order, failing when a line is not a documented envelope.
func streamedTypes(t *testing.T, stdout, stderr string) []string {
	t.Helper()
	evs, err := decodeEnvelopes(stdout)
	if err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr)
	}
	types := make([]string, len(evs))
	for i, ev := range evs {
		types[i] = ev.Type
	}
	return types
}

func TestNativeErrorStreamsDocumented(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.fakeRun("--format", "jsonl", "--scenario", "allowance-exhausted")...)
	types := streamedTypes(t, stdout, stderr)
	if !slices.Contains(types, "attempt.native_error") {
		t.Fatalf("exit %d: stream %v has no attempt.native_error line; stderr: %s", code, types, stderr)
	}
}

// TestClaudeRateLimitStreamsDocumented is issue #74's case: a claudecode
// rate-limit error is a journaled attempt.native_error, and it streams.
func TestClaudeRateLimitStreamsDocumented(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Exit: 1, Stream: []string{
		claudeInit("none"),
		`{"type":"assistant","error":{"class":"rate_limit"}}`,
		claudeResult("error_retry", true, ""),
	}})
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	types := streamedTypes(t, stdout, stderr)
	if !slices.Contains(types, "attempt.native_error") {
		t.Fatalf("exit %d: stream %v has no attempt.native_error line; stderr: %s", code, types, stderr)
	}
}

// TestRecoveryOwnershipResolvedDocumented: recover replays a run's whole
// history, and the recovery evidence it adds (I12, v2 §2) is documented.
func TestRecoveryOwnershipResolvedDocumented(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux orphan scanner acceptance: the escaped descendant is found by /proc")
	}
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "escapee")
	wait()
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("escaped writer recover %d: %s", code, stderr)
	}
	pids, err := workers.OrphanPIDs(d.AttemptID)
	if err != nil || len(pids) != 1 {
		t.Fatalf("orphan scan %v %v", pids, err)
	}
	// The test owns this scripted descendant and verifies its recorded start
	// time before killing it. Production recovery only observes it.
	var identities []workers.ProcessIdentity
	for _, ev := range f.events(t, d.RunID) {
		if ev.Type == "attempt.stopped" {
			var stop struct {
				Identities []workers.ProcessIdentity `json:"unresolved_identities"`
			}
			if err := json.Unmarshal(ev.Payload, &stop); err != nil {
				t.Fatal(err)
			}
			identities = stop.Identities
		}
	}
	if len(identities) != 1 || identities[0].PID != pids[0] {
		t.Fatalf("descendant identity %v", identities)
	}
	start, err := workers.ProcessStartTime(pids[0])
	if err != nil || !start.Equal(identities[0].StartTime) {
		t.Fatalf("descendant identity changed %v", err)
	}
	proc, err := os.FindProcess(pids[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = proc.Release()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := workers.ProcessStartTime(pids[0]); errors.Is(err, workers.ErrNoProcess) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant still alive")
		}
		time.Sleep(20 * time.Millisecond)
	}

	code, stdout, stderr := f.run(t, "recover", "--format", "jsonl", d.RunID)
	if code != 4 {
		t.Fatalf("resolved partial recovery %d: %s", code, stderr)
	}
	types := streamedTypes(t, stdout, stderr)
	if !slices.Contains(types, "recovery.ownership_resolved") {
		t.Fatalf("recover stream %v has no recovery.ownership_resolved line", types)
	}
}

func TestEveryEmittedEventTypeIsDocumented(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	catalog := map[string]string{}
	for _, dir := range []string{"../supervisor", "../workers", "../control", "../migrate"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if v, ok := n.(*ast.ValueSpec); ok {
					for i, id := range v.Names {
						if i < len(v.Values) {
							if lit, ok := v.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								value, err := strconv.Unquote(lit.Value)
								if err != nil {
									t.Fatal(err)
								}
								catalog[f.Name.Name+"."+id.Name] = value
							}
						}
					}
				}
				return true
			})
		}
	}
	for _, dir := range []string{"../supervisor", "../workers", "../control", "../migrate"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			records := sourceEmissions(t, file, nil, catalog)
			if bad := emissionProblems(records); len(bad) > 0 {
				t.Fatalf("%s: invalid event source forms: %v", file, bad)
			}
			for _, record := range records {
				seen[record.typ] = true
			}
		}
	}
	types := slices.Sorted(maps.Keys(seen))
	t.Logf("scan found %d distinct event types", len(types))
	if len(types) < 25 {
		t.Fatalf("scan found %d distinct event types %v; it is not reaching the emitters", len(types), types)
	}

	t.Run("an undocumented emitter is flagged", func(t *testing.T) {
		t.Parallel()
		src := `package fixture
const surpriseEvent = "attempt.surprise"
func emit(p *pipeline, ctx context.Context) {
	_ = p.append(ctx, "attempt.other_surprise", nil)
	ev, _ := newEvent(runID, "", "", "run.third_surprise", nil, now)
	_ = applyEvent(ctx, j, p, runID, "apply.fourth_surprise", rec)
}
`
		got := undocumented(emittedTypes(t, "fixture.go", src))
		want := []string{"attempt.surprise", "attempt.other_surprise", "run.third_surprise", "apply.fourth_surprise"}
		if !slices.Equal(got, want) {
			t.Fatalf("flagged %v, want %v", got, want)
		}
	})
	t.Run("a payload string is not collected", func(t *testing.T) {
		t.Parallel()
		src := `package fixture
func emit(p *pipeline, ctx context.Context) {
	_ = p.append(ctx, "receipt.written", map[string]any{"schema_version": 1, "path": "receipt.json"})
	out := append(list, "receipt.json")
}
`
		got := emittedTypes(t, "fixture.go", src)
		if !slices.Equal(got, []string{"receipt.written"}) {
			t.Fatalf("collected %v, want only receipt.written", got)
		}
	})
}

// Internal types are adopted in attachment-control design §§3.2/7; they are
// canonical schema2 records, never additions to the public JSONL vocabulary.
func TestCanonicalInternalSourceForms(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"service.diagnostic", "service.adopted"} {
		t.Run(typ, func(t *testing.T) {
			src := fmt.Sprintf(`package fixture
import "github.com/turbokast/mythhelm/internal/journal"
const serviceEventType=%q
func emit(){_=journal.Event{SchemaVersion:2,Type:serviceEventType}}
`, typ)
			records := sourceEmissions(t, "fixture.go", src, nil)
			if len(records) != 2 {
				t.Fatalf("collector=%v; want declaration and canonical emission", records)
			}
			if bad := emissionProblems(records); len(bad) != 0 {
				t.Fatalf("legitimate internal schema2 source rejected: %v", bad)
			}
			if supervisor.IsStreamEvent(typ) {
				t.Fatal("internal type entered public vocabulary")
			}
		})
	}
	for _, expr := range []string{`ledger.Event{SchemaVersion:canonical.SchemaVersion,Type:"service.diagnostic"}`, `canonical.Envelope{SchemaVersion:canonical.SchemaVersion,Type:"service.adopted"}`} {
		src := `package fixture;import ledger "github.com/turbokast/mythhelm/internal/journal";import canonical "github.com/turbokast/mythhelm/internal/v2contract";func emit(){_=` + expr + `}`
		records := sourceEmissions(t, "fixture.go", src, nil)
		if len(records) != 1 {
			t.Fatalf("aliased canonical source escaped collection: %v", records)
		}
		if bad := emissionProblems(records); len(bad) != 0 {
			t.Fatalf("valid aliased schema2 source rejected: %v", bad)
		}
	}
	cases := []struct{ name, body string }{
		{"unrecognised record form", `func emit(){_=other.Record{SchemaVersion:2,Type:"service.adopted"}}`},
		{"unqualified event form", `func emit(){_=Event{SchemaVersion:2,Type:"service.diagnostic"}}`},
		{"internal via public", `func emit(){_,_=newEvent(r,"","","service.diagnostic",nil,now)}`},
		{"internal alias via public", `const hiddenEventType="service.adopted";func emit(){_,_=newEvent(r,"","",hiddenEventType,nil,now)}`},
		{"unknown internal", `func emit(){_=journal.Event{SchemaVersion:2,Type:"service.extra"}}`},
		{"schema1 internal", `func emit(){_=journal.Event{SchemaVersion:1,Type:"service.diagnostic"}}`},
		{"future schema", `func emit(){_=journal.Event{SchemaVersion:3,Type:"service.adopted"}}`},
		{"v2 envelope mislabeled schema1", `func emit(){_=canonical.Envelope{SchemaVersion:1,Type:"run.state_changed"}}`},
		{"missing schema", `func emit(){_=journal.Event{Type:"service.adopted"}}`},
		{"unresolved schema", `func emit(){_=journal.Event{SchemaVersion:future(),Type:"service.adopted"}}`},
		{"mislabeled event form", `func emit(){_=other.Event{SchemaVersion:2,Type:"service.diagnostic"}}`},
		{"unknown direct public", `func emit(){_=journal.Event{SchemaVersion:1,Type:"attempt.extra"}}`},
		{"unresolved type", `func emit(){_=journal.Event{SchemaVersion:2,Type:unknown()}}`},
		{"unresolved public append", `func emit(){_=p.append(ctx,unknown(),nil)}`},
		{"unknown declaration", `const hiddenEventType="service.extra"`},
		{"journal schema1 selector", `func emit(){_=journal.Event{SchemaVersion:journal.EnvelopeVersion,Type:"service.adopted"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `package fixture;import "github.com/turbokast/mythhelm/internal/journal";import canonical "github.com/turbokast/mythhelm/internal/v2contract";` + tc.body
			records := sourceEmissions(t, "fixture.go", src, nil)
			if len(records) == 0 {
				t.Fatal("violating fixture escaped collection")
			}
			if bad := emissionProblems(records); len(bad) == 0 {
				t.Fatalf("violating source accepted: %v", records)
			}
		})
	}
}
