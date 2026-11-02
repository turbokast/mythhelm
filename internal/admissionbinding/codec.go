package admissionbinding

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrInvalidContract identifies malformed or internally inconsistent evidence.
var ErrInvalidContract = errors.New("admissionbinding: invalid contract")

// ErrCapabilityUnsupported identifies a well-formed unsupported version or bound.
var ErrCapabilityUnsupported = errors.New("admissionbinding: capability unsupported")

const maxBytes = 1 << 20
const domainV1 = "MYTHHELM:service-admission-binding:v1\n"

// EncodeV1 validates and returns independent canonical bytes. Nil collections
// encode as empty arrays; optional objects retain explicit nulls.
func EncodeV1(b BindingV1) ([]byte, error) {
	if err := ValidateV1(b); err != nil {
		return nil, err
	}
	return canonical(b)
}

// DigestV1 identifies canonical evidence; it grants no authority.
func DigestV1(b BindingV1) ([32]byte, error) {
	raw, err := EncodeV1(b)
	if err != nil {
		return [32]byte{}, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(domainV1))
	_, _ = h.Write(raw)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// DecodeV1 refuses ambiguity before constructing a usable binding. Neither
// encoding/json's case folding nor its duplicate/null acceptance is used as a
// schema validator. Failed outputs are always zero.
func DecodeV1(raw []byte) (BindingV1, error) {
	if len(raw) > maxBytes {
		return BindingV1{}, fmt.Errorf("%w: byte bound", ErrCapabilityUnsupported)
	}
	if !utf8.Valid(raw) {
		return BindingV1{}, fmt.Errorf("%w: UTF-8", ErrInvalidContract)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := shape(dec, reflect.TypeFor[BindingV1](), 0); err != nil {
		return BindingV1{}, fmt.Errorf("%w: shape", ErrInvalidContract)
	}
	if _, err := dec.Token(); err != io.EOF {
		return BindingV1{}, fmt.Errorf("%w: trailing input", ErrInvalidContract)
	}
	var b BindingV1
	if err := json.Unmarshal(raw, &b); err != nil {
		return BindingV1{}, fmt.Errorf("%w: value", ErrInvalidContract)
	}
	encoded, err := canonical(b)
	if err != nil {
		return BindingV1{}, err
	}
	if !bytes.Equal(raw, encoded) {
		return BindingV1{}, fmt.Errorf("%w: noncanonical", ErrInvalidContract)
	}
	if err := ValidateV1(b); err != nil {
		return BindingV1{}, err
	}
	return b, nil
}

func shape(d *json.Decoder, t reflect.Type, depth int) error {
	if depth > 64 {
		return ErrInvalidContract
	}
	tok, err := d.Token()
	if err != nil {
		return err
	}
	if t.Kind() == reflect.Pointer {
		if tok == nil {
			return nil
		}
		return shapeToken(d, t.Elem(), depth, tok)
	}
	if tok == nil {
		return ErrInvalidContract
	}
	return shapeToken(d, t, depth, tok)
}
func shapeToken(d *json.Decoder, t reflect.Type, depth int, tok json.Token) error {
	switch t.Kind() {
	case reflect.Struct:
		if tok != json.Delim('{') {
			return ErrInvalidContract
		}
		seen := make(map[string]bool, t.NumField())
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrInvalidContract
			}
			field, ok := t.FieldByName(name)
			if !ok || field.Name != name {
				return ErrInvalidContract
			}
			seen[name] = true
			if err := shape(d, field.Type, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') || len(seen) != t.NumField() {
			return ErrInvalidContract
		}
	case reflect.Slice:
		if tok != json.Delim('[') {
			return ErrInvalidContract
		}
		for d.More() {
			if err := shape(d, t.Elem(), depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrInvalidContract
		}
	case reflect.String:
		if _, ok := tok.(string); !ok {
			return ErrInvalidContract
		}
	case reflect.Bool:
		if _, ok := tok.(bool); !ok {
			return ErrInvalidContract
		}
	case reflect.Int64:
		n, ok := tok.(json.Number)
		if !ok {
			return ErrInvalidContract
		}
		if _, err := n.Int64(); err != nil {
			return ErrInvalidContract
		}
	default:
		return ErrInvalidContract
	}
	return nil
}

// canonical uses only closed DTO fields. Copying recursively also prevents
// normalisation from modifying caller-owned slices or nullable objects.
func canonical(b BindingV1) ([]byte, error) {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(collections(reflect.ValueOf(b)).Interface()); err != nil {
		return nil, fmt.Errorf("%w: encode", ErrInvalidContract)
	}
	raw := bytes.TrimSuffix(out.Bytes(), []byte{'\n'})
	// encoding/json escapes these for JSONP even with HTML escaping disabled;
	// binding JSON is UTF-8 data, never executable JavaScript.
	var utf8JSON []byte
	for i := 0; i < len(raw); {
		if raw[i] == '\\' && i+1 < len(raw) {
			if i+6 <= len(raw) && (string(raw[i:i+6]) == `\u2028` || string(raw[i:i+6]) == `\u2029`) {
				if raw[i+5] == '8' {
					utf8JSON = append(utf8JSON, 0xe2, 0x80, 0xa8)
				} else {
					utf8JSON = append(utf8JSON, 0xe2, 0x80, 0xa9)
				}
				i += 6
				continue
			}
			// Consume the entire escape, especially escaped backslashes, before
			// looking for another Unicode escape token.
			utf8JSON = append(utf8JSON, raw[i:i+2]...)
			i += 2
			continue
		}
		utf8JSON = append(utf8JSON, raw[i])
		i++
	}
	raw = utf8JSON
	return raw, nil
}
func collections(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Struct:
		c := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			c.Field(i).Set(collections(v.Field(i)))
		}
		return c
	case reflect.Slice:
		c := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			c.Index(i).Set(collections(v.Index(i)))
		}
		return c
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		c := reflect.New(v.Type().Elem())
		c.Elem().Set(collections(v.Elem()))
		return c
	default:
		return v
	}
}

