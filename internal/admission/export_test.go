package admission

import "context"

// DecideWithHarness runs the production pipeline over the default table plus
// one extra row, so a test can prove dispatch goes through the table.
func DecideWithHarness(ctx context.Context, req Request, name string,
	decide func(context.Context, Request, Decision) (Decision, error)) (Decision, error) {
	table := append(defaultDeciders(), harnessDecider{name: name, adapterID: "builtin/" + name, decide: decide})
	return decideWith(ctx, table, req)
}

// Validate is the request validation over the default table.
func Validate(req *Request) error { return validateWith(defaultDeciders(), req) }
