package integration

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/workspace"
)

const largeFileThreshold = 1 << 20

func inspectFlags(ctx context.Context, workdir string, changed []ChangedPath) ([]Flag, error) {
	flags := make([]Flag, 0)
	for _, entry := range changed {
		name := entry.Path
		if name == "mythhelm.toml" {
			flags = append(flags, Flag{Name: "check_config_changed", Path: name})
		}
		if strings.HasSuffix(name, "_test.go") || strings.Contains("/"+name, "/testdata/") ||
			strings.Contains("/"+name, "/tests/") {
			flags = append(flags, Flag{Name: "test_files_changed", Path: name})
		}
		if entry.BlobID == "" {
			continue
		}
		if entry.Mode == "120000" {
			outside, err := security.ResolvesOutside(workdir, filepath.FromSlash(name))
			if outside || err != nil {
				flags = append(flags, Flag{Name: "symlink_escape", Path: name})
			}
			continue
		}
		sizeRaw, err := workspace.Git(ctx, workdir, false, "cat-file", "-s", entry.BlobID)
		if err != nil {
			return nil, fmt.Errorf("size of changed blob %s: %w", name, err)
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(sizeRaw)), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid blob size for %s: %w", name, err)
		}
		if size > largeFileThreshold {
			flags = append(flags, Flag{Name: "large_file", Path: name})
		}
		scan := newBlobScanner()
		if err := workspace.GitBlob(ctx, workdir, entry.BlobID, scan); err != nil {
			return nil, fmt.Errorf("scan changed blob %s: %w", name, err)
		}
		if scan.binary {
			flags = append(flags, Flag{Name: "binary", Path: name})
		}
		for _, pattern := range scan.patterns {
			flags = append(flags, Flag{Name: "secret_pattern", Path: name, Pattern: pattern})
		}
	}
	status, err := workspace.Git(ctx, workdir, false, "status", "--porcelain=v1", "--ignored", "--untracked-files=all", "-z")
	if err != nil {
		return nil, fmt.Errorf("count ignored outputs: %w", err)
	}
	count := 0
	for entry := range bytes.SplitSeq(status, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("!! ")) {
			count++
		}
	}
	if count > 0 {
		flags = append(flags, Flag{Name: "ignored_outputs", Count: count})
	}
	return flags, nil
}

// blobScanner checks every streamed chunk with an overlap, keeping memory
// bounded even for a blob much larger than Git's captured-output limit.
type blobScanner struct {
	tail     []byte
	binary   bool
	patterns []string
}

func newBlobScanner() *blobScanner { return &blobScanner{} }

func (s *blobScanner) Write(p []byte) (int, error) {
	n := len(p)
	const chunk = 1 << 20
	for len(p) > 0 {
		take := min(len(p), chunk)
		window := make([]byte, 0, len(s.tail)+take)
		window = append(window, s.tail...)
		window = append(window, p[:take]...)
		if bytes.IndexByte(window, 0) >= 0 {
			s.binary = true
		}
		for _, name := range security.SecretPatternNames(window) {
			if !slices.Contains(s.patterns, name) {
				s.patterns = append(s.patterns, name)
			}
		}
		const overlap = 4096
		if len(window) > overlap {
			s.tail = append(s.tail[:0], window[len(window)-overlap:]...)
		} else {
			s.tail = append(s.tail[:0], window...)
		}
		p = p[take:]
	}
	return n, nil
}
