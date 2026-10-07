package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/buildinfo"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/statedir"
)

// doctorProbeTimeout bounds every subprocess doctor runs. A hung version
// query degrades to a reported failure, never a hung doctor.
const doctorProbeTimeout = 10 * time.Second

// sandboxTools are the Unix containment helpers doctor reports the
// presence of. The list is informational: this slice never uses them.
var sandboxTools = []string{"bwrap", "nsjail", "firejail"}

// nativeVersionLine matches `claude --version` output.
var nativeVersionLine = regexp.MustCompile(`^(\d+\.\d+\.\d+) \(Claude Code\)$`)

func runDoctor(args []string, stdio Stdio) error {
	fs := newFlagSet("doctor")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	positional, err := parseFlags(fs, args, stdio)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usageErrorf("unexpected argument %q", positional[0])
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	}
	rep := diagnose(os.Environ())
	if *format == "jsonl" {
		return json.NewEncoder(stdio.Out).Encode(rep)
	}
	return writeDoctorPlain(stdio.Out, rep)
}

// doctorReport is every fact AC-12.1 names. Unknown measurements stay
// explicit strings; credential values never enter it.
type doctorReport struct {
	Type          string         `json:"type"`
	Mythhelm      map[string]any `json:"mythhelm"`
	Git           map[string]any `json:"git"`
	Claude        map[string]any `json:"claude"`
	CredEnv       []string       `json:"credential_env_names"`
	Settings      map[string]any `json:"native_settings"`
	Sandbox       map[string]any `json:"sandbox"`
	StateDir      map[string]any `json:"state_dir"`
	Terminal      map[string]any `json:"terminal"`
	Qualification map[string]any `json:"qualification"`
}

func diagnose(env []string) doctorReport {
	info := buildinfo.Get()
	termOut, termErr := termFacts()
	return doctorReport{
		Type:     "doctor",
		Mythhelm: map[string]any{"version": info.Version, "commit": info.Commit, "go": info.GoVersion, "os": info.OS, "arch": info.Arch},
		Git:      gitFacts(),
		Claude:   claudeFacts(env),
		CredEnv:  credentialNames(env),
		Settings: settingsFacts(),
		Sandbox:  sandboxFacts(),
		StateDir: stateFacts(),
		Terminal: map[string]any{"TERM": termEnv("TERM"), "COLORTERM": termEnv("COLORTERM"),
			"NO_COLOR": os.Getenv("NO_COLOR") != "", "TERM_PROGRAM": termEnv("TERM_PROGRAM"), //nolint:misspell // NO_COLOR is the standard variable name.
			"stdout_terminal": termOut, "stderr_terminal": termErr},
		Qualification: qualificationFacts(),
	}
}

func gitFacts() map[string]any {
	path, err := exec.LookPath("git")
	if err != nil {
		return map[string]any{"path": "not found", "version": "unknown"}
	}
	out, err := runBounded("git", []string{"--version"})
	version := "query failed"
	if err == nil {
		version = strings.TrimSpace(string(out))
	}
	return map[string]any{"path": path, "version": version}
}

func claudeFacts(env []string) map[string]any {
	path, err := exec.LookPath("claude")
	if err != nil {
		return map[string]any{"path": "not found", "version": "unknown", "auth": "run admission to query"}
	}
	// The version query never inherits proxy variables: a version probe
	// must not become a network request (AC-12.2).
	scrubbed := slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		return upper == "HTTP_PROXY" || upper == "HTTPS_PROXY" || upper == "ALL_PROXY"
	})
	out, err := runBoundedEnv("claude", []string{"--version"}, scrubbed)
	version := "query failed"
	if err == nil {
		if m := nativeVersionLine.FindSubmatch(bytes.TrimSpace(out)); len(m) == 2 {
			version = string(m[1])
		} else {
			version = "unrecognised output"
		}
	}
	// Auth status may contact the network, so doctor never runs it:
	// admission queries the native identity when a run needs it.
	return map[string]any{"path": path, "version": version, "auth": "run admission to query"}
}

func credentialNames(env []string) []string {
	names := security.CredentialEnvNames(env)
	if names == nil {
		return []string{}
	}
	return names
}

func settingsFacts() map[string]any {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return map[string]any{"error": "home directory unknown"}
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = home
	}
	manifest, err := claudecode.InventorySettings(home, cwd)
	if err != nil {
		return map[string]any{"error": cell(err.Error())}
	}
	sources := make([]string, 0, len(manifest.Digests))
	for name, digest := range manifest.Digests {
		sources = append(sources, name+":sha256:"+digest)
	}
	slices.Sort(sources)
	return map[string]any{"sources": sources, "hooks": manifest.Hooks,
		"mcp_servers": manifest.MCPServers, "requires_trust": manifest.RequiresTrust}
}

