package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"

	"github.com/turbokast/mythhelm/internal/supervisor"
)

// recoveryCommand formats a command; it never executes a shell. The explicit
// state assignment overrides a caller's unrelated MYTHHELM_HOME.
func recoveryCommand(stateDir, runID, goos string) (string, error) {
	return stateCommand(stateDir, runID, goos, "recover")
}

func stateCommand(stateDir, runID, goos, action string) (string, error) {
	if !filepath.IsAbs(stateDir) || strings.ContainsFunc(stateDir, unicode.IsControl) || !regexp.MustCompile(`^run_[A-Za-z0-9_]{1,128}$`).MatchString(runID) {
		return "", errors.New("invalid state directory or run ID")
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	switch goos {
	case "windows":
		quote = func(s string) string {
			return "'" + strings.NewReplacer("'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛").Replace(s) + "'"
		}
		return fmt.Sprintf("$env:MYTHHELM_HOME = %s; mythhelm %s %s", quote(stateDir), action, quote(runID)), nil
	case "linux", "darwin":
		return fmt.Sprintf("env %s mythhelm %s %s", quote("MYTHHELM_HOME="+stateDir), action, quote(runID)), nil
	default:
		return "", fmt.Errorf("no tested recovery shell for platform %q", goos)
	}
}

func recoveryHint(stateDir, runID, goos string) string {
	command, err := recoveryCommand(stateDir, runID, goos)
	if err != nil {
		return fmt.Sprintf("recovery command unavailable: %s; state %s; run %s; open a supported POSIX sh/bash/zsh or PowerShell shell", cell(err.Error()), cell(stateDir), cell(runID))
	}
	return shellLabel(goos) + ": " + command
}
func shellLabel(goos string) string {
	if goos == "windows" {
		return "PowerShell"
	}
	return "POSIX sh/bash/zsh"
}

// demoCleanupSafe is evaluated only after runLiveTUI has joined its owner and
// actions. A client error or incomplete attempt evidence cannot authorize deletion.
func demoCleanupSafe(out supervisor.Outcome, err error) bool {
	if err != nil || out.Detached || out.DetachedRunning || out.RunID == "" || out.AttemptID == "" {
		return false
	}
	switch out.State {
	case supervisor.RunReadyForReview, supervisor.RunCompleted, supervisor.RunCancelled, supervisor.RunBlocked, supervisor.RunFailed:
	default:
		return false
	}
	switch out.AttemptState {
	case supervisor.AttemptSucceededNative, supervisor.AttemptFailedNative, supervisor.AttemptStopped:
		return true
	default:
		return false
	}
}

func demoRetentionNotice(w io.Writer, repo, state, runID string) error {
	_, err := fmt.Fprintf(w, "demo repository retained: %s\ndemo state retained: %s\n%s\nFor other shells (including fish or cmd), open the named supported shell. Retain both demo directories and their evidence until recovery proves a terminal outcome and no live writer. Recovery preserves evidence for inspection/export. Only then remove these two demo directories explicitly.\n", cell(repo), cell(state), tuiRecoveryHint(state, runID))
	return err
}

func tuiReviewHint(stateDir, runID string) string {
	command, err := stateCommand(stateDir, runID, runtime.GOOS, "review")
	if err != nil {
		return "review command unavailable: " + cell(err.Error())
	}
	return shellLabel(runtime.GOOS) + ": " + command
}
