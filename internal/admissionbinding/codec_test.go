package admissionbinding_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/workers"

	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
)

// Synthetic fixture, independently spelled in ADR field order; its hash was
// computed independently with Python hashlib, including the domain separator.
const golden = `{"SchemaVersion":1,"PreparationID":"prep_1","PeerID":"peer_1","RootID":"root_1","Generation":1,"ContextEpoch":"epoch_1","PolicyRevision":"policy_1","ExpiresAt":"2026-01-01T00:05:00Z","RunID":"run_1","TaskID":"task_1","AttemptID":"attempt_1","Request":{"SchemaVersion":2,"Repository":"/tmp/repo","TaskFile":"/tmp/task.md","TaskSHA256":"sha256:ABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABAB","Adapter":"claudecode","Billing":"subscription-declared","ExecutionProfile":"restricted","Revision":"main","Host":"standalone","Scenario":"","UseCommitted":false,"NoChecks":false,"KeepGoing":true,"TrustProjectConfig":"sha256:abababababababababababababababababababababababababababababababab","TrustNativeConfig":"sha256:abababababababababababababababababababababababababababababababab","DeclareEntitlement":"plan=pro,extra-usage=disabled","StripCredentialEnv":true,"AllowUntestedNativeVersion":true,"Ceilings":{"ExecutionNanos":60000000000,"Repairs":0,"Replans":null,"TransportRetries":2}},"Decision":{"StateRootID":"root_1","RunID":"run_1","TaskID":"task_1","AttemptID":"attempt_1","RunDir":"/tmp/state/runs/run_1","Workdir":"/tmp/state/runs/run_1/workspace","Host":"standalone","Scenario":"","GitName":"Synthetic Fixture","GitEmail":"fixture@example.com","ProjectConfig":{"SchemaVersion":1,"Passthrough":["ZED","ALPHA"],"AllowedTools":["Read","Bash(go test:*)"],"Checks":[{"Name":"vet","Argv":["go","vet","./..."],"FailOnOutput":true,"Timeout":"10s"},{"Name":"test","Argv":["go","test","./..."],"FailOnOutput":false,"Timeout":"1m"}],"Envelopes":{"Execution":"2m","Repairs":3,"Replans":1,"TransportRetries":5}},"ConfigDigest":"abababababababababababababababababababababababababababababababab","RepoIdentity":"/tmp/repo","RecordTrust":true,"NoChecks":false,"KeepGoing":true,"EnvelopeFlags":{"ExecutionNanos":60000000000,"Repairs":0,"Replans":null,"TransportRetries":2},"ResolvedCeilings":{"ExecutionNanos":60000000000,"Repairs":0,"Replans":1,"TransportRetries":2},"Declaration":{"AdapterID":"builtin/claudecode","PlanClass":"pro","ExtraUsage":"disabled","IdentityRef":"abababababababababababababababababababababababababababababababab","DeclaredAt":"2026-01-01T00:00:00Z"},"RecordNativeTrust":true,"NativeConfigDigest":"abababababababababababababababababababababababababababababababab","NativeHooks":0,"NativeTrustGrant":"native_config:sha256:abababababababababababababababababababababababababababababababab","NativeAuth":{"LoggedIn":true,"AuthMethod":"claude.ai","APIProvider":"firstParty","SubscriptionType":"pro","IdentityRef":"abababababababababababababababababababababababababababababababab","ConfigIdentity":"config_1"},"Task":{"SHA256":"abababababababababababababababababababababababababababababababab","Title":"Synthetic <&> task","ArtifactRef":"owned/task_1","Bytes":12},"Snapshot":{"SourceRepo":"/tmp/repo","Branch":"main","BaseRev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Dirty":false,"Source":"--rev"},"Profile":{"Name":"restricted","Contained":true,"Consent":"--execution-profile","Disclosure":"fixture","Boundary":{"Profile":"restricted","OS":"linux","Route":"builtin/claudecode","Boundary":"fixture","Version":"1","Owner":"fixture","Coverage":{"Filesystem":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"},"Process":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"},"Network":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"},"Credential":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"}}}},"Adapter":{"ID":"builtin/claudecode","Version":"1","Harness":"claude-code","Surface":"native-cli-structured"},"Probe":{"Executable":"/tmp/native","Version":"2.1.284","SHA256":"abababababababababababababababababababababababababababababababab","OS":"linux","Arch":"amd64","Compatibility":"fixture-tested"},"Proposal":{"Path":"/tmp/native","Args":["--print","second","first"],"Dir":"/tmp/state/runs/run_1/workspace","StopLadder":[{"Signal":"interrupt","GraceNanos":0},{"Signal":"kill","GraceNanos":1}],"Overrides":[{"Kind":"env","Name":"FIXTURE","Value":"yes","Reason":"synthetic"}],"Billing":{"Mode":"subscription-declared","CredentialProvenance":"native-login","EntitlementClass":"included-plan","EntitlementSource":"user_declared","PaidContinuation":"unknown","PaidContinuationUserDeclaration":"disabled","Qualified":false,"G05":"not-passed"},"Manifest":[{"Source":"project","Digest":"abababababababababababababababababababababababababababababababab"},{"Source":"user","Digest":"abababababababababababababababababababababababababababababababab"}],"Capabilities":{"SchemaVersion":1,"AdapterID":"builtin/claudecode","AdapterVersion":"1","RuntimeVersion":"2.1.284","Mode":"headless","ExecutionSurface":"native-cli-structured","HarnessID":"claude-code","FidelityQualification":"fixture-tested","Billing":{"Entitlement":"subscription","IncludedOnlySupported":"unknown","PaidOveragePrevention":"unsupported","EvidenceID":"evidence_1"},"HostIntegration":{"HerdrEmbedded":"unsupported","HerdrStateBridge":"unknown","NativeInteractiveAttach":"unsupported"},"Capabilities":{"StructuredEvents":"supported","Resume":"unsupported","LiveSteer":"unknown","ApprovalBridge":"unsupported","UsageTokens":"supported","QuotaRemaining":"unknown","HardMonetaryLimit":"unsupported","NativeSubagents":"unsupported","Reconnect":"unknown","ModelMetadata":null},"Sandbox":{"Status":"supported","Scope":"fixture"},"Platform":{"OS":"linux","WorkerDetachment":"supported","ProcessTreeOwnership":"supported"},"Qualification":"synthetic fixture only"}},"Environment":{"PolicyRevision":"policy_1","ContextEpoch":"epoch_1","PrincipalRef":"principal_1","NativeConfigIdentity":"config_1","RequiredNames":["ALPHA","ZED"],"Destinations":["first-party"]}},"Challenges":[{"ID":"challenge_1","Kind":"project-config","Text":"Trust exact project configuration"},{"ID":"challenge_2","Kind":"native-config","Text":"Trust exact native configuration"},{"ID":"challenge_3","Kind":"keep-going","Text":"Continue within finite envelope"}]}`
const goldenDigest = "07d88fb10fa723e5cba070cb2b6ec0a43f95bd7e2f0278f236641aaa270c2816"

