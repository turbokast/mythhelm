// Package tui implements the focused single-mission terminal view.
//
// This file exists only to keep the TUI dependency pins tidy-clean: the
// blank imports below hold the exact module versions design §16 D11 and D12
// name as direct requirements, so go mod tidy keeps them instead of dropping
// the unimported requires. Later tasks replace this pin file with real
// consumers of these modules.
package tui

import (
	_ "github.com/charmbracelet/bubbles" // Pin: hold as a direct requirement per the package comment.
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/lipgloss"
	_ "github.com/mattn/go-isatty"
)
