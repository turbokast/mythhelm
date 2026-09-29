package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrGitTooOld means the installed git is older than 2.30 (exit 7).
var ErrGitTooOld = errors.New("git 2.30 or newer is required")

// Unsupported repository features, as named in PreflightResult.Unsupported.
const (
	FeatureShallow    = "shallow"
	FeatureSubmodules = "submodules"
	FeatureLFS        = "lfs"
	FeatureSparse     = "sparse-checkout"
)

// PreflightResult describes the user's repository as found before admission.
type PreflightResult struct {
	Top         string   // working-tree top level
	Branch      string   // current branch short name; empty when HEAD is detached
	HeadRev     string   // full object ID of the HEAD commit
	Dirty       bool     // tracked modifications or untracked, non-ignored files
	Unsupported []string // repository features a snapshot cannot carry (AC-3.5)
}

// Preflight inspects the repository containing repo without writing to it:
// every command runs with GIT_OPTIONAL_LOCKS=0 (I08). Features are checked on
// the HEAD commit, which is what the snapshot copies.
func Preflight(ctx context.Context, repo string) (PreflightResult, error) {
	var p PreflightResult
	out, err := Git(ctx, repo, true, "version")
	if err != nil {
		return p, err
	}
	if err := requireGitVersion(string(out)); err != nil {
		return p, err
	}
	if out, err = Git(ctx, repo, true, "rev-parse", "--show-toplevel"); err != nil {
		return p, fmt.Errorf("not a git working tree: %w", err)
	}
	p.Top = filepath.FromSlash(strings.TrimSpace(string(out)))
	if p.HeadRev, err = gitLine(ctx, p.Top, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return p, fmt.Errorf("resolve HEAD: %w", err)
	}
	if p.Branch, err = currentBranch(ctx, p.Top); err != nil {
		return p, err
	}
	if p.Unsupported, err = unsupportedFeatures(ctx, p.Top); err != nil {
		return p, err
	}
	out, err = Git(ctx, p.Top, true, "status", "--porcelain=v2", "--untracked-files=normal", "-z")
	if err != nil {
		return p, err
	}
	p.Dirty = len(out) > 0
	return p, nil
}

func gitLine(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := Git(ctx, dir, true, args...)
	return strings.TrimSpace(string(out)), err
}

func currentBranch(ctx context.Context, top string) (string, error) {
	branch, err := gitLine(ctx, top, "symbolic-ref", "--quiet", "--short", "HEAD")
	var gitErr *GitError
	if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
		return "", nil // detached HEAD
	}
	return branch, err
}

func unsupportedFeatures(ctx context.Context, top string) ([]string, error) {
	var found []string
	shallow, err := gitLine(ctx, top, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return nil, err
	}
	if shallow == "true" {
		found = append(found, FeatureShallow)
	}
	tree, err := Git(ctx, top, true, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	if err != nil {
		return nil, err
	}
	submodules, lfs, err := scanTree(ctx, top, tree)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(top, ".gitmodules")); err == nil {
		submodules = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("check .gitmodules: %w", err)
	}
	if submodules {
		found = append(found, FeatureSubmodules)
	}
	if lfs {
		found = append(found, FeatureLFS)
	}
	sparse, err := gitLine(ctx, top, "config", "--bool", "--get", "core.sparseCheckout")
	var gitErr *GitError
	if err != nil && (!errors.As(err, &gitErr) || gitErr.ExitCode != 1) { // exit 1: unset
		return nil, err
	}
	if sparse == "true" {
		found = append(found, FeatureSparse)
	}
	return found, nil
}

// scanTree reads `ls-tree -r -z` output and reports gitlinks (submodules)
// and any committed .gitattributes that routes paths through the LFS filter.
func scanTree(ctx context.Context, top string, tree []byte) (submodules, lfs bool, err error) {
	for entry := range bytes.SplitSeq(tree, []byte{0}) {
		meta, name, ok := strings.Cut(string(entry), "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta) // mode type object
		if len(fields) != 3 {
			return false, false, fmt.Errorf("unexpected ls-tree entry %q", entry)
		}
		switch {
		case fields[1] == "commit":
			submodules = true
		case !lfs && path.Base(name) == ".gitattributes" && fields[1] == "blob":
			blob, err := Git(ctx, top, true, "cat-file", "blob", fields[2])
			if err != nil {
				return false, false, err
			}
			lfs = lfsFilter.Match(blob)
		}
	}
	return submodules, lfs, nil
}

var lfsFilter = regexp.MustCompile(`(?m)^[^#\n]*\sfilter=lfs(\s|$)`)

var gitVersionRE = regexp.MustCompile(`^git version (\d+)\.(\d+)`)

func requireGitVersion(out string) error {
	m := gitVersionRE.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return fmt.Errorf("unrecognised git version output %q", strings.TrimSpace(out))
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < 2 || major == 2 && minor < 30 {
		return fmt.Errorf("%w: found %s", ErrGitTooOld, strings.TrimPrefix(m[0], "git version "))
	}
	return nil
}