func fixture(t *testing.T) ab.BindingV1 {
	t.Helper()
	var b ab.BindingV1
	if err := json.Unmarshal([]byte(golden), &b); err != nil {
		t.Fatal(err)
	}
	return b
}
func TestBindingV1CanonicalInputs(t *testing.T) { // I02/I20 (v2 sections 2/6/8).
	t.Run("strict parser", TestBindingV1StrictParser)
	t.Run("exact bounds", TestBindingV1Bounds)
	t.Run("optional evidence", TestBindingV1OptionalEvidence)
	t.Run("Unicode escapes", TestBindingV1UnicodeAndLiteralEscapes)
	t.Run("owning OS paths", TestBindingV1OwningOSPaths)
	b := fixture(t)
	got, err := ab.EncodeV1(b)
	if err != nil || string(got) != golden {
		t.Fatalf("canonical bytes: %s; error %v", got, err)
	}
	sum, err := ab.DigestV1(b)
	if err != nil || hex.EncodeToString(sum[:]) != goldenDigest {
		t.Fatalf("digest %x, error %v", sum, err)
	}
	decoded, err := ab.DecodeV1([]byte(golden))
	if err != nil || !reflect.DeepEqual(decoded, b) {
		t.Fatalf("decode differs: %v", err)
	}
}
func TestBindingV1SentinelErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ab.BindingV1)
		want   error
	}{
		{"valid", func(*ab.BindingV1) {}, nil},
		{"malformed-supported-version", func(b *ab.BindingV1) { b.RunID = "different" }, ab.ErrInvalidContract},
		{"future-schema", func(b *ab.BindingV1) { b.SchemaVersion = 2 }, ab.ErrCapabilityUnsupported},
		{"unsupported-bound", func(b *ab.BindingV1) {
			for i := 3; i < 129; i++ {
				b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: fmt.Sprintf("challenge_%d", i+1), Kind: "project-config", Text: "fixture"})
			}
		}, ab.ErrCapabilityUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := fixture(t)
			tc.mutate(&b)
			enc, e1 := ab.EncodeV1(b)
			sum, e2 := ab.DigestV1(b)
			var rawBuf bytes.Buffer
			encRaw := json.NewEncoder(&rawBuf)
			encRaw.SetEscapeHTML(false)
			if err := encRaw.Encode(b); err != nil {
				t.Fatal(err)
			}
			raw := bytes.TrimSuffix(rawBuf.Bytes(), []byte{"\n"[0]})
			dec, e3 := ab.DecodeV1(raw)
			for i, err := range []error{ab.ValidateV1(b), e1, e2, e3} {
				for n := range 3 {
					if tc.want == nil {
						if err != nil {
							t.Fatalf("API %d: %v", i, err)
						}
						break
					}
					if !errors.Is(err, tc.want) {
						t.Fatalf("API %d wrap %d: %v", i, n, err)
					}
					other := ab.ErrInvalidContract
					if errors.Is(tc.want, other) {
						other = ab.ErrCapabilityUnsupported
					}
					if errors.Is(err, other) {
						t.Fatalf("both sentinels: %v", err)
					}
					err = fmt.Errorf("caller: %w", err)
				}
			}
			if tc.want != nil && (enc != nil || sum != [32]byte{} || !reflect.DeepEqual(dec, ab.BindingV1{})) {
				t.Fatal("failed output is usable")
			}
		})
	}
}
func TestAdmissionBindingStandardLibraryOnly(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "go", "list", "-deps", "-f", `{{if not .Standard}}{{.ImportPath}}{{end}}`, "github.com/turbokast/mythhelm/internal/admissionbinding")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.FieldsSeq(string(out)) {
		if line != "github.com/turbokast/mythhelm/internal/admissionbinding" {
			t.Fatalf("non-standard dependency %s", line)
		}
	}
}

func TestBindingV1StrictParser(t *testing.T) {
	cases := map[string]string{
		"duplicate":        strings.Replace(golden, `"SchemaVersion":1`, `"SchemaVersion":1,"SchemaVersion":1`, 1),
		"nested duplicate": strings.Replace(golden, `"Name":"vet"`, `"Name":"vet","Name":"vet"`, 1),
		"unknown":          strings.Replace(golden, `"Generation":1`, `"Generation":1,"Unknown":false`, 1),
		"missing":          strings.Replace(golden, `"Generation":1,`, "", 1),
		"case":             strings.Replace(golden, `"Generation"`, `"generation"`, 1),
		"null":             strings.Replace(golden, `"Generation":1`, `"Generation":null`, 1),
		"array null":       strings.Replace(golden, `"Args":["--print","second","first"]`, `"Args":null`, 1),
		"float":            strings.Replace(golden, `"Generation":1`, `"Generation":1.0`, 1),
		"overflow":         strings.Replace(golden, `"Generation":1`, `"Generation":9223372036854775808`, 1),
		"whitespace":       " " + golden,
		"field order":      strings.Replace(golden, `"SchemaVersion":1,"PreparationID":"prep_1"`, `"PreparationID":"prep_1","SchemaVersion":1`, 1),
		"escaped":          strings.Replace(golden, "Synthetic <&> task", `Synthetic \u003c&> task`, 1),
		"trailing":         golden + "{}",
		"utf8":             strings.Replace(golden, "Synthetic <&> task", string([]byte{255}), 1),
		"surrogate":        strings.Replace(golden, "Synthetic <&> task", `\ud800`, 1),
		"depth":            strings.Repeat("[", 65) + strings.Repeat("]", 65),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ab.DecodeV1([]byte(raw))
			if !errors.Is(err, ab.ErrInvalidContract) || !reflect.DeepEqual(got, ab.BindingV1{}) {
				t.Fatalf("got usable value/error %v", err)
			}
		})
	}
	if _, err := ab.DecodeV1(bytes.Repeat([]byte(" "), (1<<20)+1)); !errors.Is(err, ab.ErrCapabilityUnsupported) {
		t.Fatalf("byte bound: %v", err)
	}
}

func TestBindingV1HashSpellings(t *testing.T) {
	t.Run("pinned source flag grammar", TestBindingV1NativeTrustFlagSource) // I09 (v2 section 2): absence is not a zero digest.
	for _, task := range []string{strings.Repeat("ab", 32), strings.Repeat("AB", 32), "sha256:" + strings.Repeat("ab", 32), "sha256:" + strings.Repeat("AB", 32)} {
		b := fixture(t)
		b.Request.TaskSHA256 = task
		raw, err := ab.EncodeV1(b)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ab.DecodeV1(raw)
		if err != nil || got.Request.TaskSHA256 != task {
			t.Fatalf("spelling lost: %v", err)
		}
	}
	b := fixture(t)
	b.Request.NoChecks = true
	b.Decision.NoChecks = true
	b.Request.TrustProjectConfig = ""
	b.Decision.RecordTrust = false
	b.Decision.ConfigDigest = ""
	b.Decision.ProjectConfig = ab.ProjectConfigV1{}
	b.Decision.ResolvedCeilings.Replans = 2
	b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: "no_checks", Kind: "no-checks", Text: "Run without checks"})
	b.Decision.RecordNativeTrust = false
	b.Request.TrustNativeConfig = ""
	b.Decision.NativeConfigDigest = ""
	b.Decision.NativeHooks = nil
	b.Decision.NativeTrustGrant = ""
	b.Decision.NativeAuth = nil
	b.Decision.Environment.NativeConfigIdentity = ""
	b.Request.Adapter = "fake"
	b.Request.Billing = "local-scripted"
	b.Request.Scenario = "happy"
	b.Request.DeclareEntitlement = ""
	b.Request.StripCredentialEnv = false
	b.Request.AllowUntestedNativeVersion = false
	b.Decision.Scenario = "happy"
	b.Decision.Declaration = nil
	b.Decision.Adapter.ID = "builtin/fake"
	b.Decision.Adapter.Harness = "fake"
	b.Decision.Profile.Boundary.Route = "builtin/fake"
	b.Decision.Proposal.Billing.Mode = "local-scripted"
	b.Decision.Proposal.Capabilities.AdapterID = "builtin/fake"
	b.Decision.Proposal.Capabilities.HarnessID = "fake"
	raw, err := ab.EncodeV1(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ab.DecodeV1(raw)
	if err != nil || got.Decision.ConfigDigest != "" || got.Decision.NativeHooks != nil || got.Decision.NativeAuth != nil || !bytes.Contains(raw, []byte(`"Checks":[]`)) {
		t.Fatalf("absence changed: %v", err)
	}
	for name, mutate := range map[string]func(*ab.BindingV1){
		"upper trust":         func(b *ab.BindingV1) { b.Request.TrustProjectConfig = "sha256:" + strings.Repeat("AB", 32) },
		"unprefixed trust":    func(b *ab.BindingV1) { b.Request.TrustNativeConfig = strings.Repeat("ab", 32) },
		"derived uppercase":   func(b *ab.BindingV1) { b.Decision.Task.SHA256 = strings.Repeat("AB", 32) },
		"different bytes":     func(b *ab.BindingV1) { b.Request.TaskSHA256 = strings.Repeat("ac", 32) },
		"no-checks populated": func(b *ab.BindingV1) { b.Request.NoChecks = true; b.Decision.NoChecks = true },
	} {
		t.Run(name, func(t *testing.T) {
			b := fixture(t)
			mutate(&b)
			if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
				t.Fatalf("accepted invalid hash/config: %v", err)
			}
		})
	}
}

