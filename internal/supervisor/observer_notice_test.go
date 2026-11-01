package supervisor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func noticeInventory(t *testing.T, name string, src any) map[string]int {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]int{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pos := -1
		switch sel.Sel.Name {
		case "Notice":
			found["UNCLASSIFIED"]++
		case "inform":
			pos = 0
		case "diagnostic":
			pos = 1
		}
		if pos >= 0 {
			if pos >= len(call.Args) {
				found["UNCLASSIFIED"]++
			} else if id, ok := call.Args[pos].(*ast.Ident); ok {
				found[id.Name]++
			} else {
				found["UNCLASSIFIED"]++
			}
		}
		return true
	})
	return found
}

func TestObserverNoticeSourceInventory(t *testing.T) {
	t.Parallel()
	expected := map[string]map[string]int{
		"pipeline.go":   {"NoticeReservationHeld": 1, "NoticeRenderRefresh": 4, "DiagnosticQuotaUnavailable": 1, "DiagnosticWorkerIdentityUnresolved": 2, "DiagnosticCandidateFreezeFailed": 1, "DiagnosticStopRequested": 2, "DiagnosticSpoolIngestionFailed": 1, "DiagnosticWorkerExitedEarly": 1, "DiagnosticDeadlineReached": 1},
		"recover.go":    {"DiagnosticRecoveryOwnershipUnresolved": 2, "NoticeWorkerReattached": 1},
		"acceptance.go": {"DiagnosticVerificationUnavailable": 1, "DiagnosticVerificationBindingUnavailable": 1},
	}
	total := 0
	for name, want := range expected {
		got := noticeInventory(t, name, nil)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s notice inventory=%v want=%v", name, got, want)
		}
		for _, n := range got {
			total += n
		}
	}
	if total != 19 {
		t.Fatalf("notice source inventory=%d want all19 producers", total)
	}
	for _, src := range []string{`package probe;func emit(){h.Notice("raw/account/path")}`, `package probe;func emit(){p.diagnostic(ctx,unknown,"raw")}`} {
		got := noticeInventory(t, "probe.go", src)
		if reflect.DeepEqual(got, expected["pipeline.go"]) {
			t.Fatal("unclassified source accepted as classified inventory")
		}
		if got["UNCLASSIFIED"] == 0 && got["unknown"] == 0 {
			t.Fatalf("unclassified source escaped inventory: %v", got)
		}
	}
}

func TestServiceDiagnosticDurableBeforeHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE migration_state SET phase='adopted' WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sink := NewObserverSink()
	reader, err := sink.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	leaked := false
	p := &pipeline{j: j, d: admission.Decision{RunID: "run_notice", StateDir: dir}, prod: NewProducer("sup_notice", 7), h: Hooks{Observer: sink, Notice: func(string) { leaked = true }}}
	codes := []DiagnosticCode{DiagnosticQuotaUnavailable, DiagnosticWorkerIdentityUnresolved, DiagnosticWorkerExitedEarly, DiagnosticSpoolIngestionFailed, DiagnosticCandidateFreezeFailed, DiagnosticVerificationUnavailable, DiagnosticVerificationBindingUnavailable, DiagnosticStopRequested, DiagnosticDeadlineReached, DiagnosticRecoveryOwnershipUnresolved}
	for i, code := range codes {
		if err := p.diagnostic(t.Context(), code, "raw/account/path/native sentinel"); err != nil {
			t.Fatal(err)
		}
		events, err := j.Events(t.Context(), "run_notice", 0)
		if err != nil || len(events) != i+1 {
			t.Fatalf("diagnostic missing before hint: %d %v", len(events), err)
		}
		ev := events[len(events)-1]
		if err := validateDiagnosticEvent(ev, "run_notice", "", 7); err != nil {
			t.Fatalf("invalid diagnostic envelope: %v", err)
		}
		if strings.Contains(string(ev.Payload), "sentinel") || strings.Contains(string(ev.Payload), "raw/") {
			t.Fatalf("raw notice persisted: %s", ev.Payload)
		}
		batch, err := reader.DrainHints(16)
		if err != nil || !reflect.DeepEqual(batch.Codes, []NoticeCode{NoticeRenderRefresh}) {
			t.Fatalf("durable refresh=%v err=%v", batch, err)
		}
	}
	if leaked {
		t.Fatal("service called raw terminal notice")
	}
	err = p.diagnostic(t.Context(), DiagnosticCode("raw/account/path"), "raw")
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
		t.Fatalf("unknown diagnostic=%v", err)
	}
	batch, err := reader.DrainHints(16)
	if err != nil || len(batch.Codes) != 0 {
		t.Fatalf("invalid diagnostic hinted: %v %v", batch, err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.diagnostic(t.Context(), DiagnosticStopRequested, "raw"); err == nil {
		t.Fatal("persistence failure looked recorded")
	}
	batch, err = reader.DrainHints(16)
	if err != nil || len(batch.Codes) != 0 {
		t.Fatalf("failed persistence hinted: %v %v", batch, err)
	}
}

func TestServiceDiagnosticClosedEnvelope(t *testing.T) {
	t.Parallel()
	d := diagnostic{Code: DiagnosticStopRequested, RunID: "run_notice", AttemptID: "att_notice", Generation: 7, EvidenceRefs: []string{"evt_evidence"}}
	payload, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	base := journal.Event{SchemaVersion: 2, Type: serviceDiagnosticEventType, RunID: d.RunID, AttemptID: d.AttemptID, Generation: 7, Payload: payload, ObservedAt: time.Now().UTC()}
	if err := validateDiagnosticEvent(base, d.RunID, d.AttemptID, 7); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*journal.Event){"schema1": func(e *journal.Event) { e.SchemaVersion = 1 }, "future schema": func(e *journal.Event) { e.SchemaVersion = 3 }, "wrong event": func(e *journal.Event) { e.Type = "service.unknown" }, "wrong run": func(e *journal.Event) { e.RunID = "run_other" }, "wrong attempt": func(e *journal.Event) { e.AttemptID = "att_other" }, "wrong generation": func(e *journal.Event) { e.Generation = 8 }, "raw code": func(e *journal.Event) {
		e.Payload = json.RawMessage(`{"code":"raw/account/path","run_id":"run_notice","attempt_id":"att_notice","generation":7,"evidence_refs":[]}`)
	}, "unknown field": func(e *journal.Event) {
		e.Payload = json.RawMessage(strings.TrimSuffix(string(payload), "}") + `,"raw":"account"}`)
	}, "duplicate": func(e *journal.Event) {
		e.Payload = json.RawMessage(strings.TrimSuffix(string(payload), "}") + `,"generation":7}`)
	}, "raw reference": func(e *journal.Event) {
		bad := d
		bad.EvidenceRefs = []string{"/tmp/account/native"}
		e.Payload, _ = json.Marshal(bad)
	}, "payload generation mismatch": func(e *journal.Event) { bad := d; bad.Generation = 6; e.Payload, _ = json.Marshal(bad) }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := base
			mutate(&e)
			if err := validateDiagnosticEvent(e, d.RunID, d.AttemptID, 7); err == nil {
				t.Fatal("invalid diagnostic accepted")
			}
		})
	}
}

