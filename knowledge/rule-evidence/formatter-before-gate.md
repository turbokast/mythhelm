# Evidence: formatter-before-gate

Record for `.claude/rules/formatter-before-gate.md`.

## Hazard

Agents often run the check (`gofmt -l`, `go mod tidy -diff`, a lint) first, read its complaint, edit by hand, and run it again. Each round costs a gate cycle and a turn to learn what the formatter would have applied silently, and hand edits to formatting are frequently wrong in a new way. Newly written test tables and long assertion lines are the most common source.

## Mechanism

Running `gofmt -w` and `go mod tidy` first makes the check a confirmation rather than feedback. CI enforces the checks (`gofmt -l .` must be empty, `go mod tidy -diff` must be clean).

## Instances

None recorded in this repository yet.

## Loosening criteria

None expected; the rule costs nothing when followed.
