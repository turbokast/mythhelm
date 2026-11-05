package admission

import (
	"errors"
	"testing"
)

func TestServiceEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	for _, req := range []Request{{Adapter: AdapterClaudeCode, Env: []string{"PATH=caller-sentinel", "ANTHROPIC_API_KEY=caller-sentinel"}}, {Adapter: AdapterFake, Confirm: func(string) (bool, error) { return true, nil }}} {
		d, ch, err := PrepareService(t.Context(), req)
		var blocked *BlockedError
		if !errors.As(err, &blocked) || !blocked.Capability || len(ch) != 0 || d.RunID != "" {
			t.Fatalf("untrusted service context produced %q, %v, %v", d.RunID, ch, err)
		}
	}
}