func sandboxFacts() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"note": "no sandbox check on windows; reported only, not used in this slice"}
	}
	present := []string{}
	for _, tool := range sandboxTools {
		if _, err := exec.LookPath(tool); err == nil {
			present = append(present, tool)
		}
	}
	return map[string]any{"present": present, "note": "reported only, not used in this slice"}
}

func stateFacts() map[string]any {
	dir, err := statedir.Resolve()
	if err != nil {
		return map[string]any{"error": cell(err.Error())}
	}
	// Lstat only: a missing state dir is reported, never created (AC-12.2).
	// Only a non-existence error means absent; anything else (permissions,
	// I/O) is the kind of fault doctor exists to surface.
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{"path": dir, "status": "absent (not created)"}
	}
	if err != nil {
		return map[string]any{"path": dir, "status": "unreadable", "error": cell(err.Error())}
	}
	kind := "file"
	switch {
	case fi.IsDir():
		kind = "directory"
	case fi.Mode()&os.ModeSymlink != 0:
		kind = "symlink"
	}
	return map[string]any{"path": dir, "status": "present", "type": kind, "mode": fmt.Sprintf("%O", fi.Mode().Perm())}
}

// qualificationFacts reads the versioned qualification records through a
// read-only registry open (AC-5.3): the same map feeds plain and JSONL
// output. Every failure — an unresolvable state dir, a missing dir, an
// unreadable or version-mismatched database, a missing table — reads as
// unavailable with its reason, never an error exit and never fabricated
// rows (I09). An existing but unseeded dir reads as an empty record list:
// doctor never seeds.
func qualificationFacts() map[string]any {
	dir, err := statedir.Resolve()
	if err != nil {
		return map[string]any{"status": "unavailable (" + err.Error() + ")"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), doctorProbeTimeout)
	defer cancel()
	reg, err := qualify.OpenReadOnly(ctx, dir)
	if err != nil {
		return map[string]any{"status": "unavailable (" + err.Error() + ")"}
	}
	defer func() { _ = reg.Close() }()
	recs, err := reg.List(ctx)
	if err != nil {
		return map[string]any{"status": "unavailable (" + err.Error() + ")"}
	}
	out := make([]any, 0, len(recs))
	for _, rec := range recs {
		out = append(out, qualificationRecord(rec))
	}
	return map[string]any{"records": out}
}

// qualificationRecord renders one registry record for the doctor report:
// identity, progress, per-column verdicts with their evidence ids, the
// current revision count, the pinned drift triggers and the drift state.
func qualificationRecord(rec qualify.Record) map[string]any {
	return map[string]any{
		"harness":            rec.Key.Harness,
		"surface":            rec.Key.Surface,
		"progress":           string(rec.Progress),
		"fidelity":           qualificationColumn(rec.Fidelity),
		"entitlement":        qualificationColumn(rec.Entitlement),
		"lifecycle":          qualificationColumn(rec.Lifecycle),
		"evidence_revisions": rec.Revision,
		"latest_evidence":    latestEvidenceID(rec),
		"drift_triggers": map[string]any{
			"executable_digest": rec.Key.ExecutableDigest,
			"config_digest":     rec.Key.ConfigDigest,
		},
		"drift":     qualificationDrift(rec),
		"next_test": rec.NextTest,
	}
}

// qualificationColumn renders one column verdict with its evidence ids. The
// id list is never nil, so JSONL carries [] rather than null when a column
// has no evidence.
func qualificationColumn(col qualify.Column) map[string]any {
	ids := make([]string, 0, len(col.Evidence))
	for _, e := range col.Evidence {
		ids = append(ids, e.ID)
	}
	return map[string]any{"verdict": string(col.Verdict), "evidence": ids}
}

// qualificationDrift reports the record's drift state against unobserved
// digests: doctor performs no live probe. Pins still unknown read clean —
// nothing pinned can drift (I09) — while established pins read unobserved
// with the pin, failing closed instead of claiming a pinned build is still
// present.
func qualificationDrift(rec qualify.Record) string {
	if _, reason := qualify.CheckDrift(qualify.DriftInput{Record: rec}); reason != "" {
		return reason
	}
	return "clean"
}

// latestEvidenceID names the most recent evidence across the three columns
// by At (a zero At sorts oldest; column order breaks ties), or "none" when
// the record carries no evidence.
func latestEvidenceID(rec qualify.Record) string {
	latest := ""
	var latestAt time.Time
	found := false
	for _, col := range []qualify.Column{rec.Fidelity, rec.Entitlement, rec.Lifecycle} {
		for _, e := range col.Evidence {
			if !found || e.At.After(latestAt) {
				latest, latestAt, found = e.ID, e.At, true
			}
		}
	}
	if !found {
		return "none"
	}
	return latest
}

