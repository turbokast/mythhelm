package security

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const maxSymlinkHops = 255

// ResolvesOutside reports whether path, with every symlink followed, lies
// outside root (itself resolved). A relative path is taken relative to root.
// Symlinks are followed even when their target does not exist, so a dangling
// link that points outside is still an escape, and ".." is applied after the
// preceding component is resolved, never lexically across a symlink.
//
// This is a pre-check, not a sandbox: the filesystem can change after it
// returns (§12.7).
func ResolvesOutside(root, path string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(absRoot); err != nil {
		return false, fmt.Errorf("resolve root: %w", err)
	}
	realRoot, err := resolvePath(absRoot)
	if err != nil {
		return false, fmt.Errorf("resolve root: %w", err)
	}
	if !filepath.IsAbs(path) {
		path = absRoot + string(filepath.Separator) + path
	}
	realPath, err := resolvePath(path)
	if err != nil {
		return false, fmt.Errorf("resolve %s: %w", path, err)
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		return true, nil // no relative path exists, e.g. another volume
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// resolvePath resolves the absolute path p component by component. A missing
// component (or one below a non-directory) is kept lexically and resolution
// continues, so a later ".." can climb back to a real symlink.
func resolvePath(p string) (string, error) {
	vol := filepath.VolumeName(p)
	resolved := vol + string(filepath.Separator)
	rest := splitPath(p[len(vol):])
	hops := 0
	for len(rest) > 0 {
		c := rest[0]
		rest = rest[1:]
		switch c {
		case "", ".":
			continue
		case "..":
			resolved = filepath.Dir(resolved)
			continue
		}
		next := filepath.Join(resolved, c)
		fi, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			resolved = next
			continue
		}
		if err != nil {
			return "", err
		}
		target, isLink, err := readLink(next, fi)
		if err != nil {
			return "", err
		}
		if !isLink {
			resolved = next
			continue
		}
		if hops++; hops > maxSymlinkHops {
			return "", fmt.Errorf("too many levels of symbolic links at %s", next)
		}
		switch tvol := filepath.VolumeName(target); {
		case filepath.IsAbs(target):
			resolved = tvol + string(filepath.Separator)
			target = target[len(tvol):]
		case target != "" && os.IsPathSeparator(target[0]): // rooted on the current volume (Windows)
			resolved = filepath.VolumeName(resolved) + string(filepath.Separator)
		}
		rest = append(splitPath(target), rest...)
	}
	return resolved, nil
}

// readLink returns the target of a symlink, or of a Windows junction or other
// link-like reparse point, which Lstat reports as irregular. A symlink whose
// target cannot be read is an error, so the check fails closed; an irregular
// file that is not a link (Readlink fails) is an ordinary component.
func readLink(path string, fi fs.FileInfo) (string, bool, error) {
	mode := fi.Mode()
	irregular := runtime.GOOS == "windows" && mode&fs.ModeIrregular != 0
	if mode&fs.ModeSymlink == 0 && !irregular {
		return "", false, nil
	}
	target, err := os.Readlink(path)
	switch {
	case err == nil:
		return target, true, nil
	case mode&fs.ModeSymlink == 0:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("read link %s: %w", path, err)
	}
}

func splitPath(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == filepath.Separator })
}
