package supervisor

import "slices"

// noReasonRecorded is what a receipt says for an unknown leaf the table does
// not explain: a receipt is built after its run ended, so it never fails on a
// missing label, and the tests fail when any fixture reaches this text.
const noReasonRecorded = "no reason recorded"

// Reasons shared by several leaves, and the ones a run's own history changes.
const (
	reasonNotAdmitted    = "the run was not admitted"
	reasonNoVerification = "no verification was recorded"
	reasonBeforeBinding  = "recorded before acceptance binding existed"
	reasonNoChecks       = "no checks were admitted for this run (waived, or refused under inspect)"
	reasonPinUnreadable  = "acceptance pin unreadable"
	reasonNoEnvelope     = "no run envelope was recorded"
	reasonNoAttempt      = "no attempt started"
	reasonNoCandidate    = "no candidate was frozen"
	reasonNoNativeResult = "the native reported no result"
	reasonUnenforced     = "the admitted boundary does not enforce this dimension"
)

// unknownReasons returns the reason for every leaf of the receipt that can
// render unknown, keyed by its normalised path: array indices are `[]`. It is
// derived from the producers in buildReceipt, so a new unknown leaf needs an
// entry here before it ships. The map is fresh on each call because a receipt
// replaces the entries its own run explains differently.
func unknownReasons() map[string]string {
	return map[string]string{
		"requested_outcome.title":                       "the admitted task file is not in the run directory",
		"admitted_snapshot.branch":                      "no branch was recorded for the admitted snapshot",
		"admitted_snapshot.base_rev":                    "no base revision was recorded for this run",
		"admitted_snapshot.dirty_at_admission":          reasonNotAdmitted,
		"execution_bundle.harness":                      reasonNotAdmitted,
		"execution_bundle.surface":                      reasonNotAdmitted,
		"execution_bundle.native_version":               reasonNotAdmitted,
		"execution_bundle.native_sha256":                reasonNotAdmitted,
		"execution_bundle.compatibility":                reasonNotAdmitted,
		"execution_bundle.model":                        "the native session reported no model",
		"execution_bundle.permission_mode":              "the native session reported no permission mode",
		"execution_bundle.allowed_tools":                "no allowed tools were admitted",
		"execution_bundle.boundary":                     "no boundary evidence was admitted (a trusted-host run, or a run that was not admitted)",
		"execution_bundle.boundary.coverage.filesystem": reasonUnenforced,
		"execution_bundle.boundary.coverage.process":    reasonUnenforced,
		"execution_bundle.boundary.coverage.network":    reasonUnenforced,
		"execution_bundle.boundary.coverage.credential": reasonUnenforced,

		"native_configuration.settings_digests.user":    "no settings digest was recorded for this source",
		"native_configuration.settings_digests.project": "no settings digest was recorded for this source",
		"native_configuration.hooks":                    "no native configuration inventory was admitted",
		"native_configuration.mcp_servers":              "the native session reported no MCP servers",
		"native_configuration.trust_grant":              "no native configuration trust grant was recorded",

		"billing.qualified":                             reasonNotAdmitted,
		"billing.g05":                                   reasonNotAdmitted,
		"billing.credential_provenance":                 reasonNotAdmitted,
		"billing.entitlement_class":                     reasonNotAdmitted,
		"billing.entitlement_source":                    reasonNotAdmitted,
		"billing.paid_continuation":                     reasonNotAdmitted,
		"billing.paid_continuation_user_declaration":    "the user made no paid-continuation declaration",
		"billing.init_api_key_source":                   "the native init event reported no auth source",
		"billing.retail_equivalent_estimate_usd.value":  "the native reported no retail-equivalent estimate",
		"billing.retail_equivalent_estimate_usd.source": "the native reported no retail-equivalent estimate",
		"billing.tokens.source":                         "the native reported no usage",
		"billing.tokens.by_model":                       "the native reported no usage",
		"billing.native_auth":                           "no native login evidence was admitted",
		"billing.native_auth.auth_method":               "the native login status reported no value for this field",
		"billing.native_auth.api_provider":              "the native login status reported no value for this field",
		"billing.native_auth.subscription_type":         "the native login status reported no value for this field",
		"billing.native_auth.identity_ref":              "the native login status reported no value for this field",
		"billing.usage_observations[].unit":             "the observation carries no value for this field",
		"billing.usage_observations[].source":           "the observation carries no value for this field",
		"billing.usage_observations[].label":            "the observation carries no value for this field",
		"billing.usage_observations[].quantity":         "the observation carries no value for this field",
		"billing.reserve.verify_pass_checks":            reasonNoChecks,
		"billing.reserve.verify_timeout_sum":            reasonNoChecks,
		"billing.reserve.repair_ceiling":                reasonNoEnvelope,
		"billing.remaining.quantity":                    "no supported route reports remaining quota",
		"billing.remaining.bucket":                      "the run holds no billing reservation",
		"billing.next_retry.at":                         "the bucket's reset time is not known",

		"native_result.attempt_state": reasonNoAttempt,
		"native_result.subtype":       reasonNoNativeResult,
		"native_result.num_turns":     reasonNoNativeResult,
		"native_result.duration_ms":   reasonNoNativeResult,
		"native_result.session_id":    "the native reported no session",

		"candidate.base_rev":      reasonNoCandidate,
		"candidate.commit":        reasonNoCandidate,
		"candidate.tree_id":       reasonNoCandidate,
		"candidate.patch_sha256":  reasonNoCandidate,
		"candidate.changed_paths": reasonNoCandidate,

		"verification.config_sha256":      reasonNoVerification,
		"verification.evaluator":          "no evaluator was recorded for this verification",
		"verification.result":             reasonNoVerification,
		"verification.label":              reasonNoVerification,
		"verification.reason":             "no reason was recorded for this verification",
		"verification.binding":            reasonNoVerification,
		"verification.checks[].exit_code": "the check did not exit with a status (timed out, unavailable or not run)",
		"verification.checks[].evidence":  "the check recorded no evidence",
		"verification.checks[].sha256":    "the check recorded no evidence",

		"acceptance.policy_pin":                  reasonNotAdmitted,
		"acceptance.policy_pin.check_definition": reasonNoChecks,
		"acceptance.policy_pin.evaluator":        reasonNoChecks,
		"acceptance.bounds":                      reasonNoEnvelope,
		"acceptance.bounds.check_timeouts":       reasonNoChecks,
		"acceptance.overhead_ms":                 reasonNoVerification,
	}
}