type validation struct{ invalid, unsupported bool }

func (v *validation) require(ok bool) {
	if !ok {
		v.invalid = true
	}
}
func (v *validation) schema(got, want int64) {
	if got > want {
		v.unsupported = true
	} else {
		v.require(got == want)
	}
}
func (v *validation) walk(x reflect.Value, name string) {
	switch x.Kind() {
	case reflect.Struct:
		for i := 0; i < x.NumField(); i++ {
			v.walk(x.Field(i), x.Type().Field(i).Name)
		}
	case reflect.Pointer:
		if !x.IsNil() {
			v.walk(x.Elem(), name)
		}
	case reflect.Slice:
		limit := 0
		switch name {
		case "Args", "Argv", "Passthrough", "AllowedTools", "Manifest":
			limit = 256
		case "Challenges":
			limit = 128
		case "Checks":
			limit = 64
		case "StopLadder":
			limit = 16
		}
		if limit > 0 && x.Len() > limit {
			v.unsupported = true
		}
		for i := 0; i < x.Len(); i++ {
			v.walk(x.Index(i), name)
		}
	case reflect.String:
		s := x.String()
		v.require(utf8.ValidString(s) && !strings.ContainsRune(s, 0))
		limit := 4096
		if name == "Title" {
			limit = 256
		}
		if len(s) > limit {
			v.unsupported = true
		}
		if x.Type() == reflect.TypeFor[TriV1]() {
			v.require(oneOf(s, "supported", "unsupported", "unknown"))
		}
	case reflect.Int64:
		v.require(x.Int() >= 0)
	}
}