func TestBindingV1Consistency(t *testing.T) {
	for name, mutate := range map[string]func(*ab.BindingV1){
		"root":             func(b *ab.BindingV1) { b.RootID = "other" },
		"task":             func(b *ab.BindingV1) { b.TaskID = "other" },
		"attempt":          func(b *ab.BindingV1) { b.AttemptID = "other" },
		"epoch":            func(b *ab.BindingV1) { b.ContextEpoch = "other" },
		"policy":           func(b *ab.BindingV1) { b.PolicyRevision = "other" },
		"argv nul":         func(b *ab.BindingV1) { b.Decision.Proposal.Args[0] = "x\x00y" },
		"bad tri":          func(b *ab.BindingV1) { b.Decision.Proposal.Capabilities.Capabilities.Reconnect = new(ab.TriV1) },
		"negative":         func(b *ab.BindingV1) { b.Decision.ResolvedCeilings.Repairs = -1 },
		"resolved ceiling": func(b *ab.BindingV1) { b.Decision.ResolvedCeilings.Repairs = 1 },
		"flags":            func(b *ab.BindingV1) { b.Decision.EnvelopeFlags.Repairs = nil },
		"run directory":    func(b *ab.BindingV1) { b.Decision.RunDir = "/tmp/state/runs/another" },
		"workdir":          func(b *ab.BindingV1) { b.Decision.Proposal.Dir = "/tmp/another" },
		"repository":       func(b *ab.BindingV1) { b.Decision.Snapshot.SourceRepo = "/tmp/another" },
		"exe":              func(b *ab.BindingV1) { b.Decision.Proposal.Path = "/tmp/another" },
		"version":          func(b *ab.BindingV1) { b.Decision.Proposal.Capabilities.RuntimeVersion = "other" },
		"adapter":          func(b *ab.BindingV1) { b.Decision.Proposal.Capabilities.AdapterID = "other" },
		"billing":          func(b *ab.BindingV1) { b.Decision.Proposal.Billing.Mode = "local-scripted" },
		"manifest sort": func(b *ab.BindingV1) {
			b.Decision.Proposal.Manifest[0], b.Decision.Proposal.Manifest[1] = b.Decision.Proposal.Manifest[1], b.Decision.Proposal.Manifest[0]
		},
		"manifest duplicate":  func(b *ab.BindingV1) { b.Decision.Proposal.Manifest[1] = b.Decision.Proposal.Manifest[0] },
		"names sort":          func(b *ab.BindingV1) { b.Decision.Environment.RequiredNames = []string{"ZED", "ALPHA"} },
		"names duplicate":     func(b *ab.BindingV1) { b.Decision.Environment.RequiredNames = []string{"ALPHA", "ALPHA"} },
		"challenge duplicate": func(b *ab.BindingV1) { b.Challenges = append(b.Challenges, b.Challenges[0]) },
		"challenge missing":   func(b *ab.BindingV1) { b.Challenges = nil },
		"artefact escape":     func(b *ab.BindingV1) { b.Decision.Task.ArtifactRef = "../task" },
		"short commit":        func(b *ab.BindingV1) { b.Decision.Snapshot.BaseRev = "abc123" },
		"timezone":            func(b *ab.BindingV1) { b.ExpiresAt = "2026-01-01T00:05:00+00:00" },
		"zero duration":       func(b *ab.BindingV1) { *b.Request.Ceilings.ExecutionNanos = 0 },
		"fraction millis":     func(b *ab.BindingV1) { *b.Request.Ceilings.ExecutionNanos = 1 },
		"task bytes":          func(b *ab.BindingV1) { b.Decision.Task.Bytes = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			b := fixture(t)
			mutate(&b)
			if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
				t.Fatalf("accepted inconsistent binding: %v", err)
			}
		})
	}
}