// qualificationPlain renders the qualification section: a header plus one
// line per record, or one honest line when the registry is unavailable or
// holds no records yet.
func qualificationPlain(q map[string]any) []string {
	if status, ok := q["status"].(string); ok {
		return []string{"qualification: " + status}
	}
	recs, _ := q["records"].([]any)
	if len(recs) == 0 {
		return []string{"qualification: (no records)"}
	}
	lines := []string{"qualification:"}
	for _, item := range recs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s × %s: %s (fidelity %s, entitlement %s, lifecycle %s) [evidence %d revs, latest %s; drift %s]",
			m["harness"], m["surface"], m["progress"],
			columnVerdict(m["fidelity"]), columnVerdict(m["entitlement"]), columnVerdict(m["lifecycle"]),
			m["evidence_revisions"], m["latest_evidence"], m["drift"]))
	}
	return lines
}

// columnVerdict reads one rendered column's verdict, defaulting to unknown
// when the report map does not carry one.
func columnVerdict(v any) string {
	if m, ok := v.(map[string]any); ok {
		if s, ok := m["verdict"].(string); ok {
			return s
		}
	}
	return "unknown"
}

func termEnv(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return "unset"
}

// termFacts reports whether the process's own stdout and stderr are
// character devices. Doctor reads the process streams, not the passed
// writers, because only they answer the terminal question.
func termFacts() (out, err bool) {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		fi, serr := f.Stat()
		char := serr == nil && fi.Mode()&os.ModeCharDevice != 0
		if f == os.Stdout {
			out = char
		} else {
			err = char
		}
	}
	return out, err
}

// runBounded runs a read-only version probe with the process environment.
func runBounded(name string, args []string) ([]byte, error) {
	return runBoundedEnv(name, args, os.Environ())
}

// maxProbeOutput caps one version probe's captured stdout. A version line
// needs a few hundred bytes; a hostile binary on PATH must not balloon
// doctor's memory inside its 10 s timeout.
const maxProbeOutput = 4096

func runBoundedEnv(name string, args []string, env []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), doctorProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- fixed version-probe argv, no shell
	cmd.Env = env
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	out, rerr := io.ReadAll(io.LimitReader(pipe, maxProbeOutput))
	_, _ = io.Copy(io.Discard, pipe)
	if werr := cmd.Wait(); werr != nil {
		return nil, werr
	}
	return out, rerr
}

func writeDoctorPlain(w io.Writer, r doctorReport) error {
	cred := "(none)"
	if len(r.CredEnv) > 0 {
		cred = strings.Join(r.CredEnv, ", ")
	}
	present := "(no check on this platform)"
	if p, ok := r.Sandbox["present"]; ok {
		present = fmt.Sprint(p)
	}
	state := fmt.Sprint(r.StateDir["status"])
	if r.StateDir["path"] != nil {
		state = fmt.Sprint(r.StateDir["path"]) + " (" + state + ")"
	}
	if r.StateDir["error"] != nil {
		state += ": " + fmt.Sprint(r.StateDir["error"])
	}
	settings := "sources: " + fmt.Sprint(r.Settings["sources"]) + "; hooks: " + fmt.Sprint(r.Settings["hooks"]) + "; mcp: " + fmt.Sprint(r.Settings["mcp_servers"])
	if r.Settings["error"] != nil {
		settings = "error: " + fmt.Sprint(r.Settings["error"])
	}
	lines := []string{
		"mythhelm: " + fmt.Sprint(r.Mythhelm["version"]) + " (" + fmt.Sprint(r.Mythhelm["os"]) + "/" + fmt.Sprint(r.Mythhelm["arch"]) + ")",
		"git: " + fmt.Sprint(r.Git["path"]) + " (" + fmt.Sprint(r.Git["version"]) + ")",
		"claude: " + fmt.Sprint(r.Claude["path"]) + " (" + fmt.Sprint(r.Claude["version"]) + "); auth: " + fmt.Sprint(r.Claude["auth"]),
		"credential env names: " + cred,
		"native settings: " + settings,
		"sandbox present: " + present + " (" + fmt.Sprint(r.Sandbox["note"]) + ")",
		"state dir: " + state,
		"terminal: TERM=" + fmt.Sprint(r.Terminal["TERM"]) + " stdout_terminal=" + fmt.Sprint(r.Terminal["stdout_terminal"]),
	}
	lines = append(lines, qualificationPlain(r.Qualification)...)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, cell(line)); err != nil {
			return err
		}
	}
	return nil
}