// ValidateV1 enforces the pure closed-data contract, without probing files,
// looking up authority or consulting a clock. Live expiry, artefact bytes,
// physical identities, retention and qualification are revalidated by the
// service's later producers, never inferred from codec validity.
func ValidateV1(b BindingV1) error {
	v := validation{}
	v.walk(reflect.ValueOf(b), "")
	v.schema(b.SchemaVersion, 1)
	v.schema(b.Request.SchemaVersion, 2)
	d, r := b.Decision, b.Request
	p := d.Proposal
	c := p.Capabilities
	v.schema(c.SchemaVersion, 1)
	for _, id := range []string{b.PreparationID, b.RunID, b.TaskID, b.AttemptID} {
		v.require(validID(id))
	}
	for _, s := range []string{b.PeerID, b.RootID, b.ContextEpoch, b.PolicyRevision, d.Environment.PrincipalRef, d.GitName, d.GitEmail, d.Adapter.Version, d.Adapter.Surface, d.Probe.Version, d.Probe.Compatibility, c.Mode, c.ExecutionSurface, c.FidelityQualification, c.Qualification} {
		v.require(s != "")
	}
	v.require(b.Generation > 0 && timestamp(b.ExpiresAt))
	v.require(b.RootID == d.StateRootID && b.RunID == d.RunID && b.TaskID == d.TaskID && b.AttemptID == d.AttemptID)
	v.require(b.ContextEpoch == d.Environment.ContextEpoch && b.PolicyRevision == d.Environment.PolicyRevision)
	for _, pathText := range []string{r.Repository, r.TaskFile, d.RunDir, d.Workdir, d.Snapshot.SourceRepo, d.RepoIdentity, d.Probe.Executable, p.Path, p.Dir} {
		v.require(absClean(pathText, d.Probe.OS))
	}
	v.require(path.Base(sourcePath(d.RunDir, d.Probe.OS)) == b.RunID && path.Base(path.Dir(sourcePath(d.RunDir, d.Probe.OS))) == "runs")
	v.require(sourcePath(d.Workdir, d.Probe.OS) == sourcePath(d.RunDir, d.Probe.OS)+"/workspace" && p.Dir == d.Workdir)
	v.require(d.RepoIdentity == d.Snapshot.SourceRepo && withinRepository(r.Repository, d.Snapshot.SourceRepo, d.Probe.OS))
	v.require(r.Host == "" || r.Host == "standalone")
	v.require(d.Host == "standalone")
	v.require(r.NoChecks == d.NoChecks && r.KeepGoing == d.KeepGoing)
	v.require(!r.UseCommitted || r.Revision == "")
	v.require(fullCommit(d.Snapshot.BaseRev))
	v.require(oneOf(d.Snapshot.Source, "HEAD", "--use-committed", "interactive", "--rev"))
	expectedSource := "HEAD"
	switch {
	case r.Revision != "":
		expectedSource = "--rev"
	case !d.Snapshot.Dirty:
	case r.UseCommitted:
		expectedSource = "--use-committed"
	default:
		expectedSource = "interactive"
	}
	v.require(d.Snapshot.Source == expectedSource)
	adapterID := ""
	harnessID := r.Adapter
	switch r.Adapter {
	case "fake":
		adapterID = "builtin/fake"
		v.require(r.Billing == "local-scripted")
		v.require(r.Scenario == d.Scenario || r.Scenario == "" && d.Scenario == "happy")
		v.require(oneOf(d.Scenario, "allowance-exhausted", "bad-utf8", "check-fails", "count-launches", "deep", "denied", "escapee", "exit-before-result", "happy", "ignore-sigint", "ignore-term", "malformed", "native-fails", "oversized", "paced", "readonly", "slow", "tree", "usage-counters"))
	case "claudecode":
		adapterID = "builtin/claudecode"
		harnessID = "claude-code"
		v.require(r.Scenario == "" && d.Scenario == "")
		v.require(oneOf(r.Billing, "subscription-declared", "subscription-only"))
	case "codex":
		adapterID = "builtin/codex"
		v.require(r.Scenario == "" && d.Scenario == "" && !r.StripCredentialEnv && r.DeclareEntitlement == "")
		v.require(oneOf(r.Billing, "subscription-declared", "subscription-only"))
	default:
		v.invalid = true
	}
	if r.Adapter == "fake" {
		v.require(!r.StripCredentialEnv && !r.AllowUntestedNativeVersion && r.TrustNativeConfig == "" && r.DeclareEntitlement == "")
	}
	v.require(d.Adapter.ID == adapterID && d.Adapter.Harness == harnessID && c.AdapterID == adapterID && c.HarnessID == harnessID)
	v.require(c.AdapterVersion == d.Adapter.Version && c.RuntimeVersion == d.Probe.Version && c.ExecutionSurface == d.Adapter.Surface && c.Platform.OS == d.Probe.OS && p.Path == d.Probe.Executable)
	v.require(oneOf(c.Mode, "headless", "interactive"))
	v.require(oneOf(d.Probe.OS, "linux", "darwin", "windows"))
	v.require(oneOf(d.Probe.Arch, "amd64", "arm64"))
	v.require(lowerHash(d.Probe.SHA256) && lowerHash(d.Task.SHA256))
	taskHash, err := hex.DecodeString(strings.TrimPrefix(r.TaskSHA256, "sha256:"))
	v.require(err == nil && len(taskHash) == 32 && hex.EncodeToString(taskHash) == d.Task.SHA256)
	v.require(d.Task.Bytes > 0)
	if d.Task.Bytes > maxBytes {
		v.unsupported = true
	}
	v.require(relativeArtifact(d.Task.ArtifactRef))
	v.require(p.Billing.Mode == r.Billing && oneOf(p.Billing.Mode, "local-scripted", "subscription-declared", "subscription-only"))
	if c.Capabilities.StructuredEvents != "supported" {
		v.unsupported = true
	}
	profile := r.ExecutionProfile
	if profile == "" {
		profile = "restricted"
	}
	v.require(profile == d.Profile.Name && oneOf(profile, "trusted-host", "restricted", "inspect"))
	if profile == "trusted-host" {
		v.require(!d.Profile.Contained && d.Profile.Boundary == nil)
	} else {
		v.require(d.Profile.Contained && d.Profile.Boundary != nil)
		if bound := d.Profile.Boundary; bound != nil {
			v.require(bound.Profile == profile && bound.OS == d.Probe.OS && bound.Route == d.Adapter.ID && bound.Boundary != "" && bound.Version != "" && bound.Owner != "")
			for _, claim := range []ClaimV1{bound.Coverage.Filesystem, bound.Coverage.Process, bound.Coverage.Network, bound.Coverage.Credential} {
				if !claim.Enforced {
					v.unsupported = true
				}
				v.require(claim.Name != "" && claim.Version != "")
			}
		}
	}
	if d.NoChecks {
		v.require(d.ConfigDigest == "" && zeroConfig(d.ProjectConfig) && !d.RecordTrust && r.TrustProjectConfig == "")
	} else {
		v.schema(d.ProjectConfig.SchemaVersion, 1)
		v.require(lowerHash(d.ConfigDigest) && len(d.ProjectConfig.Checks) > 0)
	}
	v.require(trustFlag(r.TrustProjectConfig, d.ConfigDigest) && trustFlag(r.TrustNativeConfig, d.NativeConfigDigest))
	if d.NativeConfigDigest != "" {
		v.require(lowerHash(d.NativeConfigDigest) && d.NativeHooks != nil)
	} else {
		v.require(d.NativeHooks == nil && !d.RecordNativeTrust && d.NativeTrustGrant == "")
	}
	if d.NativeTrustGrant != "" {
		v.require(d.NativeConfigDigest != "" && d.NativeTrustGrant == "native_config:sha256:"+d.NativeConfigDigest)
	}
	if d.RecordNativeTrust {
		v.require(d.NativeTrustGrant != "")
	}
	if r.TrustProjectConfig != "" {
		v.require(d.RecordTrust)
	}
	if r.TrustNativeConfig != "" {
		v.require(d.RecordNativeTrust)
	}
	if d.RecordTrust {
		v.require(!d.NoChecks)
	}
	if auth := d.NativeAuth; auth != nil {
		v.require(lowerHash(auth.IdentityRef) && auth.ConfigIdentity != "" && auth.ConfigIdentity == d.Environment.NativeConfigIdentity)
	}
	if r.DeclareEntitlement != "" {
		plan, ok := declaredPlan(r.DeclareEntitlement)
		v.require(ok && d.Declaration != nil)
		if d.Declaration != nil {
			v.require(d.Declaration.PlanClass == plan)
		}
	}
	if decl := d.Declaration; decl != nil {
		v.require(decl.AdapterID == d.Adapter.ID && oneOf(decl.PlanClass, "pro", "max", "team", "enterprise") && decl.ExtraUsage == "disabled" && lowerHash(decl.IdentityRef) && timestamp(decl.DeclaredAt))
		if d.NativeAuth != nil {
			v.require(decl.IdentityRef == d.NativeAuth.IdentityRef)
		}
	}
	v.require(sortedUnique(d.Environment.RequiredNames) && sortedUnique(d.Environment.Destinations))
	for i, m := range p.Manifest {
		v.require(m.Source != "" && lowerHash(m.Digest))
		if i > 0 {
			v.require(p.Manifest[i-1].Source < m.Source)
		}
	}
	for _, s := range p.StopLadder {
		v.require(oneOf(s.Signal, "interrupt", "terminate", "kill"))
	}
	v.require(len(p.StopLadder) > 0)
	for _, tool := range d.ProjectConfig.AllowedTools {
		v.require(tool != "" && !strings.HasPrefix(tool, "-"))
		if len(tool) > 1024 {
			v.unsupported = true
		}
	}
	seen := map[string]bool{}
	for _, check := range d.ProjectConfig.Checks {
		v.require(checkName(check.Name) && !seen[check.Name] && len(check.Argv) > 0)
		seen[check.Name] = true
		if len(check.Argv) > 0 {
			v.require(strings.TrimSpace(check.Argv[0]) != "")
		}
		v.require(positiveDuration(check.Timeout))
	}
	validateCeilings(&v, b)
	seen = map[string]bool{}
	for _, ch := range b.Challenges {
		v.require(validID(ch.ID) && ch.Kind != "" && ch.Text != "" && !seen[ch.ID])
		seen[ch.ID] = true
	}
	// This leaf validates challenge shape only. The merged Task8 contract
	// requires exact fresh planned-set equality before any confirmation,
	// retention, durable record, acknowledgement, admission or dispatch effect.
	// Different well-formed data and its recomputed digest never supply consent.

	if d.NoChecks || d.KeepGoing || d.RecordTrust || d.RecordNativeTrust {
		v.require(len(b.Challenges) > 0)
	}
	if v.invalid {
		return fmt.Errorf("%w: inconsistent binding", ErrInvalidContract)
	}
	if v.unsupported {
		return fmt.Errorf("%w: version, capability or bound", ErrCapabilityUnsupported)
	}
	raw, err := canonical(b)
	if err != nil {
		return err
	}
	if len(raw) > maxBytes {
		return fmt.Errorf("%w: byte bound", ErrCapabilityUnsupported)
	}
	return nil
}

