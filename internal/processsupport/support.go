// Package processsupport derives Windows lifecycle capabilities from evidence.
// It depends only on the adapter contract, so adapters can consume it without
// importing the workers that launch them.
package processsupport

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// Support describes the two platform capabilities established for one pair.
type Support struct{ ProcessTreeOwnership, WorkerDetachment adapter.Tri }

type row struct {
	arch, surface string
	support       Support
	tests         []string
}

// Matrix is an immutable parsed evidence table. Its zero value qualifies no pair.
type Matrix struct{ rows []row }

//go:embed SUPPORT.md
var table []byte

var (
	testRef = regexp.MustCompile("^`(Test[A-Z][A-Za-z0-9_]*)`$")
	ciRef   = regexp.MustCompile("^`?(windows-latest|windows-11-arm)`? +https://github\\.com/turbokast/mythhelm/actions/runs/[1-9][0-9]*$")
)

const matrixHeader = "arch|surface|process_tree_ownership|worker_detachment|crash behaviour|tests|CI job"

// Parse validates the complete table before returning any support. Prose outside
// table lines is ignored; malformed, duplicate or unbound evidence rejects the
// entire table. Source-test existence and CI outcomes are checked at qualification
// time, rather than by inspecting source files in an installed application.
func Parse(table []byte) (Matrix, error) {
	if len(table) > 1<<20 {
		return Matrix{}, errors.New("process support table exceeds 1 MiB")
	}
	var m Matrix
	state := 0
	seen := map[[2]string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(table))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if !strings.HasSuffix(line, "|") {
			return Matrix{}, errors.New("unterminated process support row")
		}
		cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|"), "|")
		if len(cells) != 7 {
			return Matrix{}, errors.New("process support row must have seven columns")
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		switch state {
		case 0:
			if strings.Join(cells, "|") != matrixHeader {
				return Matrix{}, errors.New("missing process support header")
			}
			state = 1
		case 1:
			for _, cell := range cells {
				if len(cell) < 3 || strings.Trim(cell, "-") != "" {
					return Matrix{}, errors.New("invalid process support separator")
				}
			}
			state = 2
		default:
			if len(m.rows) >= 256 {
				return Matrix{}, errors.New("too many process support rows")
			}
			r, err := parseRow(cells)
			if err != nil {
				return Matrix{}, err
			}
			key := [2]string{r.arch, r.surface}
			if seen[key] {
				return Matrix{}, errors.New("duplicate process support pair")
			}
			seen[key] = true
			m.rows = append(m.rows, r)
		}
	}
	if err := scanner.Err(); err != nil {
		return Matrix{}, fmt.Errorf("read process support table: %w", err)
	}
	if state != 2 {
		return Matrix{}, errors.New("incomplete process support header")
	}
	return m, nil
}

func parseRow(c []string) (row, error) {
	if (c[0] != "amd64" && c[0] != "arm64") || c[1] == "" {
		return row{}, errors.New("invalid process support pair")
	}
	values := []adapter.Tri{adapter.Tri(c[2]), adapter.Tri(c[3])}
	for _, v := range values {
		if v != adapter.Supported && v != adapter.Unsupported && v != adapter.Unknown {
			return row{}, errors.New("invalid process support capability")
		}
	}
	if c[4] != "kill_on_close" && c[4] != "survives" {
		return row{}, errors.New("invalid process support crash behaviour")
	}
	var tests []string
	for name := range strings.SplitSeq(c[5], ",") {
		match := testRef.FindStringSubmatch(strings.TrimSpace(name))
		if match == nil {
			return row{}, errors.New("invalid process support test evidence")
		}
		tests = append(tests, match[1])
	}
	ci := ciRef.FindStringSubmatch(c[6])
	if ci == nil || (c[0] == "amd64" && ci[1] != "windows-latest") || (c[0] == "arm64" && ci[1] != "windows-11-arm") {
		return row{}, errors.New("process support requires architecture-matched Windows job and run URL")
	}
	return row{arch: c[0], surface: c[1], support: Support{values[0], values[1]}, tests: tests}, nil
}

// Windows returns the exact pair's capabilities; absent evidence stays Unknown.
func (m Matrix) Windows(goarch, surface string) Support {
	for _, r := range m.rows {
		if r.arch == goarch && r.surface == surface {
			return r.support
		}
	}
	return Support{adapter.Unknown, adapter.Unknown}
}

// Windows uses the embedded evidence table. Malformed evidence qualifies nothing
// (I09, I14, v2 §2 and §17.1), including any rows parsed before the error.
func Windows(goarch, surface string) Support {
	m, _ := Parse(table)
	return m.Windows(goarch, surface)
}
