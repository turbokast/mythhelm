// Package themes holds the built-in TUI theme token files as embedded data.
//
// The embed directive lives here because go:embed patterns are relative to
// the source directory and forbid "..", so internal/tui/theme cannot embed
// mods/ directly. Parsing and validation stay in internal/tui/theme; this
// package exposes only the raw TOML bytes.
package themes

import "embed"

// Files carries the built-in theme token files (dark.toml, light.toml).
//
//go:embed *.toml
var Files embed.FS
