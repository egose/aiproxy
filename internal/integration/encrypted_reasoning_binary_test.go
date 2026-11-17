//go:build integration && linux

package integration

import (
	"net/http"
	"testing"
)

func TestBinaryEncryptedReasoningDirectScopes(t *testing.T) {
	for _, op := range binaryReasoningOperations {
		t.Run(op.name, func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				root       string
				provider   string
				model      string
				statuses   []int
				wantStatus int
				wantBodies []bool
			}{
				{name: "builtin_no_recovery", statuses: []int{400, 200}, wantStatus: 400, wantBodies: []bool{false}},
				{name: "root_fallback_prestrip", root: binaryReasoningStrip, wantStatus: 200, wantBodies: []bool{true}},
				{name: "root_direct_recovery", root: binaryReasoningRetry, statuses: []int{400, 200}, wantStatus: 200, wantBodies: []bool{false, true}},
				{name: "provider_empty_optout", root: binaryReasoningRetry, provider: binaryReasoningReset, statuses: []int{400, 200}, wantStatus: 400, wantBodies: []bool{false}},
				{name: "model_empty_optout", root: binaryReasoningRetry, provider: binaryReasoningStrip, model: binaryReasoningReset, statuses: []int{400, 200}, wantStatus: 400, wantBodies: []bool{false}},
				{name: "provider_whole_block_defaults", root: "encrypted_reasoning {\n passthrough = false\n match_messages = [\"root-only marker\"]\n}", provider: binaryReasoningRetry, statuses: []int{400, 200}, wantStatus: 200, wantBodies: []bool{false, true}},
				{name: "model_prestrip", root: binaryReasoningRetry, provider: binaryReasoningReset, model: binaryReasoningStrip, wantStatus: 200, wantBodies: []bool{true}},
				{name: "direct_two_attempt_bound", root: binaryReasoningRetry, statuses: []int{400, 400, 200}, wantStatus: 400, wantBodies: []bool{false, true}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					upstream := newBinaryReasoningUpstream(t, op, tc.statuses...)
					backup := newBinaryReasoningUpstream(t, op)
					addr := freeAddr(t)
					providers := binaryReasoningProvider("p1", upstream.server.URL, "synthetic-p1", "upstream-one", tc.provider, tc.model) +
						binaryReasoningProvider("p2", backup.server.URL, "synthetic-p2", "upstream-two", "", "")
					srv, _ := startBinaryReasoningServer(t, addr, binaryReasoningConfig(addr, tc.root, providers, binaryReasoningAlias(binaryReasoningStrip, "[400, 503]")))
					postBinaryReasoning(t, srv, op, "p1/m", tc.wantStatus)
					upstream.assertCalls(t, op, "synthetic-p1", "upstream-one", tc.wantBodies...)
					backup.assertCalls(t, op, "synthetic-p2", "upstream-two")
				})
			}
		})
	}
}