func validateCeilings(v *validation, b BindingV1) {
	r, d := b.Request, b.Decision
	v.require(reflect.DeepEqual(r.Ceilings, optional(d.EnvelopeFlags)))
	if r.Ceilings.ExecutionNanos != nil {
		v.require(*r.Ceilings.ExecutionNanos > 0 && *r.Ceilings.ExecutionNanos%1000000 == 0)
	}
	resolved := FiniteCeilingsV1{1800000000000, 3, 2, 5}
	cfg := d.ProjectConfig.Envelopes
	if cfg.Execution != "" {
		dur, err := time.ParseDuration(cfg.Execution)
		v.require(err == nil && dur > 0)
		resolved.ExecutionNanos = int64(dur)
	}
	for _, row := range []struct {
		in  *int64
		out *int64
	}{{cfg.Repairs, &resolved.Repairs}, {cfg.Replans, &resolved.Replans}, {cfg.TransportRetries, &resolved.TransportRetries}, {r.Ceilings.ExecutionNanos, &resolved.ExecutionNanos}, {r.Ceilings.Repairs, &resolved.Repairs}, {r.Ceilings.Replans, &resolved.Replans}, {r.Ceilings.TransportRetries, &resolved.TransportRetries}} {
		if row.in != nil {
			*row.out = *row.in
		}
	}
	v.require(resolved == d.ResolvedCeilings && resolved.ExecutionNanos > 0)
}
func optional(p *OptionalCeilingsV1) OptionalCeilingsV1 {
	if p == nil {
		return OptionalCeilingsV1{}
	}
	return *p
}
func zeroConfig(c ProjectConfigV1) bool {
	return c.SchemaVersion == 0 && len(c.Passthrough) == 0 && len(c.AllowedTools) == 0 && len(c.Checks) == 0 && c.Envelopes.Execution == "" && c.Envelopes.Repairs == nil && c.Envelopes.Replans == nil && c.Envelopes.TransportRetries == nil
}
func oneOf(s string, values ...string) bool {
	return slices.Contains(values, s)
}
func validID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
func lowerHash(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func fullCommit(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	if s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func trustFlag(raw, digest string) bool {
	return raw == "" || lowerHash(digest) && raw == "sha256:"+digest
}

// sourcePath is only a structural comparison spelling. It never resolves a
// physical path and never changes the stored case or bytes.
func sourcePath(s, osName string) string {
	if osName == "windows" {
		return strings.ReplaceAll(s, `\`, "/")
	}
	return s
}
func absClean(s, osName string) bool {
	if osName != "windows" {
		return path.IsAbs(s) && path.Clean(s) == s
	}
	if strings.Contains(s, "/") {
		return false
	} // Windows Clean uses backslashes.
	if len(s) >= 3 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':' && s[2] == '\\' {
		tail := sourcePath(s[2:], osName)
		return path.Clean(tail) == tail
	}
	if strings.HasPrefix(s, `\\`) {
		parts := strings.Split(s[2:], `\`)
		if len(parts) < 2 {
			return false
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." || part == "?" {
				return false
			}
		}
		return true
	}
	return false
}
func relativeArtifact(s string) bool {
	return s != "" && s != "." && !path.IsAbs(s) && !strings.ContainsAny(s, `\:`) && path.Clean(s) == s && s != ".." && !strings.HasPrefix(s, "../")
}
func timestamp(s string) bool {
	t, err := time.Parse(time.RFC3339Nano, s)
	return err == nil && !t.IsZero() && t.Format(time.RFC3339Nano) == s && strings.HasSuffix(s, "Z")
}
func positiveDuration(s string) bool { d, err := time.ParseDuration(s); return err == nil && d > 0 }
func checkName(s string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`).MatchString(s)
}
func sortedUnique(values []string) bool {
	for i, s := range values {
		if s == "" || i > 0 && values[i-1] >= s {
			return false
		}
	}
	return true
}

// declaredPlan mirrors the pinned admission parser: trim each comma-delimited
// part, reject duplicate/unknown keys, and preserve the original request text.
func declaredPlan(raw string) (string, bool) {
	fields := map[string]string{}
	for part := range strings.SplitSeq(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || key == "" || value == "" {
			return "", false
		}
		if _, seen := fields[key]; seen {
			return "", false
		}
		fields[key] = value
	}
	return fields["plan"], len(fields) == 2 && oneOf(fields["plan"], "pro", "max", "team", "enterprise") && fields["extra-usage"] == "disabled"
}

func withinRepository(request, root, osName string) bool {
	request, root = sourcePath(request, osName), sourcePath(root, osName)
	return request == root || strings.HasPrefix(request, strings.TrimSuffix(root, "/")+"/")
}
