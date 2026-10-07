---
name: test-driven-development
description: Red-green-refactor for Go — use when implementing any feature, bug fix or behaviour change, before writing implementation code
argument-hint: "[behaviour to implement]"
---

# Test-Driven Development

Write the test first. Watch it fail for the right reason. Write the least code that passes. If you did not see the test fail, you do not know that it tests anything.

## Input

`$ARGUMENTS`: optional. The behaviour or acceptance criterion to implement.

## Invocation contexts

- **Slash command**: guides the cycle for the named behaviour.
- **Model-invoked**: the same, before any implementation code is written.
- **Non-interactive**: the same. Exceptions to test-first (step 0) are never self-granted: without a stated exception in the dispatching prompt, write the test.

## Steps

0. **Scope.** Every feature, bug fix and behaviour change is test-first. The only exceptions (throwaway prototypes, generated code, pure configuration) are agreed with the requester beforehand. Code written before its test is deleted and rewritten from the test.

### 1. Red: write one failing test

One behaviour per test, named for the behaviour, using real code and faking only what crosses the process boundary.

```go
func TestAdmissionBlocksUnknownBilling(t *testing.T) { // I02
	t.Parallel()
	got := admission.Decide(admission.Input{Billing: admission.BillingUnknown})
	if got.Allowed {
		t.Fatalf("Decide(unknown billing).Allowed = true, want false")
	}
	if got.Code != admission.CodeBillingUnresolved {
		t.Errorf("Code = %q, want %q", got.Code, admission.CodeBillingUnresolved)
	}
}
```

For several inputs, use a table with a name per case:

```go
tests := []struct {
	name string
	in   string
	want []string
}{
	{name: "flag after positional", in: "apply run_1 --to-branch x", want: []string{"run_1"}},
	{name: "flag before positional", in: "apply --to-branch x run_1", want: []string{"run_1"}},
}
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		t.Parallel()
		// ...
	})
}
```

Every acceptance test covering a JSONL or envelope output must assert the `type` discriminator value (or the schema's equivalent routing field) of each emitted object, not just that the output parses or has the right shape. A test named `*JSONL*` / `*Envelope*` without a discriminator assertion is incomplete.

### 2. Verify red: watch it fail

```bash
go test -run '^TestAdmissionBlocksUnknownBilling$' ./internal/admission/
```

| Result | Valid red? | Action |
|---|---|---|
| Assertion failure (`got X, want Y`) | Yes | Go to green |
| Does not compile (undefined name) | No | Add a stub returning the wrong value (`return Decision{Allowed: true}`), re-run until the assertion fails |
| Panic, setup failure, timeout | No | Fix the test setup; these say nothing about the behaviour |
| Passes | No | It tests existing behaviour; fix the test |

Then apply `.claude/rules/red-first.md`: would the test also fail against a plausible wrong fix?

### 3. Green: the least code that passes

Write only what the test demands. No options, parameters or branches the tests do not exercise.

### 4. Verify green

```bash
go test -race ./internal/admission/
```

The new test passes, the package's other tests still pass, and there is no new vet output. If other tests fail, fix them now.

### 5. Refactor

With tests green: remove duplication, improve names, extract a helper (with `t.Helper()`). Add no behaviour. Re-run after each change.

### 6. Repeat per behaviour

Build the task one function at a time: test, red, green, run the package's tests, next. Never write all the tests and then all the code; you lose the signal of which change broke what. When the task's behaviours are done, run the full gates (`.claude/skills/quality-gates/SKILL.md`).

### Bug fixes

Reproduce the bug in a failing test first, then fix. The test stays as the regression test.

### Anti-patterns

| Anti-pattern | Instead |
|---|---|
| Asserting only that a fake was called | Assert the output or side effect |
| Test-only exported functions in production code | An unexported seam, or one guarded by `testing.Testing()` |
| A fake with a shape the real dependency never produces | Mirror the real shape; take it from a recorded or documented example |
| Mocking internal packages | Use the real package; fake only external processes, network and time |
| `time.Sleep` to wait | Wait on the channel, process exit or file; inject the clock |
| `err == nil` as the only assertion | Assert the value produced |

## Output

For each behaviour: the test name, the observed red (the failing line) and the observed green (the passing run). These go into the task's completion entry.