func TestObserverNoticeInventoryRefusesMissingEntry(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(".", "pipeline.go"))
	if err != nil {
		t.Fatal(err)
	}
	original := noticeInventory(t, "pipeline.go", raw)
	modified := strings.Replace(string(raw), "p.inform(NoticeReservationHeld,", "p.inform(NoticeRenderRefresh,", 1)
	changed := noticeInventory(t, "mutant.go", modified)
	if reflect.DeepEqual(changed, original) {
		t.Fatal("deleting reservation inventory treatment was invisible")
	}
}

func TestOperationCauseSurvivesDiagnosticFailure(t *testing.T) {
	for _, operation := range []string{"freeze", "ingest"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			j, err := journal.Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			sink := NewObserverSink()
			reader, err := sink.OpenReader()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = reader.Close() }()
			p := &pipeline{j: j, d: admission.Decision{RunID: "run_notice", AttemptID: "att_notice", StateDir: dir}, prod: NewProducer("sup_notice", 7), h: Hooks{Observer: sink}}
			switch operation {
			case "freeze":
				// Invalid admitted candidate metadata is the operation failure;
				// the independently closed journal makes its diagnostic fail.
				err = p.freezeCandidate(t.Context())
			case "ingest":
				// Attempt lookup and subsequent diagnostic append both fail on
				// the closed journal; neither error may replace the other.
				err = p.watch(t.Context(), AttemptRef{StateDir: dir, RunID: "run_notice", AttemptID: "att_notice"}, make(chan struct{}))
			}
			joined, ok := err.(interface{ Unwrap() []error })
			if !ok || len(joined.Unwrap()) != 2 {
				t.Fatalf("%s lost operation or diagnostic cause: %v", operation, err)
			}
			causes := joined.Unwrap()
			if operation == "freeze" && !strings.Contains(causes[0].Error(), "invalid candidate metadata") {
				t.Fatalf("freeze operation cause=%v", causes[0])
			}
			var diagnosticErr *v2contract.ControlError
			if !errors.As(causes[1], &diagnosticErr) || diagnosticErr.Code != v2contract.CodePersistenceUnavailable || !strings.Contains(causes[1].Error(), "database is closed") {
				t.Fatalf("diagnostic failure=%v", causes[1])
			}
			batch, err := reader.DrainHints(16)
			if err != nil || len(batch.Codes) != 0 {
				t.Fatalf("failed diagnostics hinted: %v %v", batch, err)
			}
		})
	}
}