// Each rule assigns the actual source field a disposition and exact DTO
// destination (or '-' for values that must be rederived/rejected). PR means
// persist and freshly rederive; X means never serialised. This is an inventory
// of current main structs, not an import of future owner/permit symbols.
//
//nolint:gosec // G101: rules contain source field names and dispositions, no credential values.
func TestBindingV1SourceFieldInventory(t *testing.T) {
	t.Run("bound identities", TestBindingV1Consistency)
	t.Run("actual selector", TestBindingV1CurrentMainSelector)
	type inventory struct {
		source reflect.Type
		rules  map[string]string
	}
	rows := []inventory{
		{reflect.TypeFor[admission.Decision](), map[string]string{
			"StateDir": "R Decision.StateRootID held physical root", "RunID": "P Decision.RunID allocated once", "TaskID": "P Decision.TaskID allocated once", "AttemptID": "P Decision.AttemptID allocated once",
			"RunDir": "PR Decision.RunDir root/runs/run", "Workdir": "PR Decision.Workdir run/workspace", "Host": "P Decision.Host standalone only", "Scenario": "P Decision.Scenario fake only", "GitName": "P Decision.GitName service identity retention", "GitEmail": "P Decision.GitEmail service identity retention",
			"ProjectConfig": "PR Decision.ProjectConfig strict immutable blob", "ConfigDigest": "PR Decision.ConfigDigest exact committed bytes", "RepoIdentity": "PR Decision.RepoIdentity canonical physical identity", "RecordTrust": "P Decision.RecordTrust exact project intent", "NoChecks": "P Decision.NoChecks explicit confirmation", "KeepGoing": "P Decision.KeepGoing unchanged confirmation", "EnvelopeFlags": "PR Decision.EnvelopeFlags null unset inputs", "Declaration": "PR Decision.Declaration nonsecret assertion", "RecordNativeTrust": "P Decision.RecordNativeTrust exact native intent", "NativeConfigDigest": "PR Decision.NativeConfigDigest fresh inventory", "NativeHooks": "PR Decision.NativeHooks measured count or null", "NativeTrustGrant": "PR Decision.NativeTrustGrant exact grant or absent", "NativeAuth": "PR Decision.NativeAuth qualified status", "Task": "PR Decision.Task owned artefact", "Snapshot": "PR Decision.Snapshot physical repo and commit", "Profile": "PR Decision.Profile current boundary", "Adapter": "PR Decision.Adapter registered adapter", "Probe": "PR Decision.Probe executable and qualification", "Proposal": "PR Decision.Proposal pinned recipe",
		}},
		{reflect.TypeFor[control.RunSpec](), map[string]string{
			"SchemaVersion": "P Request.SchemaVersion exactly 2", "Repository": "PR Request.Repository physical source", "TaskFile": "P Request.TaskFile original provenance only", "TaskSHA256": "PR Request.TaskSHA256 preserve raw spelling", "Adapter": "PR Request.Adapter exact selector", "Billing": "PR Request.Billing exact route", "ExecutionProfile": "PR Request.ExecutionProfile no downgrade", "Revision": "PR Request.Revision resolved full commit", "Host": "P Request.Host standalone", "Scenario": "P Request.Scenario fake only", "UseCommitted": "P Request.UseCommitted exact source selection", "NoChecks": "P Request.NoChecks explicit flag", "KeepGoing": "P Request.KeepGoing explicit flag", "TrustProjectConfig": "PR Request.TrustProjectConfig exact lowercase prefixed consent", "TrustNativeConfig": "PR Request.TrustNativeConfig exact lowercase prefixed consent", "DeclareEntitlement": "PR Request.DeclareEntitlement exact assertion", "StripCredentialEnv": "PR Request.StripCredentialEnv service filtering", "AllowUntestedNativeVersion": "PR Request.AllowUntestedNativeVersion unchanged executable", "Ceilings": "PR Request.Ceilings exact converted inputs",
		}},
		{reflect.TypeFor[control.RunCeilings](), map[string]string{"ExecutionMillis": "P Request.Ceilings.ExecutionNanos exact multiply without overflow", "Repairs": "P Request.Ceilings.Repairs nullable", "Replans": "P Request.Ceilings.Replans nullable", "TransportRetries": "P Request.Ceilings.TransportRetries nullable"}},
		{reflect.TypeFor[admission.Task](), map[string]string{"Content": "R Decision.Task.ArtifactRef bounded owned exact bytes never JSON", "SHA256": "PR Decision.Task.SHA256 normalised content hash", "Title": "P Decision.Task.Title policy permitted title"}},
		{reflect.TypeFor[admission.Snapshot](), map[string]string{"SourceRepo": "PR Decision.Snapshot.SourceRepo physical repo", "Branch": "P Decision.Snapshot.Branch captured", "BaseRev": "PR Decision.Snapshot.BaseRev full commit", "Dirty": "P Decision.Snapshot.Dirty observation only", "Source": "P Decision.Snapshot.Source source selection"}},
		{reflect.TypeFor[admission.Profile](), map[string]string{"Name": "PR Decision.Profile.Name no downgrade", "Contained": "PR Decision.Profile.Contained actual boundary", "Consent": "P Decision.Profile.Consent exact authority", "Disclosure": "P Decision.Profile.Disclosure exact disclosure", "Boundary": "PR Decision.Profile.Boundary qualified current evidence"}},
		{reflect.TypeFor[admission.ProjectConfig](), map[string]string{"SchemaVersion": "PR Decision.ProjectConfig.SchemaVersion strict schema", "Environment": "PR Decision.ProjectConfig.Passthrough names only", "Adapters": "PR Decision.ProjectConfig.AllowedTools ordered tools", "Checks": "PR Decision.ProjectConfig.Checks ordered checks", "Envelopes": "PR Decision.ProjectConfig.Envelopes exact inputs"}},
		{reflect.TypeFor[admission.CheckConfig](), map[string]string{"Name": "PR Decision.ProjectConfig.Checks.Name exact", "Argv": "PR Decision.ProjectConfig.Checks.Argv ordered nonsecret argv", "FailOnOutput": "PR Decision.ProjectConfig.Checks.FailOnOutput exact", "Timeout": "PR Decision.ProjectConfig.Checks.Timeout positive duration"}},
		{reflect.TypeFor[admission.EnvelopesConfig](), map[string]string{"Execution": "PR Decision.ProjectConfig.Envelopes.Execution duration", "Repairs": "PR Decision.ProjectConfig.Envelopes.Repairs nullable", "Replans": "PR Decision.ProjectConfig.Envelopes.Replans nullable", "TransportRetries": "PR Decision.ProjectConfig.Envelopes.TransportRetries nullable"}},
		{reflect.TypeFor[billing.Ceilings](), map[string]string{"Execution": "PR Decision.EnvelopeFlags.ExecutionNanos negative unset becomes null", "Repairs": "PR Decision.EnvelopeFlags.Repairs negative unset becomes null", "Replans": "PR Decision.EnvelopeFlags.Replans negative unset becomes null", "TransportRetries": "PR Decision.EnvelopeFlags.TransportRetries negative unset becomes null"}},
		{reflect.TypeFor[admission.Declaration](), map[string]string{"AdapterID": "PR Decision.Declaration.AdapterID exact", "PlanClass": "PR Decision.Declaration.PlanClass assertion", "ExtraUsage": "PR Decision.Declaration.ExtraUsage assertion", "IdentityRef": "PR Decision.Declaration.IdentityRef hashed identity", "DeclaredAt": "PR Decision.Declaration.DeclaredAt UTC timestamp"}},
		{reflect.TypeFor[admission.AuthEvidence](), map[string]string{"LoggedIn": "PR Decision.NativeAuth.LoggedIn status only", "AuthMethod": "PR Decision.NativeAuth.AuthMethod qualified status", "APIProvider": "PR Decision.NativeAuth.APIProvider qualified status", "SubscriptionType": "PR Decision.NativeAuth.SubscriptionType status only", "IdentityRef": "PR Decision.NativeAuth.IdentityRef nonsecret identity", "ConfigDirectory": "R Decision.NativeAuth.ConfigIdentity approved config resolution"}},
		{reflect.TypeFor[adapter.Descriptor](), map[string]string{"ID": "PR Decision.Adapter.ID registered", "Version": "PR Decision.Adapter.Version exact", "Harness": "PR Decision.Adapter.Harness exact", "Surface": "PR Decision.Adapter.Surface qualified"}},
		{reflect.TypeFor[adapter.Probe](), map[string]string{"Executable": "PR Decision.Probe.Executable physical executable", "Version": "PR Decision.Probe.Version exact", "SHA256": "PR Decision.Probe.SHA256 normalised", "OS": "PR Decision.Probe.OS qualified", "Arch": "PR Decision.Probe.Arch qualified", "Compatibility": "PR Decision.Probe.Compatibility qualified"}},
		{reflect.TypeFor[adapter.LaunchProposal](), map[string]string{"Spec": "PR Decision.Proposal.Path pinned launch recipe", "StopLadder": "PR Decision.Proposal.StopLadder exact ordered steps", "Overrides": "PR Decision.Proposal.Overrides policy permitted", "Billing": "PR Decision.Proposal.Billing exact evidence", "Manifest": "PR Decision.Proposal.Manifest sorted source digests", "Capabilities": "PR Decision.Proposal.Capabilities complete evidence"}},
		{reflect.TypeFor[adapter.ProcSpec](), map[string]string{"Path": "PR Decision.Proposal.Path resolved executable", "Args": "PR Decision.Proposal.Args ordered nonsecret argv", "Dir": "PR Decision.Proposal.Dir managed workspace", "Env": "X - full environment even aggregate hash forbidden", "Stdin": "R Decision.Task.ArtifactRef fresh reader owned bytes"}},
		{reflect.TypeFor[adapter.StopStep](), map[string]string{"Signal": "PR Decision.Proposal.StopLadder.Signal exact", "Grace": "PR Decision.Proposal.StopLadder.GraceNanos exact nonnegative nanos"}},
		{reflect.TypeFor[adapter.ConfigDelta](), map[string]string{"Kind": "PR Decision.Proposal.Overrides.Kind policy permitted", "Name": "PR Decision.Proposal.Overrides.Name policy permitted", "Value": "PR Decision.Proposal.Overrides.Value nonsecret only otherwise X", "Reason": "PR Decision.Proposal.Overrides.Reason policy permitted"}},
		{reflect.TypeFor[adapter.ConfigManifest](), map[string]string{"Digests": "PR Decision.Proposal.Manifest sorted unique source digest pairs"}},
		{reflect.TypeFor[adapter.BillingPosture](), map[string]string{"Mode": "PR Decision.Proposal.Billing.Mode route", "CredentialProvenance": "PR Decision.Proposal.Billing.CredentialProvenance evidence", "EntitlementClass": "PR Decision.Proposal.Billing.EntitlementClass evidence", "EntitlementSource": "PR Decision.Proposal.Billing.EntitlementSource evidence", "PaidContinuation": "PR Decision.Proposal.Billing.PaidContinuation observation", "PaidContinuationUserDeclaration": "PR Decision.Proposal.Billing.PaidContinuationUserDeclaration assertion", "Qualified": "PR Decision.Proposal.Billing.Qualified current qualification", "G05": "PR Decision.Proposal.Billing.G05 current qualification"}},
		{reflect.TypeFor[adapter.CapabilityRecord](), map[string]string{"SchemaVersion": "PR Decision.Proposal.Capabilities.SchemaVersion versioned", "AdapterID": "PR Decision.Proposal.Capabilities.AdapterID exact", "AdapterVersion": "PR Decision.Proposal.Capabilities.AdapterVersion exact", "RuntimeVersion": "PR Decision.Proposal.Capabilities.RuntimeVersion exact", "Mode": "PR Decision.Proposal.Capabilities.Mode exact", "ExecutionSurface": "PR Decision.Proposal.Capabilities.ExecutionSurface exact", "HarnessID": "PR Decision.Proposal.Capabilities.HarnessID exact", "FidelityQualification": "PR Decision.Proposal.Capabilities.FidelityQualification evidence", "Billing": "PR Decision.Proposal.Capabilities.Billing evidence", "HostIntegration": "PR Decision.Proposal.Capabilities.HostIntegration evidence", "Capabilities": "PR Decision.Proposal.Capabilities.Capabilities evidence", "Sandbox": "PR Decision.Proposal.Capabilities.Sandbox evidence", "Platform": "PR Decision.Proposal.Capabilities.Platform evidence", "Qualification": "PR Decision.Proposal.Capabilities.Qualification evidence"}},
		{reflect.TypeFor[adapter.BillingCapabilities](), map[string]string{"Entitlement": "PR Decision.Proposal.Capabilities.Billing.Entitlement evidence", "IncludedOnlySupported": "PR Decision.Proposal.Capabilities.Billing.IncludedOnlySupported tri", "PaidOveragePrevention": "PR Decision.Proposal.Capabilities.Billing.PaidOveragePrevention tri", "EvidenceID": "PR Decision.Proposal.Capabilities.Billing.EvidenceID explicit nullable"}},
		{reflect.TypeFor[adapter.HostIntegration](), map[string]string{"HerdrEmbedded": "PR Decision.Proposal.Capabilities.HostIntegration.HerdrEmbedded tri", "HerdrStateBridge": "PR Decision.Proposal.Capabilities.HostIntegration.HerdrStateBridge tri", "NativeInteractiveAttach": "PR Decision.Proposal.Capabilities.HostIntegration.NativeInteractiveAttach tri"}},
		{reflect.TypeFor[adapter.Capabilities](), map[string]string{"StructuredEvents": "PR Decision.Proposal.Capabilities.Capabilities.StructuredEvents mandatory supported", "Resume": "PR Decision.Proposal.Capabilities.Capabilities.Resume tri", "LiveSteer": "PR Decision.Proposal.Capabilities.Capabilities.LiveSteer tri", "ApprovalBridge": "PR Decision.Proposal.Capabilities.Capabilities.ApprovalBridge tri", "UsageTokens": "PR Decision.Proposal.Capabilities.Capabilities.UsageTokens tri", "QuotaRemaining": "PR Decision.Proposal.Capabilities.Capabilities.QuotaRemaining tri", "HardMonetaryLimit": "PR Decision.Proposal.Capabilities.Capabilities.HardMonetaryLimit tri", "NativeSubagents": "PR Decision.Proposal.Capabilities.Capabilities.NativeSubagents tri", "Reconnect": "PR Decision.Proposal.Capabilities.Capabilities.Reconnect empty becomes null", "ModelMetadata": "PR Decision.Proposal.Capabilities.Capabilities.ModelMetadata empty becomes null"}},
		{reflect.TypeFor[adapter.Sandbox](), map[string]string{"Status": "PR Decision.Proposal.Capabilities.Sandbox.Status tri", "Scope": "PR Decision.Proposal.Capabilities.Sandbox.Scope exact"}},
		{reflect.TypeFor[adapter.Platform](), map[string]string{"OS": "PR Decision.Proposal.Capabilities.Platform.OS qualified", "WorkerDetachment": "PR Decision.Proposal.Capabilities.Platform.WorkerDetachment tri", "ProcessTreeOwnership": "PR Decision.Proposal.Capabilities.Platform.ProcessTreeOwnership tri"}},
		{reflect.TypeFor[contain.Evidence](), map[string]string{"Profile": "PR Decision.Profile.Boundary.Profile current", "OS": "PR Decision.Profile.Boundary.OS current", "Route": "PR Decision.Profile.Boundary.Route current", "Boundary": "PR Decision.Profile.Boundary.Boundary current", "Version": "PR Decision.Profile.Boundary.Version current", "Owner": "PR Decision.Profile.Boundary.Owner current", "Coverage": "PR Decision.Profile.Boundary.Coverage current"}},
		{reflect.TypeFor[contain.Coverage](), map[string]string{"Filesystem": "PR Decision.Profile.Boundary.Coverage.Filesystem current", "Process": "PR Decision.Profile.Boundary.Coverage.Process current", "Network": "PR Decision.Profile.Boundary.Coverage.Network current", "Credential": "PR Decision.Profile.Boundary.Coverage.Credential current"}},
		{reflect.TypeFor[contain.Claim](), map[string]string{"Name": "PR Decision.Profile.Boundary.Coverage.Filesystem.Name qualified", "Version": "PR Decision.Profile.Boundary.Coverage.Filesystem.Version qualified", "Enforced": "PR Decision.Profile.Boundary.Coverage.Filesystem.Enforced current", "Detail": "PR Decision.Profile.Boundary.Coverage.Filesystem.Detail evidence"}},
		{reflect.TypeFor[contain.Policy](), map[string]string{"Profile": "R Decision.Profile.Name derived qualified policy", "Workdir": "R Decision.Workdir managed workspace", "ReadOnly": "R Decision.Profile.Name derived inspect policy", "AuthBinds": "R - native approved binds never caller authority", "ProxyAddr": "R - runtime address pinned before spawn"}},
		{reflect.TypeFor[contain.AuthBind](), map[string]string{"Source": "R - approved native store location never binding", "Target": "R - derived qualified sandbox binding"}},

		{reflect.TypeFor[workers.Launch](), map[string]string{"ExecutionDeadline": "R - launch policy committed before spawn", "LaunchToken": "R - fresh protected coordination token journal hash only", "TaskID": "PR TaskID exact", "AdapterID": "PR Decision.Adapter.ID exact", "Path": "PR Decision.Proposal.Path exact", "NativeSHA256": "PR Decision.Probe.SHA256 exact", "Args": "PR Decision.Proposal.Args ordered", "Dir": "PR Decision.Proposal.Dir exact", "Env": "X - full environment forbidden in binding", "PromptPath": "R Decision.Task.ArtifactRef owned artefact exact hash", "StopLadder": "PR Decision.Proposal.StopLadder exact", "LadderVersion": "R - pinned launch policy", "Nonce": "R - fresh protected coordination nonce journal hash only", "Generation": "PR Generation held service generation", "Containment": "R Decision.Profile.Boundary derived policy", "ProxyAllow": "R Decision.Environment.Destinations qualified endpoint set deny all by default", "UserConfigPaths": "R Decision.Proposal.Manifest exact one-to-one qualified inventory", "UserConfigDigests": "PR Decision.Proposal.Manifest exact one-to-one source digests"}},
	}
	// Anonymous source tables are also inventoried; additions inside them fail.
	cfg := reflect.TypeFor[admission.ProjectConfig]()
	env, _ := cfg.FieldByName("Environment")
	adapters, _ := cfg.FieldByName("Adapters")
	claude, _ := adapters.Type.FieldByName("ClaudeCode")
	rows = append(rows, inventory{env.Type, map[string]string{"Passthrough": "PR Decision.ProjectConfig.Passthrough ordered names"}}, inventory{adapters.Type, map[string]string{"ClaudeCode": "PR Decision.ProjectConfig.AllowedTools exact"}}, inventory{claude.Type, map[string]string{"AllowedTools": "PR Decision.ProjectConfig.AllowedTools ordered"}})
	registered := map[reflect.Type]bool{}
	for _, row := range rows {
		registered[row.source] = true
		if err := checkInventory(row.source, row.rules); err != nil {
			t.Fatal(err)
		}
	}
	// Walk every nested source struct so an added complex field cannot evade
	// completeness merely by receiving an outer rule.
	var visit func(reflect.Type)
	visited := map[reflect.Type]bool{}
	visit = func(typ reflect.Type) {
		if visited[typ] {
			return
		}
		visited[typ] = true
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
			visit(typ.Elem())
		case reflect.Struct:
			if typ == reflect.TypeFor[time.Time]() {
				return
			}
			if !registered[typ] {
				t.Errorf("unmapped nested source %s", typ)
			}
			for field := range typ.Fields() {
				visit(field.Type)
			}
		}
	}
	visit(reflect.TypeFor[admission.Decision]())
	visit(reflect.TypeFor[control.RunSpec]())
	visit(reflect.TypeFor[workers.Launch]())
	// Counterfactuals keep the completeness assertion falsifiable.
	dropped := map[string]string{}
	for k, v := range rows[0].rules {
		if k != "RunID" {
			dropped[k] = v
		}
	}
	if checkInventory(rows[0].source, dropped) == nil {
		t.Fatal("dropped mapping accepted")
	}
	added := reflect.StructOf([]reflect.StructField{{Name: "FutureField", Type: reflect.TypeFor[string]()}})
	if checkInventory(added, map[string]string{}) == nil {
		t.Fatal("new source field accepted")
	}
}

