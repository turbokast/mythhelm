package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SourceFingerprint hashes the state of the repository whose top level is
// repo, without running git: every working-tree entry (path, type,
// permissions and content, or target for a symlink), the index, HEAD,
// packed-refs, refs/** and config. It is equal before and after an operation
// exactly when that operation left the checkout byte-identical (I08, AC-3.4).
// It returns "sha256:<hex>".
func SourceFingerprint(repo string) (string, error) {
	gitDir, commonDir, err := gitDirs(repo)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if err := hashTree(h, "worktree", repo, true); err != nil {
		return "", err
	}
	for _, f := range []struct{ label, path string }{
		{"index", filepath.Join(gitDir, "index")},
		{"HEAD", filepath.Join(gitDir, "HEAD")},
		{"packed-refs", filepath.Join(commonDir, "packed-refs")},
		{"config", filepath.Join(commonDir, "config")},
	} {
		if err := hashFile(h, f.label, f.path); err != nil {
			return "", err
		}
	}
	if err := hashTree(h, "refs", filepath.Join(commonDir, "refs"), false); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// gitDirs finds the git directory of the working tree at repo and its common
// directory: they differ for a linked worktree, whose .git is a file.
func gitDirs(repo string) (gitDir, commonDir string, err error) {
	gitDir = filepath.Join(repo, ".git")
	fi, err := os.Stat(gitDir)
	if err != nil {
		return "", "", fmt.Errorf("fingerprint %s: %w", repo, err)
	}
	if !fi.IsDir() {
		data, err := os.ReadFile(gitDir) // #nosec G304 -- the repository's own .git file
		if err != nil {
			return "", "", err
		}
		target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
		if !ok {
			return "", "", fmt.Errorf("fingerprint %s: malformed .git file", repo)
		}
		gitDir = resolveFrom(repo, target)
	}
	commonDir = gitDir
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil { // #nosec G304 G703 -- inside the repository's git directory
		commonDir = resolveFrom(gitDir, strings.TrimSpace(string(data)))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	return gitDir, commonDir, nil
}

func resolveFrom(base, p string) string {
	p = filepath.FromSlash(p)
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// hashTree hashes every entry under root in lexical order, skipping the
// top-level .git when skipGit is set. A missing root hashes as absent.
func hashTree(h hash.Hash, label, root string, skipGit bool) error {
	if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
		put(h, "%s\x00absent\n", label)
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skipGit && rel == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		mode := fi.Mode()
		put(h, "%s\x00%s\x00%v\x00", label, rel, mode.Type()|mode.Perm())
		switch {
		case mode.Type() == fs.ModeSymlink:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			put(h, "%s\x00", target) // NUL cannot occur in a target or path
		case mode.IsRegular():
			return hashContent(h, p)
		default:
			put(h, "\n")
		}
		return nil
	})
}

func hashFile(h hash.Hash, label, path string) error {
	put(h, "%s\x00", label)
	err := hashContent(h, path)
	if errors.Is(err, fs.ErrNotExist) {
		put(h, "absent\n")
		return nil
	}
	return err
}

// hashContent writes the SHA-256 of the file at path, so each entry adds a
// fixed-length record whatever the file's size.
func hashContent(h hash.Hash, path string) error {
	f, err := os.Open(path) // #nosec G304 -- a file inside the repository being fingerprinted
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only: a close error loses nothing
	fh := sha256.New()
	if _, err := io.Copy(fh, f); err != nil {
		return err
	}
	put(h, "%x\n", fh.Sum(nil))
	return nil
}

// put writes one formatted field into the fingerprint; hash writes never fail.
func put(h hash.Hash, format string, args ...any) {
	h.Write(fmt.Appendf(nil, format, args...))
}
