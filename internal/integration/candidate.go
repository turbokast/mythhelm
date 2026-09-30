// Package integration freezes a stopped attempt's workspace into a reviewable
// commit without changing the workspace HEAD or the user's repository.
package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/turbokast/mythhelm/internal/workspace"
)

// CommitMeta is fixed at admission or attempt launch. The title is the first
// task heading, and the identity is the user's Git identity at admission.
type CommitMeta struct {
	RunID, AttemptID string
	Title            string
	Name, Email      string
	Partial          bool
}

// ChangedPath records the blob at the candidate revision. Deleted paths have
// an empty BlobID.
type ChangedPath struct {
	Path   string `json:"path"`
	BlobID string `json:"blob_id,omitempty"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

// Flag records a validation concern without storing matched secret values.
type Flag struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// Candidate is the frozen tree and its review metadata.
type Candidate struct {
	BaseRev, Commit, Tree, PatchSHA256 string
	Changed                            []ChangedPath
	Flags                              []Flag
	Partial                            bool
}

var refPart = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Freeze captures the whole working tree using a private index. It parents
// the candidate on baseRev even if the native moved HEAD. No user index, HEAD,
// or source repository ref is changed.
func Freeze(ctx context.Context, workdir, baseRev string, meta CommitMeta) (Candidate, error) {
	var c Candidate
	if !refPart.MatchString(meta.AttemptID) || !refPart.MatchString(meta.RunID) ||
		meta.Name == "" || meta.Email == "" || strings.ContainsAny(meta.Name+meta.Email+meta.Title, "\r\n") ||
		strings.Contains(strings.ToLower(meta.Title), "signed-off-by:") {
		return c, errors.New("invalid candidate metadata")
	}
	base, err := workspace.Git(ctx, workdir, false, "rev-parse", "--verify", "--end-of-options", baseRev+"^{commit}")
	if err != nil {
		return c, fmt.Errorf("resolve admitted base: %w", err)
	}
	c.BaseRev = strings.TrimSpace(string(base))
	c.Partial = meta.Partial

	f, err := os.CreateTemp("", "mythhelm-freeze-index-*")
	if err != nil {
		return c, fmt.Errorf("create private index: %w", err)
	}
	index := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(index)
		return c, err
	}
	// Git creates a fresh index at this path; an empty pre-created index is
	// invalid. The matching .lock is removed by Git on command completion.
	if err := os.Remove(index); err != nil {
		return c, err
	}
	defer func() { _ = os.Remove(index) }()
	if _, err := workspace.GitWithIndex(ctx, workdir, index, "read-tree", c.BaseRev); err != nil {
		return c, fmt.Errorf("initialise private index: %w", err)
	}
	if _, err := workspace.GitWithIndex(ctx, workdir, index, "add", "-A", "--", "."); err != nil {
		return c, fmt.Errorf("stage workspace in private index: %w", err)
	}
	tree, err := workspace.GitWithIndex(ctx, workdir, index, "write-tree")
	if err != nil {
		return c, fmt.Errorf("write candidate tree: %w", err)
	}
	c.Tree = strings.TrimSpace(string(tree))
	message := strings.TrimSpace(strings.TrimPrefix(meta.Title, "#")) + "\n\nMYTHHELM-Run: " + meta.RunID
	commit, err := workspace.GitWithIndex(ctx, workdir, index,
		"-c", "user.name="+meta.Name, "-c", "user.email="+meta.Email,
		"commit-tree", c.Tree, "-p", c.BaseRev, "-m", message)
	if err != nil {
		return c, fmt.Errorf("commit candidate tree: %w", err)
	}
	c.Commit = strings.TrimSpace(string(commit))
	changed, err := workspace.Git(ctx, workdir, false, "diff-tree", "-r", "--no-renames", "--raw", "-z", c.BaseRev, c.Commit)
	if err != nil {
		return c, fmt.Errorf("list candidate changes: %w", err)
	}
	if c.Changed, err = parseChanged(changed); err != nil {
		return c, err
	}
	h := sha256.New()
	if err := workspace.GitPatchSHA256(ctx, workdir, c.BaseRev, c.Commit, h); err != nil {
		return c, fmt.Errorf("hash candidate patch: %w", err)
	}
	c.PatchSHA256 = hex.EncodeToString(h.Sum(nil))
	if c.Flags, err = inspectFlags(ctx, workdir, c.Changed); err != nil {
		return c, err
	}
	ref := "refs/mythhelm/candidates/" + meta.AttemptID
	if _, err := workspace.Git(ctx, workdir, false, "update-ref", ref, c.Commit, strings.Repeat("0", len(c.Commit))); err != nil {
		return c, fmt.Errorf("publish candidate ref: %w", err)
	}
	return c, nil
}

func parseChanged(raw []byte) ([]ChangedPath, error) {
	parts := bytes.Split(raw, []byte{0})
	changed := make([]ChangedPath, 0, len(parts)/2)
	for i := 0; i+1 < len(parts) && len(parts[i]) > 0; i += 2 {
		meta := strings.Fields(strings.TrimPrefix(string(parts[i]), ":"))
		if len(meta) != 5 || len(parts[i+1]) == 0 {
			return nil, fmt.Errorf("malformed diff-tree entry")
		}
		id := meta[3]
		if meta[4] == "D" {
			id = ""
		}
		changed = append(changed, ChangedPath{Path: string(parts[i+1]), BlobID: id, Status: meta[4], Mode: meta[1]})
	}
	return changed, nil
}

// RefPath gives the candidate's private ref without accepting arbitrary Git
// ref syntax from an external caller.
func RefPath(attemptID string) (string, error) {
	if !refPart.MatchString(attemptID) {
		return "", errors.New("invalid attempt ID")
	}
	return filepath.ToSlash("refs/mythhelm/candidates/" + attemptID), nil
}