func checkInventory(source reflect.Type, rules map[string]string) error {
	if len(rules) != source.NumField() {
		return fmt.Errorf("%s inventory count %d, actual %d", source, len(rules), source.NumField())
	}
	for field := range source.Fields() {
		parts := strings.Fields(rules[field.Name])
		if len(parts) < 3 || !slicesContain([]string{"P", "R", "PR", "X"}, parts[0]) {
			return fmt.Errorf("%s.%s lacks disposition", source, field.Name)
		}
		if parts[1] != "-" {
			typ := reflect.TypeFor[ab.BindingV1]()
			for name := range strings.SplitSeq(parts[1], ".") {
				for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
					typ = typ.Elem()
				}
				if typ.Kind() != reflect.Struct {
					return fmt.Errorf("invalid target %s", parts[1])
				}
				f, ok := typ.FieldByName(name)
				if !ok {
					return fmt.Errorf("missing target %s", parts[1])
				}
				typ = f.Type
			}
		}
	}
	return nil
}
func slicesContain(values []string, s string) bool {
	return slices.Contains(values, s)
}

func TestBindingV1UnicodeAndLiteralEscapes(t *testing.T) {
	for _, s := range []string{`literal \u2028 and \u2029`, "actual \u2028 and \u2029", `literal \\u2028 and \\\u2029`, "mixed \\u2028 \u2028"} {
		b := fixture(t)
		b.Decision.Task.Title = s
		raw, err := ab.EncodeV1(b)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(raw) {
			t.Fatalf("invalid JSON for %q", s)
		}
		got, err := ab.DecodeV1(raw)
		if err != nil || got.Decision.Task.Title != s {
			t.Fatalf("escape changed for %q: %q, %v", s, got.Decision.Task.Title, err)
		}
	}
}

