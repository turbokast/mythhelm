// Package security holds the slice's defence-in-depth primitives: log
// redaction, terminal sanitising, the child-environment allowlist and the
// symlink-escape path check (design §6.3, §11; master spec §12.5, §12.7).
package security

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// secretPatterns are the §12.7 shapes. Each replacement keeps any captured
// label (the "Bearer " or "api_key=" prefix) and replaces only the secret. A
// quoted labelled value is replaced up to its closing quote, spaces included.
var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
	repl string
}{
	{"anthropic_key", regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]+`), redacted},
	{"github_fine_grained_token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]+`), redacted},
	{"github_token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]+`), redacted},
	{"aws_access_key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), redacted},
	{"bearer_token", regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]+`), "${1}" + redacted},
	{"quoted_assignment_double", regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)["']?\s*[:=]\s*")(?:[^"\\]|\\.)*`), "${1}" + redacted},
	{"quoted_assignment_single", regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)["']?\s*[:=]\s*')[^']*`), "${1}" + redacted},
	{"assignment", regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)["']?\s*[:=]\s*)[^\s"']+`), "${1}" + redacted},
}

// SecretPatternNames returns names of known secret shapes present in content.
// It never returns the matched values, so callers can record validation flags.
func SecretPatternNames(content []byte) []string {
	var names []string
	for _, p := range secretPatterns {
		if p.re.Match(content) {
			names = append(names, p.name)
		}
	}
	return names
}

// Redact replaces known secret shapes in s. It is defence in depth, not proof
// that the result is secret-free (§12.5).
func Redact(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// credentialSuffixes name credential-bearing attributes once the key is
// upper-cased and stripped of '_' and '-'. Suffix matching drops "apiKey" and
// "ANTHROPIC_AUTH_TOKEN" but keeps "apiKeySource" and "input_tokens".
var credentialSuffixes = []string{"APIKEY", "ACCESSKEY", "PRIVATEKEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "CREDENTIALS"}

func credentialName(key string) bool {
	k := strings.ToUpper(strings.NewReplacer("_", "", "-", "").Replace(key))
	for _, s := range credentialSuffixes {
		if strings.HasSuffix(k, s) {
			return true
		}
	}
	return false
}

// RedactingHandler wraps a slog.Handler. It redacts the message and every
// string-like attribute value, and drops attributes named like a credential.
// Non-string values other than groups and scalars are rendered with %v and
// redacted, so a struct or map never reaches the wrapped handler unredacted.
type RedactingHandler struct {
	next slog.Handler
}

// NewRedactingHandler returns a handler that redacts before delegating to h.
func NewRedactingHandler(h slog.Handler) slog.Handler {
	return &RedactingHandler{next: h}
}

// Enabled reports whether the wrapped handler handles records at level l.
func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

// Handle redacts r and passes it to the wrapped handler.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, Redact(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a, ok := redactAttr(a); ok {
			out.AddAttrs(a)
		}
		return true
	})
	return h.next.Handle(ctx, out)
}

// WithAttrs redacts attrs before they are bound to the wrapped handler.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RedactingHandler{next: h.next.WithAttrs(redactAttrs(attrs))}
}

// WithGroup returns a redacting handler over h's wrapped handler with the group.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func redactAttrs(attrs []slog.Attr) []slog.Attr {
	kept := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if a, ok := redactAttr(a); ok {
			kept = append(kept, a)
		}
	}
	return kept
}

func redactAttr(a slog.Attr) (slog.Attr, bool) {
	if credentialName(a.Key) {
		return slog.Attr{}, false
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		v = slog.StringValue(Redact(v.String()))
	case slog.KindGroup:
		v = slog.GroupValue(redactAttrs(v.Group())...)
	case slog.KindAny:
		v = slog.StringValue(Redact(fmt.Sprint(v.Any())))
	}
	return slog.Attr{Key: a.Key, Value: v}, true
}