// opaqueUnknown names the native-supplied subtrees: their keys are the
// native's, so they never become part of a path.
func opaqueUnknown(path string) bool {
	return path == "billing.tokens.by_model" || path == "native_configuration.mcp_servers"
}

// attachUnknownReasons walks r for leaves equal to unknown, stores each one's
// reason from reasons under r["unknown_reasons"], and returns the sorted paths
// that have none. A reason for a leaf that is not unknown is not stored.
func attachUnknownReasons(r Receipt, reasons map[string]string) (missing []string) {
	found := map[string]string{}
	unregistered := map[string]bool{}
	walkUnknown(map[string]any(r), "", func(path string) {
		if reason, ok := reasons[path]; ok {
			found[path] = reason
			return
		}
		unregistered[path] = true
	})
	r["unknown_reasons"] = found
	for path := range unregistered {
		missing = append(missing, path)
	}
	slices.Sort(missing)
	return missing
}

func walkUnknown(v any, path string, visit func(path string)) {
	switch v := v.(type) {
	case string:
		if v == unknown {
			visit(path)
		}
	case map[string]any:
		for key, child := range v {
			if path == "" && key == "unknown_reasons" {
				continue
			}
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if _, leaf := child.(string); !leaf && opaqueUnknown(childPath) {
				continue
			}
			walkUnknown(child, childPath, visit)
		}
	case []any:
		for _, child := range v {
			walkUnknown(child, path+"[]", visit)
		}
	case []string:
		for _, child := range v {
			walkUnknown(child, path+"[]", visit)
		}
	}
}