func TestBindingV1Bounds(t *testing.T) {
	cases := []struct {
		name  string
		at    func(*ab.BindingV1, int)
		limit int
	}{
		{"challenges", func(b *ab.BindingV1, n int) {
			for i := len(b.Challenges); i < n; i++ {
				b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: fmt.Sprintf("ch_%03d", i), Kind: "synthetic", Text: "synthetic"})
			}
		}, 128},
		{"checks", func(b *ab.BindingV1, n int) {
			for i := len(b.Decision.ProjectConfig.Checks); i < n; i++ {
				b.Decision.ProjectConfig.Checks = append(b.Decision.ProjectConfig.Checks, ab.CheckV1{Name: fmt.Sprintf("check_%03d", i), Argv: []string{"go", "test"}, Timeout: "1s"})
			}
		}, 64},
		{"launch argv", func(b *ab.BindingV1, n int) { b.Decision.Proposal.Args = make([]string, n) }, 256},
		{"check argv", func(b *ab.BindingV1, n int) {
			b.Decision.ProjectConfig.Checks[0].Argv = make([]string, n)
			b.Decision.ProjectConfig.Checks[0].Argv[0] = "go"
		}, 256},
		{"tools", func(b *ab.BindingV1, n int) {
			b.Decision.ProjectConfig.AllowedTools = make([]string, n)
			for i := range n {
				b.Decision.ProjectConfig.AllowedTools[i] = "Read"
			}
		}, 256},
		{"passthrough", func(b *ab.BindingV1, n int) {
			b.Decision.ProjectConfig.Passthrough = make([]string, n)
			for i := range n {
				b.Decision.ProjectConfig.Passthrough[i] = "SYNTHETIC"
			}
		}, 256},
		{"manifest", func(b *ab.BindingV1, n int) {
			b.Decision.Proposal.Manifest = nil
			for i := range n {
				b.Decision.Proposal.Manifest = append(b.Decision.Proposal.Manifest, ab.DigestSourceV1{Source: fmt.Sprintf("source_%03d", i), Digest: strings.Repeat("ab", 32)})
			}
		}, 256},
		{"stop steps", func(b *ab.BindingV1, n int) {
			b.Decision.Proposal.StopLadder = make([]ab.StopV1, n)
			for i := range n {
				b.Decision.Proposal.StopLadder[i].Signal = "kill"
			}
		}, 16},
		{"title bytes", func(b *ab.BindingV1, n int) { b.Decision.Task.Title = strings.Repeat("x", n) }, 256},
		{"ordinary string bytes", func(b *ab.BindingV1, n int) { b.Decision.Proposal.Overrides[0].Value = strings.Repeat("x", n) }, 4096},
		{"tool bytes", func(b *ab.BindingV1, n int) { b.Decision.ProjectConfig.AllowedTools[0] = strings.Repeat("x", n) }, 1024},
		{"task bytes", func(b *ab.BindingV1, n int) { b.Decision.Task.Bytes = int64(n) }, 1 << 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := fixture(t)
			tc.at(&b, tc.limit)
			if err := ab.ValidateV1(b); err != nil {
				t.Fatalf("exact bound: %v", err)
			}
			tc.at(&b, tc.limit+1)
			if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrCapabilityUnsupported) || errors.Is(err, ab.ErrInvalidContract) {
				t.Fatalf("over bound: %v", err)
			}
		})
	}
	// Fill independently counted ASCII fields to the exact canonical byte cap.
	b := fixture(t)
	b.Decision.Proposal.Args = make([]string, 256)
	for i := range b.Decision.Proposal.Args {
		b.Decision.Proposal.Args[i] = strings.Repeat("x", 4000)
	}
	raw, err := ab.EncodeV1(b)
	if err != nil {
		t.Fatal(err)
	}
	remaining := (1 << 20) - len(raw)
	for i := range b.Decision.Proposal.Args {
		add := min(remaining, 96)
		b.Decision.Proposal.Args[i] += strings.Repeat("x", add)
		remaining -= add
	}
	if remaining != 0 {
		t.Fatalf("fixture cannot reach byte cap: %d", remaining)
	}
	raw, err = ab.EncodeV1(b)
	if err != nil || len(raw) != 1<<20 {
		t.Fatalf("exact byte cap %d, %v", len(raw), err)
	}
	if _, err := ab.DecodeV1(raw); err != nil {
		t.Fatalf("decode byte cap: %v", err)
	}
	b.Decision.Proposal.Args[255] += "x"
	if raw, err := ab.EncodeV1(b); raw != nil || !errors.Is(err, ab.ErrCapabilityUnsupported) {
		t.Fatalf("encode over byte cap: %v", err)
	}
}

