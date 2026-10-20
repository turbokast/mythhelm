package processsupport

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestDocsMatchWindowsMatrix(t *testing.T) {
	t.Parallel()
	m, err := Parse(table)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("..", "..")
	docs := map[string]string{}
	for _, name := range []string{"docs/limitations.md", "README.md", "docs/decisions/0004-slice-process-model.md"} {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name))) //nolint:gosec // G304: names come only from the fixed documentation list above.
		if err != nil {
			t.Fatal(err)
		}
		docs[name] = string(b)
	}
	for _, issue := range windowsDocsIssues(docs, m) {
		t.Error(issue)
	} // I14 (v2 §2, §17.1).
}

func TestWindowsDocsRejectUnsupportedClaims(t *testing.T) {
	t.Parallel()
	const limits = "Windows lifecycle tests cover fake processes only. Untested: second interrupt and hosted console cases. Launch is refused inside a kill-on-close parent job."
	for _, tc := range []struct{ name, text, want string }{
		{"qualified limits", limits, ""},
		{"invented supported pair", limits + "\n\nWindows amd64 `invented-surface` is supported.", "absent matrix support"},
		{"Windows-first recovery", limits + "\n\nWindows recovery is supported.", "unqualified recovery"},
		{"paragraph recovery context", limits + "\n\nWindows lifecycle support is experimental. Recovery is supported.", "unqualified recovery"},
		{"negated recovery", limits + "\n\nWindows recovery is not supported.", ""},
		{"qualified live worker", limits + "\n\nWindows recovery works only with a live worker; worker death remains unresolved.", ""},
		{"multiple supported architectures", limits + "\n\nWindows amd64 and arm64 `invented-surface` are supported.", "absent matrix support"},
		{"unqualified recovery", limits + "\n\nSupervision and recovery work on Linux, macOS and Windows.", "unqualified recovery"},
		{"missing refusal", "Windows fake lifecycle tests only; second interrupt and hosted console cases are untested.", "parent-job launch refusal"},
		{"missing untested subset", "Windows launch is refused inside a kill-on-close parent job.", "untested subset"},
		{"historical unrelated recovery", limits + "\n\nUnix recovery works on Linux and macOS.", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := windowsDocsIssues(map[string]string{"docs/limitations.md": tc.text}, Matrix{})
			if tc.want == "" {
				if len(issues) != 0 {
					t.Fatalf("unexpected issues: %v", issues)
				}
				return
			}
			if !strings.Contains(strings.Join(issues, "\n"), tc.want) {
				t.Fatalf("issues=%v, want %q", issues, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name, status string
		valid        bool
	}{
		{"accepted Unix and superseded Windows", "Status: accepted for Unix; Windows superseded by [ADR 0017](0017-windows-process-ownership.md).", true},
		{"stale accepted status", "Status: accepted", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := windowsDocsIssues(map[string]string{"docs/decisions/0004-slice-process-model.md": tc.status}, Matrix{})
			if (len(issues) == 0) != tc.valid {
				t.Fatalf("issues=%v valid=%v", issues, tc.valid)
			}
		})
	}

	// A real parsed row permits only the capability and exact pair it establishes.
	m, err := Parse([]byte(header + fixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{fixture, strings.Replace(fixture, "supported | unknown", "unknown | supported", 1)} {
		matrix, err := Parse([]byte(header + row))
		if err != nil {
			t.Fatal(err)
		}
		claim := "Windows amd64 `scripted-child-process` process_tree_ownership and worker_detachment are supported."
		if issues := windowsDocsIssues(map[string]string{"README.md": claim}, matrix); len(issues) == 0 {
			t.Fatal("combined claim accepted with only one supported capability")
		}
	}

	for _, tc := range []struct {
		arch, surface string
		valid         bool
	}{
		{"amd64", "scripted-child-process", true}, {"arm64", "scripted-child-process", false}, {"amd64", "another-surface", false},
	} {
		text := fmt.Sprintf("Windows %s `%s` process_tree_ownership is supported.", tc.arch, tc.surface)
		issues := windowsDocsIssues(map[string]string{"README.md": text}, m)
		if (len(issues) == 0) != tc.valid {
			t.Errorf("%s: issues=%v valid=%v", text, issues, tc.valid)
		}
	}
}

// Inspect Windows paragraphs, and only the supersession status in the historical
// ADR: its old Decision text is preserved as the evidence for what was replaced.
func windowsDocsIssues(docs map[string]string, m Matrix) []string {
	var issues []string
	recovery := regexp.MustCompile(`(?i)\b(recovery|recover)\b`)
	positiveRecovery := regexp.MustCompile(`(?i)\b(works?|supported|on Linux)\b`)
	positiveSupport := regexp.MustCompile(`(?i)\b(is|are) supported\b`)
	qualifiedRecovery := regexp.MustCompile(`(?i)(not supported|unsupported|does not work|unresolved|quarantin|live worker|reconnect)`)
	archRef := regexp.MustCompile(`(?i)\b(amd64|arm64)\b`)
	surfaceRef := regexp.MustCompile("`([^`]+)`")
	for name, doc := range docs {
		var paragraphs []string
		for p := range strings.SplitSeq(doc, "\n\n") {
			p = strings.Join(strings.Fields(p), " ")
			if name == "docs/decisions/0004-slice-process-model.md" {
				if strings.Contains(p, "Status:") {
					paragraphs = append(paragraphs, p)
				}
			} else if strings.Contains(strings.ToLower(p), "windows") {
				paragraphs = append(paragraphs, p)
			}
		}
		windows := strings.Join(paragraphs, "\n")
		if name == "docs/decisions/0004-slice-process-model.md" && (!strings.Contains(windows, "accepted for Unix") || !strings.Contains(windows, "superseded") || !strings.Contains(windows, "0017-windows-process-ownership.md")) {
			issues = append(issues, name+": missing bounded Windows supersession")
		}
		for sentence := range strings.SplitSeq(windows, ".") {
			if name == "docs/limitations.md" && recovery.MatchString(sentence) && positiveRecovery.MatchString(sentence) && !qualifiedRecovery.MatchString(sentence) && (!strings.Contains(sentence, "on Linux and macOS") || strings.Contains(strings.ToLower(sentence), "windows")) {
				issues = append(issues, name+": unqualified recovery claim")
			}
			if !positiveSupport.MatchString(sentence) {
				continue
			}
			for _, arch := range archRef.FindAllString(sentence, -1) {
				for _, ref := range surfaceRef.FindAllStringSubmatch(sentence, -1) {
					if strings.EqualFold(ref[1], "process_tree_ownership") || strings.EqualFold(ref[1], "worker_detachment") {
						continue
					}
					support := m.Windows(strings.ToLower(arch), ref[1])
					tree := strings.Contains(strings.ToLower(sentence), "process_tree_ownership")
					detach := strings.Contains(strings.ToLower(sentence), "worker_detachment")
					valid := (!tree && detach || support.ProcessTreeOwnership == adapter.Supported) && (!detach && tree || support.WorkerDetachment == adapter.Supported)
					if !valid {
						issues = append(issues, name+": absent matrix support for "+arch+"/"+ref[1])
					}
				}
			}
		}
		if name != "docs/limitations.md" {
			continue
		}
		lower := strings.ToLower(windows)
		if !strings.Contains(lower, "untested") || !strings.Contains(lower, "second interrupt") || !strings.Contains(lower, "console") {
			issues = append(issues, name+": missing untested subset")
		}
		refusal := regexp.MustCompile(`(?i)launch[^.]*refused[^.]*kill-on-close parent job`)
		if !refusal.MatchString(windows) {
			issues = append(issues, name+": missing parent-job launch refusal")
		}
	}
	return issues
}
