package fake

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter/ndjson"
)

// AgentCommand is the hidden mythhelm command that runs the fake agent.
const AgentCommand = "__fake-agent"

//go:embed scenarios/*.json
var scenarioFS embed.FS

// Scenarios returns the names of the embedded scenarios, sorted.
func Scenarios() []string {
	entries, err := fs.ReadDir(scenarioFS, "scenarios")
	if err != nil {
		panic(err) // the directory is embedded at build time
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	return names
}

type scenario struct {
	Steps []step `json:"steps"`
}

// step is one scripted action. Op selects which other fields apply.
type step struct {
	Op string `json:"op"`

	// emit: exactly one of these is the frame written, followed by '\n'.
	Frame     json.RawMessage `json:"frame,omitempty"`      // compacted JSON
	Raw       string          `json:"raw,omitempty"`        // written verbatim
	RawBase64 []byte          `json:"raw_base64,omitempty"` // bytes JSON cannot carry, such as invalid UTF-8
	Synth     string          `json:"synth,omitempty"`      // "oversized" or "deep", sized from the ndjson defaults

	// write: a file relative to the workdir.
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`

	// sleep, spawn_escapee: how long to sleep, or how long the escapee lingers.
	MS int `json:"ms,omitempty"`

	// ignore_signals: "INT" and/or "TERM".
	Signals []string `json:"signals,omitempty"`

	// exit: the exit code.
	Code int `json:"code,omitempty"`
}

func loadScenario(name string) (scenario, error) {
	if !slices.Contains(Scenarios(), name) {
		return scenario{}, fmt.Errorf("%w %q (have %s)", ErrUnknownScenario, name, strings.Join(Scenarios(), ", "))
	}
	src, err := scenarioFS.ReadFile(path.Join("scenarios", name+".json"))
	if err != nil {
		return scenario{}, err
	}
	sc, err := parseScenario(src)
	if err != nil {
		return scenario{}, fmt.Errorf("scenario %s: %w", name, err)
	}
	return sc, nil
}

func parseScenario(src []byte) (scenario, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	var sc scenario
	if err := dec.Decode(&sc); err != nil {
		return scenario{}, err
	}
	for i, st := range sc.Steps {
		if err := st.validate(); err != nil {
			return scenario{}, fmt.Errorf("step %d (%s): %w", i, st.Op, err)
		}
	}
	return sc, nil
}

func (st step) validate() error {
	switch st.Op {
	case "emit":
		n := 0
		for _, set := range []bool{len(st.Frame) > 0, st.Raw != "", len(st.RawBase64) > 0, st.Synth != ""} {
			if set {
				n++
			}
		}
		if n != 1 {
			return errors.New("needs exactly one of frame, raw, raw_base64 or synth")
		}
		if st.Synth != "" && st.Synth != "oversized" && st.Synth != "deep" {
			return fmt.Errorf("unknown synth %q", st.Synth)
		}
	case "write", "append":
		if !filepath.IsLocal(filepath.FromSlash(st.Path)) {
			return fmt.Errorf("path %q is not local to the workdir", st.Path)
		}
	case "sleep", "spawn_escapee":
		if st.MS <= 0 {
			return errors.New("ms must be positive")
		}
	case "ignore_signals":
		if len(st.Signals) == 0 {
			return errors.New("signals is empty")
		}
		for _, s := range st.Signals {
			if s != "INT" && s != "TERM" {
				return fmt.Errorf("signal %q is not INT or TERM", s)
			}
		}
	case "exit":
	default:
		return errors.New("unknown op")
	}
	return nil
}

// AgentMain runs the fake agent and returns its exit code. It drains the
// prompt from stdin, then plays the scenario's steps. Unless a scenario
// ignores them, SIGINT exits 130 and SIGTERM exits 143, as a conventional
// CLI would.
func AgentMain(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet(AgentCommand, flag.ContinueOnError)
	fl.SetOutput(stderr)
	name := fl.String("scenario", "", "embedded scenario to play")
	workdir := fl.String("workdir", "", "directory the scenario writes into")
	linger := fl.Duration("linger", 0, "sleep this long, then exit (the escapee's body)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *linger > 0 {
		time.Sleep(*linger)
		return 0
	}
	report := func(code int, err error) int {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", AgentCommand, err)
		return code
	}
	fail := func(err error) int { return report(1, err) }
	sc, err := loadScenario(*name)
	if err != nil {
		return report(2, err)
	}
	if *workdir == "" {
		return report(2, errors.New("--workdir is required"))
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	if _, err := io.Copy(io.Discard, stdin); err != nil {
		return fail(fmt.Errorf("reading the prompt: %w", err))
	}
	for _, st := range sc.Steps {
		select {
		case sig := <-sigs:
			return signalExit(sig)
		default:
		}
		switch st.Op {
		case "emit":
			if _, err := stdout.Write(append(st.frame(), '\n')); err != nil {
				return fail(err)
			}
		case "write", "append":
			p := filepath.Join(*workdir, filepath.FromSlash(st.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				return fail(err)
			}
			if st.Op == "append" {
				f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) // #nosec G304 -- validated local path in an embedded scenario
				if err != nil {
					return fail(err)
				}
				_, err = io.WriteString(f, st.Content)
				err = errors.Join(err, f.Close())
				if err != nil {
					return fail(err)
				}
			} else if err := os.WriteFile(p, []byte(st.Content), 0o600); err != nil {
				return fail(err)
			}
		case "sleep":
			select {
			case sig := <-sigs:
				return signalExit(sig)
			case <-time.After(time.Duration(st.MS) * time.Millisecond):
			}
		case "ignore_signals":
			for _, s := range st.Signals {
				signal.Ignore(map[string]os.Signal{"INT": os.Interrupt, "TERM": syscall.SIGTERM}[s])
			}
		case "spawn_escapee":
			if err := spawnEscapee(time.Duration(st.MS) * time.Millisecond); err != nil {
				return fail(err)
			}
		case "exit":
			return st.Code
		}
	}
	return 0
}

func signalExit(sig os.Signal) int {
	if sig == syscall.SIGTERM {
		return 143
	}
	return 130
}

// frame returns the bytes an emit step writes, without the newline.
func (st step) frame() []byte {
	switch {
	case len(st.Frame) > 0:
		var b bytes.Buffer
		if err := json.Compact(&b, st.Frame); err != nil {
			return st.Frame // unreachable: decoding a RawMessage guarantees valid JSON
		}
		return b.Bytes()
	case st.Raw != "":
		return []byte(st.Raw)
	case len(st.RawBase64) > 0:
		return st.RawBase64
	case st.Synth == "oversized":
		return []byte(`{"type":"` + frameProgress + `","pad":"` + strings.Repeat("x", ndjson.DefaultMaxFrame) + `"}`)
	default: // deep
		n := ndjson.DefaultMaxDepth + 1
		return []byte(`{"type":"` + frameProgress + `","nest":` + strings.Repeat("[", n) + strings.Repeat("]", n) + `}`)
	}
}