func TestBindingV1OptionalEvidence(t *testing.T) {
	b := fixture(t)
	b.Request.Ceilings = ab.OptionalCeilingsV1{}
	b.Decision.EnvelopeFlags = nil
	b.Decision.ResolvedCeilings = ab.FiniteCeilingsV1{ExecutionNanos: 120000000000, Repairs: 3, Replans: 1, TransportRetries: 5}
	b.Decision.Declaration = nil
	b.Request.DeclareEntitlement = ""
	b.Decision.Profile = ab.ProfileV1{Name: "trusted-host", Consent: "--execution-profile", Disclosure: "synthetic"}
	b.Request.ExecutionProfile = "trusted-host"
	b.Decision.Proposal.Capabilities.Billing.EvidenceID = nil
	b.Decision.Proposal.Capabilities.Capabilities.Reconnect = nil
	supported := ab.TriV1("supported")
	b.Decision.Proposal.Capabilities.Capabilities.ModelMetadata = &supported
	raw, err := ab.EncodeV1(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ab.DecodeV1(raw)
	if err != nil || got.Decision.EnvelopeFlags != nil || got.Decision.Declaration != nil || got.Decision.Profile.Boundary != nil || got.Decision.Proposal.Capabilities.Capabilities.Reconnect != nil || got.Decision.Proposal.Capabilities.Capabilities.ModelMetadata == nil || *got.Decision.Proposal.Capabilities.Capabilities.ModelMetadata != "supported" {
		t.Fatalf("optional state changed: %v", err)
	}
	if b.Decision.EnvelopeFlags != nil {
		t.Fatal("encoding mutated input")
	}
}

func TestBindingV1DeclaredInputsAgree(t *testing.T) {
	for name, mutate := range map[string]func(*ab.BindingV1){
		"native grant digest": func(b *ab.BindingV1) {
			b.Decision.NativeTrustGrant = "native_config:sha256:" + strings.Repeat("ac", 32)
		},
		"project flag without intent": func(b *ab.BindingV1) { b.Decision.RecordTrust = false },
		"native flag without intent":  func(b *ab.BindingV1) { b.Decision.RecordNativeTrust = false },
		"declaration absent":          func(b *ab.BindingV1) { b.Decision.Declaration = nil },
		"declaration changed":         func(b *ab.BindingV1) { b.Request.DeclareEntitlement = "plan=max,extra-usage=disabled" },
		"declaration duplicate":       func(b *ab.BindingV1) { b.Request.DeclareEntitlement = "plan=pro,plan=pro,extra-usage=disabled" },
		"auth identity":               func(b *ab.BindingV1) { b.Decision.NativeAuth.IdentityRef = "garbage" },
		"empty checks":                func(b *ab.BindingV1) { b.Decision.ProjectConfig.Checks = nil },
	} {
		t.Run(name, func(t *testing.T) {
			b := fixture(t)
			mutate(&b)
			if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
				t.Fatalf("accepted conflicting input: %v", err)
			}
		})
	}
}

func TestBindingV1SnapshotResolution(t *testing.T) {
	for _, tc := range []struct {
		name, revision, source string
		dirty, useCommitted    bool
	}{
		{"clean committed flag", "", "HEAD", false, true},
		{"clean default", "", "HEAD", false, false},
		{"dirty committed flag", "", "--use-committed", true, true},
		{"dirty confirmation", "", "interactive", true, false},
		{"explicit revision", "main", "--rev", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := fixture(t)
			b.Request.Revision = tc.revision
			b.Request.UseCommitted = tc.useCommitted
			b.Decision.Snapshot.Dirty = tc.dirty
			b.Decision.Snapshot.Source = tc.source
			if err := ab.ValidateV1(b); err != nil {
				t.Fatalf("pinned source selection: %v", err)
			}
		})
	}
}