func TestBinaryEncryptedReasoningAliasBodyAndAttemptBound(t *testing.T) {
	for _, op := range binaryReasoningOperations {
		t.Run(op.name, func(t *testing.T) {
			for _, tc := range []struct {
				name         string
				root         string
				first        string
				second       string
				alias        string
				retryCodes   string
				firstStatus  []int
				secondStatus []int
				wantStatus   int
				firstBodies  []bool
				secondBodies []bool
			}{
				{name: "explicit_reset_overrides_target_and_root", root: binaryReasoningRetry, first: binaryReasoningStrip, second: binaryReasoningStrip, alias: binaryReasoningReset, firstStatus: []int{400, 200}, wantStatus: 400, firstBodies: []bool{false}},
				{name: "explicit_strip_overrides_reset_targets", root: binaryReasoningReset, first: binaryReasoningReset, second: binaryReasoningReset, alias: binaryReasoningStrip, firstStatus: []int{503}, wantStatus: 200, firstBodies: []bool{true}, secondBodies: []bool{true}},
				{name: "absent_alias_restores_original", first: binaryReasoningRetry, second: binaryReasoningRetry, firstStatus: []int{400, 503}, wantStatus: 200, firstBodies: []bool{false, true}, secondBodies: []bool{false}},
				{name: "explicit_alias_keeps_stripped", first: binaryReasoningReset, second: binaryReasoningReset, alias: binaryReasoningRetry, firstStatus: []int{400, 503}, wantStatus: 200, firstBodies: []bool{false, true}, secondBodies: []bool{true}},
				{name: "absent_alias_single_shared_recovery", first: binaryReasoningRetry, second: binaryReasoningRetry, firstStatus: []int{400, 503}, secondStatus: []int{400, 200}, wantStatus: 400, firstBodies: []bool{false, true}, secondBodies: []bool{false}},
				{name: "explicit_alias_single_shared_recovery", alias: binaryReasoningRetry, firstStatus: []int{400, 503}, secondStatus: []int{400, 200}, wantStatus: 400, firstBodies: []bool{false, true}, secondBodies: []bool{true}},
				{name: "prestrip_keeps_recovery_allowance", first: binaryReasoningStrip, second: binaryReasoningRetry, firstStatus: []int{503}, secondStatus: []int{400, 200}, wantStatus: 200, firstBodies: []bool{true}, secondBodies: []bool{false, true}},
				{name: "prestrip_cannot_defeat_later_reset", first: binaryReasoningStrip, second: binaryReasoningReset, firstStatus: []int{503}, secondStatus: []int{400, 200}, wantStatus: 400, firstBodies: []bool{true}, secondBodies: []bool{false}},
				{name: "ordinary_400_failover_then_recovery", first: binaryReasoningReset, second: binaryReasoningRetry, retryCodes: "[400, 503]", firstStatus: []int{400}, secondStatus: []int{400, 200}, wantStatus: 200, firstBodies: []bool{false}, secondBodies: []bool{false, true}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					first := newBinaryReasoningUpstream(t, op, tc.firstStatus...)
					second := newBinaryReasoningUpstream(t, op, tc.secondStatus...)
					addr := freeAddr(t)
					providers := binaryReasoningProvider("p1", first.server.URL, "synthetic-p1", "upstream-one", tc.first, "") +
						binaryReasoningProvider("p2", second.server.URL, "synthetic-p2", "upstream-two", tc.second, "")
					retryCodes := tc.retryCodes
					if retryCodes == "" {
						retryCodes = "[503]"
					}
					srv, _ := startBinaryReasoningServer(t, addr, binaryReasoningConfig(addr, tc.root, providers, binaryReasoningAlias(tc.alias, retryCodes)))
					postBinaryReasoning(t, srv, op, "alias/route", tc.wantStatus)
					first.assertCalls(t, op, "synthetic-p1", "upstream-one", tc.firstBodies...)
					second.assertCalls(t, op, "synthetic-p2", "upstream-two", tc.secondBodies...)
				})
			}
		})
	}
}

func TestBinaryEncryptedReasoningSIGHUPUpdateAndRollback(t *testing.T) {
	for _, op := range binaryReasoningOperations {
		t.Run(op.name, func(t *testing.T) {
			old := newBinaryReasoningUpstream(t, op)
			updated := newBinaryReasoningUpstream(t, op, 400, 200, 400, 200)
			rejected := newBinaryReasoningUpstream(t, op)
			addr := freeAddr(t)
			source := func(root string, upstream *binaryReasoningUpstream, key, model string) string {
				return binaryReasoningConfig(addr, root, binaryReasoningProvider("p1", upstream.server.URL, key, model, "", ""), "")
			}
			srv, path := startBinaryReasoningServer(t, addr, source(binaryReasoningStrip, old, "synthetic-old", "upstream-old"))
			postBinaryReasoning(t, srv, op, "p1/m", http.StatusOK)
			old.assertCalls(t, op, "synthetic-old", "upstream-old", true)

			writeConfigPath(t, path, source(binaryReasoningRetry, updated, "synthetic-updated", "upstream-updated"))
			reloadBinaryTLS(t, srv, true)
			postBinaryReasoning(t, srv, op, "p1/m", http.StatusOK)
			updated.assertCalls(t, op, "synthetic-updated", "upstream-updated", false, true)

			writeConfigPath(t, path, source(`encrypted_reasoning { on_caller_mismatch = "invalid" }`, rejected, "synthetic-rejected", "upstream-rejected"))
			reloadBinaryTLS(t, srv, false)
			postBinaryReasoning(t, srv, op, "p1/m", http.StatusOK)
			old.assertCalls(t, op, "synthetic-old", "upstream-old", true)
			updated.assertCalls(t, op, "synthetic-updated", "upstream-updated", false, true, false, true)
			rejected.assertCalls(t, op, "synthetic-rejected", "upstream-rejected")
		})
	}
}
