package qualify

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/turbokast/mythhelm/internal/journal"
)

// EnsureSeeded inserts the SeedV1 records missing from the table and
// verifies every seed key reads back. A complete table is untouched; a
// partial seed is completed. Concurrent first runs are safe: the single
// INSERT OR IGNORE statement serialises same-key inserts. Every failure
// returns the journal error wrapped.
func EnsureSeeded(ctx context.Context, j *journal.Journal) error {
	seed := SeedV1()
	for _, rec := range seed {
		raw, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("qualify: encoding seed record for %s: %w", rec.Key.Harness, err)
		}
		if _, err := j.InsertQualificationRecordIfAbsent(ctx, KeyHash(rec.Key), 1, rec.Digest, string(raw)); err != nil {
			return fmt.Errorf("qualify: seeding qualification records: %w", err)
		}
	}
	// Verify the seven current rows read back: fewer than seven means a
	// seed key is missing, and the journal error says so.
	for _, rec := range seed {
		if _, _, err := j.CurrentQualificationRecord(ctx, KeyHash(rec.Key)); err != nil {
			return fmt.Errorf("qualify: verifying qualification seed: %w", err)
		}
	}
	return nil
}

// seedHarness is one v2 §7.2 candidate: its id, honest starting progress
// and the next test that could advance it with the authority that test
// needs (AC-4.3).
type seedHarness struct {
	harness  string
	progress Progress
	nextTest string
}

// seedHarnesses follows the v2 §7.2 table order. Only claude-code has a
// concrete next evidence step, so only it starts blocked; the rest are
// planned with the first qualification question each must answer.
var seedHarnesses = []seedHarness{
	{
		harness:  "claude-code",
		progress: ProgressBlocked,
		nextTest: "Prove included-only entitlement across the AT-03 routes with authorised tests; authority: maintainer allowance (MH-12)",
	},
	{
		harness:  "codex",
		progress: ProgressPlanned,
		nextTest: "Qualify native-managed sign-in, approvals and effort metadata; authority: maintainer live-test grant",
	},
	{
		harness:  "opencode",
		progress: ProgressPlanned,
		nextTest: "Qualify provider-specific entitlement across all routes and plugins; authority: maintainer live-test grant",
	},
	{
		harness:  "muse",
		progress: ProgressPlanned,
		nextTest: "Qualify subscription credential precedence and observer/child resource use; authority: maintainer live-test grant",
	},
	{
		harness:  "kimi",
		progress: ProgressPlanned,
		nextTest: "Qualify the membership route with Extra Usage disabled; authority: maintainer live-test grant",
	},
	{
		harness:  "cursor",
		progress: ProgressPlanned,
		nextTest: "Establish plan/model inclusion and paid-continuation evidence; authority: maintainer live-test grant",
	},
	{
		harness:  "antigravity",
		progress: ProgressPlanned,
		nextTest: "Prove the effective no-overage setting for the exact CLI/account/model; authority: maintainer live-test grant",
	},
}

// SeedV1 returns the seven v2 §7.2 harness records for the local platform,
// each blocked or planned with the next test that could advance it. Every
// unestablished field stays "unknown" (I09): the seed claims no digest,
// snapshot or class before evidence exists. It records nothing;
// EnsureSeeded writes the missing rows.
func SeedV1() []Record {
	out := make([]Record, 0, len(seedHarnesses))
	for _, h := range seedHarnesses {
		rec := Record{
			SchemaVersion: 2,
			Revision:      1,
			Key: Key{
				Harness:          h.harness,
				Surface:          unknownString,
				ExecutableDigest: unknownString,
				AdapterProtocol:  unknownString,
				OS:               runtime.GOOS,
				Arch:             runtime.GOARCH,
				ProviderEndpoint: unknownString,
				ModelSnapshot:    unknownString,
				EffortSettings:   unknownString,
				AuthCategory:     unknownString,
				ConfigDigest:     unknownString,
				TrustProfile:     unknownString,
				WorkspaceClass:   unknownString,
				EntitlementClass: unknownString,
			},
			Progress:     h.progress,
			Fidelity:     unknownColumn(),
			Entitlement:  unknownColumn(),
			Lifecycle:    unknownColumn(),
			Capabilities: map[string]Capability{},
			Quota:        unknownDatum(),
			NextTest:     h.nextTest,
		}
		out = append(out, mustDigest(rec))
	}
	return out
}

// mustDigest sets rec's content digest, panicking on the unreachable
// failure: the seed sets every scale value explicitly.
func mustDigest(rec Record) Record {
	digest, err := CanonicalDigest(rec)
	if err != nil {
		panic("qualify: seed record out of scale: " + err.Error())
	}
	rec.Digest = digest
	return rec
}

// unknownColumn is a column with no evidence either way (AC-1.2, I09).
func unknownColumn() Column {
	return Column{Verdict: Unknown, Evidence: []Evidence{}}
}

// unknownDatum is a quantity with nothing established about it (AC-5.2).
func unknownDatum() Datum {
	return Datum{
		Quantity: unknownString,
		Label:    DatumUnknown,
		Unit:     unknownString,
		Scope:    unknownString,
		Source:   unknownString,
	}
}