func windowsFixture(t *testing.T, root string) ab.BindingV1 {
	t.Helper()
	b := fixture(t)
	b.Request.Repository = root + `\repo`
	b.Request.TaskFile = root + `\task.md`
	b.Decision.RunDir = root + `\state\runs\run_1`
	b.Decision.Workdir = b.Decision.RunDir + `\workspace`
	b.Decision.Snapshot.SourceRepo = b.Request.Repository
	b.Decision.RepoIdentity = b.Request.Repository
	b.Decision.Probe.Executable = root + `\native`
	b.Decision.Probe.OS = "windows"
	b.Decision.Proposal.Path = b.Decision.Probe.Executable
	b.Decision.Proposal.Dir = b.Decision.Workdir
	b.Decision.Proposal.Capabilities.Platform.OS = "windows"
	b.Decision.Profile.Boundary.OS = "windows"
	return b
}
func TestBindingV1OwningOSPaths(t *testing.T) {
	for _, root := range []string{`C:`, `c:`, `\\server\share`} {
		t.Run(root, func(t *testing.T) {
			b := windowsFixture(t, root)
			raw, err := ab.EncodeV1(b)
			if err != nil {
				t.Fatalf("absolute Windows record: %v", err)
			}
			got, err := ab.DecodeV1(raw)
			if err != nil || !reflect.DeepEqual(b, got) {
				t.Fatalf("Windows roundtrip: %v", err)
			}
		})
	}
	for name, requestPath := range map[string]string{
		"volume relative": `C:repo`, "rooted without volume": `\repo`, "POSIX in Windows": `/tmp/repo`, "mixed separators": `C:\repo/sub`, "traversal": `C:\repo\..\repo`, "dot": `C:\repo\.`, "trailing separator": `C:\repo\`, "UNC server only": `\\server`, "UNC no share": `\\server\\repo`,
	} {
		t.Run(name, func(t *testing.T) {
			b := windowsFixture(t, `C:`)
			b.Request.Repository = requestPath
			if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
				t.Fatalf("noncanonical Windows path accepted: %v", err)
			}
		})
	}
	// These source-OS controls run on every host; decoding stored evidence is
	// separate from qualifying execution on the current host.
	b := fixture(t)
	if err := ab.ValidateV1(b); err != nil {
		t.Fatalf("POSIX record: %v", err)
	}
	b.Request.Repository = "tmp/repo"
	if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
		t.Fatalf("relative POSIX path accepted: %v", err)
	}
}

func fakeFixture(t *testing.T) ab.BindingV1 {
	t.Helper()
	b := fixture(t)
	b.Request.Adapter = "fake"
	b.Request.Billing = "local-scripted"
	b.Request.Scenario = "happy"
	b.Request.DeclareEntitlement = ""
	b.Request.StripCredentialEnv = false
	b.Request.AllowUntestedNativeVersion = false
	b.Request.TrustNativeConfig = ""
	b.Decision.Scenario = "happy"
	b.Decision.Declaration = nil
	b.Decision.Adapter.ID = "builtin/fake"
	b.Decision.Adapter.Harness = "fake"
	b.Decision.Profile.Boundary.Route = "builtin/fake"
	b.Decision.RecordNativeTrust = false
	b.Decision.NativeConfigDigest = ""
	b.Decision.NativeTrustGrant = ""
	b.Decision.NativeHooks = nil
	b.Decision.NativeAuth = nil
	b.Decision.Environment.NativeConfigIdentity = ""
	b.Decision.Proposal.Billing.Mode = "local-scripted"
	b.Decision.Proposal.Capabilities.AdapterID = "builtin/fake"
	b.Decision.Proposal.Capabilities.HarnessID = "fake"
	return b
}
func TestBindingV1PinnedSourceSelectors(t *testing.T) {
	for _, name := range fake.Scenarios() {
		t.Run(name, func(t *testing.T) {
			b := fakeFixture(t)
			b.Request.Scenario = name
			b.Decision.Scenario = name
			if err := ab.ValidateV1(b); err != nil {
				t.Fatalf("current fake scenario: %v", err)
			}
		})
	}
	t.Run("unknown fake scenario", func(t *testing.T) {
		b := fakeFixture(t)
		b.Request.Scenario = "future-unreviewed"
		b.Decision.Scenario = b.Request.Scenario
		if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
			t.Fatalf("unknown source selector: %v", err)
		}
	})
	t.Run("boundary source route", func(t *testing.T) {
		b := fixture(t)
		b.Decision.Profile.Boundary.Route = b.Decision.Adapter.ID
		if err := ab.ValidateV1(b); err != nil {
			t.Fatalf("actual containment route: %v", err)
		}
	})
}

func TestBindingV1RequestRepositoryProvenance(t *testing.T) {
	b := fixture(t)
	b.Request.Repository = "/tmp/repo/subdirectory"
	if err := ab.ValidateV1(b); err != nil {
		t.Fatalf("request within source repo: %v", err)
	}
	b = windowsFixture(t, `C:`)
	b.Request.Repository = `C:\repo\subdirectory`
	if err := ab.ValidateV1(b); err != nil {
		t.Fatalf("Windows request within source repo: %v", err)
	}
}

func TestBindingV1CurrentMainSelector(t *testing.T) {
	b := fixture(t)
	d := claudecode.New().Descriptor()
	b.Request.Adapter = admission.AdapterClaudeCode
	b.Decision.Adapter = ab.DescriptorV1{ID: d.ID, Version: d.Version, Harness: d.Harness, Surface: d.Surface}
	b.Decision.Proposal.Capabilities.AdapterID = d.ID
	b.Decision.Proposal.Capabilities.AdapterVersion = d.Version
	b.Decision.Proposal.Capabilities.HarnessID = d.Harness
	b.Decision.Proposal.Capabilities.ExecutionSurface = d.Surface
	if err := ab.ValidateV1(b); err != nil {
		t.Fatalf("actual registered selector/descriptor pair refused: %v", err)
	}
	b.Request.Adapter = "claude-code"
	if err := ab.ValidateV1(b); !errors.Is(err, ab.ErrInvalidContract) {
		t.Fatalf("non-selector harness label accepted: %v", err)
	}
}

func TestBindingV1ChallengeShape(t *testing.T) { // I03/I09/I20 (v2 section2): shape grants no consent.
	for _, tc := range []struct {
		name   string
		change func(*ab.BindingV1)
		want   error
	}{
		{"empty required confirmation evidence", func(b *ab.BindingV1) { b.Challenges = nil }, ab.ErrInvalidContract},
		{"duplicate identity", func(b *ab.BindingV1) { b.Challenges[1].ID = b.Challenges[0].ID }, ab.ErrInvalidContract},
		{"invalid identity", func(b *ab.BindingV1) { b.Challenges[0].ID = "../foreign" }, ab.ErrInvalidContract},
		{"empty kind", func(b *ab.BindingV1) { b.Challenges[0].Kind = "" }, ab.ErrInvalidContract},
		{"empty text", func(b *ab.BindingV1) { b.Challenges[0].Text = "" }, ab.ErrInvalidContract},
		{"nul text", func(b *ab.BindingV1) { b.Challenges[0].Text = "a\x00b" }, ab.ErrInvalidContract},
		{"string bound", func(b *ab.BindingV1) { b.Challenges[0].Text = strings.Repeat("x", 4097) }, ab.ErrCapabilityUnsupported},
		{"count bound", func(b *ab.BindingV1) {
			for i := len(b.Challenges); i < 129; i++ {
				b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: fmt.Sprintf("ch_%d", i), Kind: "synthetic", Text: "fixture"})
			}
		}, ab.ErrCapabilityUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := fixture(t)
			tc.change(&b)
			raw, err := ab.EncodeV1(b)
			if raw != nil || !errors.Is(err, tc.want) {
				t.Fatalf("shape output %q, error %v", raw, err)
			}
		})
	}
	// These are well-formed DIFFERENT data, never confirmations or authority.
	// The independently required Task8 planner must reject unauthorized semantic
	// substitutions before ANY confirmation/retention/durable row/ack or effect.
	original := fixture(t)
	base, err := ab.DigestV1(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*ab.BindingV1)
	}{
		{"changed unique identity", func(b *ab.BindingV1) { b.Challenges[0].ID = "different_unique" }},
		{"changed kind", func(b *ab.BindingV1) { b.Challenges[0].Kind = "other-semantic-kind" }},
		{"changed text", func(b *ab.BindingV1) { b.Challenges[0].Text = "Different authority wording" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := fixture(t)
			tc.change(&b)
			got, err := ab.DigestV1(b)
			if err != nil || got == base {
				t.Fatalf("different structural data not distinguished: %x, %v", got, err)
			}
			raw, err := ab.EncodeV1(b)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := ab.DecodeV1(raw)
			if err != nil || !reflect.DeepEqual(decoded.Challenges, b.Challenges) {
				t.Fatalf("different data lost: %v", err)
			}
		})
	}
	b := fixture(t)
	b.Challenges = nil
	b.Decision.KeepGoing = false
	b.Request.KeepGoing = false
	b.Decision.RecordTrust = false
	b.Decision.RecordNativeTrust = false
	b.Request.TrustProjectConfig = ""
	b.Request.TrustNativeConfig = ""
	raw, err := ab.EncodeV1(b)
	if err != nil || !bytes.Contains(raw, []byte(`"Challenges":[]`)) {
		t.Fatalf("no confirmation intent cannot represent empty collection: %v", err)
	}
}

func TestBindingV1NativeTrustFlagSource(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	for _, raw := range []string{"sha256:" + digest, "sha256:" + strings.ToUpper(digest), digest, "sha256:short"} {
		t.Run(raw, func(t *testing.T) {
			record, sourceErr := admission.CheckNativeTrustConfig(t.Context(), nil, "/tmp/repo", admission.NativeConfig{Digest: digest, RequiresTrust: true}, raw)
			b := fixture(t)
			b.Request.TrustNativeConfig = raw
			codecErr := ab.ValidateV1(b)
			if raw == "sha256:"+digest {
				if sourceErr != nil || !record || codecErr != nil {
					t.Fatalf("pinned flag grammar rejected: source=%v record=%v codec=%v", sourceErr, record, codecErr)
				}
			} else if !errors.Is(sourceErr, admission.ErrInvalid) || !errors.Is(codecErr, ab.ErrInvalidContract) {
				t.Fatalf("grammar differs: source=%v codec=%v", sourceErr, codecErr)
			}
		})
	}
}
