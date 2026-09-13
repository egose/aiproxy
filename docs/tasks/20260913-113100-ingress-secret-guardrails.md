# Ingress Secret Guardrails Using Gitleaks

Created: 2026-09-13 11:31:00 local time

## Objective And Scope

Evaluate reusing Gitleaks to detect suspected secrets in client inference requests, then implement an opt-in ingress guardrail if the integration is suitable. Prefer a pinned Go module behind a small internal interface; consider selective code/rule reuse only if measured dependency, performance, or API constraints justify it.

This is an optional improvement, not a confirmed defect in the existing proxy contract. The requested deliverable for this session is this execution plan. Implementation tasks below depend on the investigation's recorded decisions.

Proposed first release: request-side text inspection before any provider call, with audit and block modes, bounded work, safe diagnostic metadata, configuration validation, and atomic policy reload. Exact operation coverage and public configuration/error contracts are decided in SECRET-01.

Non-goals: response filtering, SSE output redaction, automatic request rewriting, credential validity checks against providers, OCR/audio transcription, arbitrary archive/file scanning, general PII detection, prompt-injection detection, and a new dashboard UI. Gitleaks detects patterns and entropy; it cannot prove a credential is live or guarantee detection of all secrets. Audit mode detects but does not prevent forwarding.

## Evidence And Analysis Coverage

- Gitleaks repository: <https://github.com/gitleaks/gitleaks>. Its `go.mod` declares `github.com/zricethezav/gitleaks/v8`, despite the repository organization name. Pin and verify a published version during the spike; do not ship a local-path `replace`.
- Gitleaks `detect/detect.go` exposes `NewDetector`, `NewDetectorDefaultConfig`, `DetectString`, and `DetectContext`. `config/config.go` embeds `DefaultConfig`. The default constructor uses package-global Viper state, so per-request/default-constructor initialization is inappropriate without further review.
- `DetectContext` checks cancellation between rules/decoding passes and returns findings without an error. It can return partial findings on cancellation; an empty slice alone cannot establish a completed clean scan. Regex calls within a rule are not interrupted by those checks.
- `detectRule` honors `gitleaks:allow` unless `IgnoreGitleaksAllow` is true; trace logging can include `finding.Secret`. `report.Finding` contains secret, match, and source-line fields. An HTTP policy wrapper must control both suppression and diagnostic exposure.
- The default engine is Go stdlib regexp (`regexp/stdlib_regex.go`); `gore2regex` selects an alternative. `detect` imports repository/source/report machinery, and `sources` imports archive support. Static-build compatibility, actual binary-size impact, and runtime cost have not been measured. Gitleaks' MIT license permits reuse subject to notice retention; this repository uses Apache-2.0.
- `internal/httpapi/handler.go` (`Handler.ServeHTTP`, approximately lines 236-343) authenticates/rate-limits, reads a body bounded to 8 MiB, extracts/authorizes/resolves the public model, checks operation support, then branches to direct or alias dispatch. The shared handler is the candidate enforcement point, rather than scanning separately inside each dispatch path.
- The same handler captures `reqBody` in a deferred payload-log closure before model validation/dispatch. `internal/httpapi/payloadlog_test.go` (`TestPayloadLogUnary`) explicitly expects the raw request body and redacted authorization header. A block placed only before dispatch would still allow that captured body to be persisted unless logging is handled deliberately.
- `internal/app/app.go` (`App.Reload`, `buildDependencies`) publishes dependency snapshots through `Handler.UpdateDependencies`. Prepare a candidate scanner before reload side effects/publication; a rejected scanner configuration must leave the active runtime usable.
- `internal/config/types.go` (`Runtime`) and `internal/httpapi/handler.go` (`Dependencies`) currently have no secret-scanning policy. Existing config load/validation, payload-log, handler, and reload tests are the extension points.

Backlog check: a focused search of `docs/` for guardrail/Gitleaks/secret-scanning work found no matching plan. The existing [alias cooldown plan](20260907-104451-alias-upstream-retry-cooldown.md) supplies related routing/reload constraints, not duplicate work. Inspection covered the entry points above and selected payload-log tests, not all adapters, configuration tooling, upstream scanner internals, or dependency platforms.

Planning baseline: worktree was clean before creation. No builds, tests, race checks, or benchmarks were run; source inspection is sufficient for this planning deliverable. Earlier feasibility suggestions about safe shared-detector reuse, binary size, and the simplicity of copying the scan loop remain hypotheses to verify, not measured conclusions.

## Ordered Tasks

P1 means prerequisite correctness/design evidence for the proposed feature. P2 means implementation or release work after those prerequisites, not lower quality requirements. Execute sequentially: scanner policy, configuration/runtime, and shared handler files have dependent contracts. No delegation is required.

### Task SECRET-01: Validate Library Reuse And Record The Ingress Contract

Status: completed

Completion evidence (recorded 2026-09-13; GO decision: direct module import):

- Pinned version: `github.com/zricethezav/gitleaks/v8 v8.30.1` (latest release tag at spike time; local checkout HEAD `b58d3f1` is post-release master, not pinned). Verified with `go get ...@v8.30.1` + `go list -m` in `/tmp/opencode/gitleaks-spike` (isolated module, own `.tool-versions` with golang 1.27.1). Production `go.mod`/`go.sum` untouched (`git status` clean; `grep -c gitleaks go.mod go.sum` = 0; module count still 102).
- Reproducible experiment directory: `/tmp/opencode/gitleaks-spike` (`main.go`, `go.mod`, binaries `spike`, `/tmp/opencode/sizeA`, `/tmp/opencode/sizeB`, `/tmp/opencode/aiproxy-baseline`, `/tmp/opencode/aiproxy-with-gitleaks`, alternate modfile `/tmp/opencode/proxy-spike.mod`). Toolchain: go1.27.1 linux/amd64, CGO_ENABLED=0 for all builds, host 32 CPU / 27 GiB RAM. Commands: `go get github.com/zricethezav/gitleaks/v8@v8.30.1`, `CGO_ENABLED=0 go build -o spike .`, `./spike`, `go test -race -run TestConcurrentDetect .`, and for the linked-size delta `go build -modfile=/tmp/opencode/proxy-spike.mod -o /tmp/opencode/aiproxy-with-gitleaks ./cmd/aiproxy` with a temporary blank-import shim + temp lib package (both deleted afterwards; production `go.mod` never edited).
- Dependency/size evidence: proxy baseline `33,862,789` bytes vs linked-scanner proxy `41,686,498` bytes = `+7,823,709` bytes (`+23%`) with CGO disabled. Module graph `102 -> 268` entries (includes `aho-corasick`, `semgroup`, `zerolog`, `viper`, `mholt/archives` via `detect -> sources` archive support, `go-gitdiff`, `filetype`, `sprig`). No CGO in the imported packages; static `CGO_ENABLED=0` build succeeds. Proxy already requires `spf13/cobra v1.10.2`, so MVS keeps the newer cobra. Gitleaks is MIT (checked `LICENSE` header); attribution required at implementation (SECRET-02).
- Startup cost: one-time config parse + `NewDetector` over the embedded 222-rule default config = `29.6 ms` (single measurement, same host). Per-request regex rebuild is forbidden; share one immutable detector per policy generation.
- Performance evidence (averages over 20 iterations, single-threaded; NOT p99): clean 4 KiB `~0.77 ms`, clean 64 KiB `~11.0 ms`, clean 1 MiB `~189 ms`, clean 8 MiB `~1.65 s` (1 pass), small matching request `~0.037 ms`, dense-match 1 MiB (49,932 findings) `~0.78 s`. Corpus: repeated benign English + random synthetic tokens (`AKIA` + 16 chars from `A-Z2-7`, `ghp_` + 36 alnum from `crypto/rand`). Consequence: full-body scanning is unbounded (`1.65 s` at 8 MiB); v1 MUST cap extracted text. Scan budget: default `max_text_bytes = 65536` keeps average inline cost at `~11 ms` on this host; larger caps are operator opt-in with documented cost. Benchmarks rerun only if the integrated extraction path changes measured cost materially.
- Concurrency/cancellation evidence: 32 goroutines x 50 iterations shared-detector `DetectString` over clean + AWS-key content = 0 errors; `go test -race` concurrent test passes. Safe subset is `Detect`/`DetectContext`/`DetectString` on a shared detector (read-only config, atomic `TotalBytes`); `DetectSource`/`AddFinding` mutate shared state and MUST NOT be used on the shared instance. Pre-canceled context returns 0 findings with `ctx.Err() = context canceled`; the wrapper MUST check `ctx.Err()` after every scan and report incomplete, never clean. Regex execution inside a rule is not interruptible; bounds (byte/string caps) are the cancellation complement.
- Bypass evidence: `gitleaks:allow` suppresses a finding with default settings (`IgnoreGitleaksAllow=false` -> 0 findings) but remains detected with `IgnoreGitleaksAllow=true` (1 finding). v1 MUST set `IgnoreGitleaksAllow=true`. Caller-controlled filenames are never passed (`FilePath` left empty), so path allowlists cannot be triggered by callers. No caller-controlled policy metadata exists.
- Logging exposure evidence: `detectRule` trace/debug logs embed `finding.Secret` (`Str("finding", finding.Secret)`); gitleaks uses a process-global zerolog logger writing to stderr at Info by default (trace/debug suppressed). v1 MUST NOT enable gitleaks trace/debug logging and MUST NOT expose `Secret`/`Match`/`Line` outside the wrapper. Wrapper exposes only outcome + rule IDs + counts.
- False-positive battery: 8 benign samples (API-key discussion, `hunter2`, `tokenize`, `-----BEGIN DISCUSSION-----`, bare `ghp_` mention, `AKIA...EXAMPLE` doc example, repeated-char string, system prompt) = 0 findings. Note: naive sequential test strings (e.g. `A-Z0-9` runs) hit the global stopword allowlist; synthetic fixtures MUST use `crypto/rand` content.
- v1 coverage contract: `POST /v1/chat/completions` and `POST /v1/responses` only (OpenAI-format inbound body, so native and translated providers share one extraction point). Scanned text = JSON-decoded string leaves at: chat `messages[].content` (string form and `input_text`/`text` parts of array form), `messages[].tool_calls[].function.arguments` (raw string; if it parses as JSON, recurse one level into string leaves), tool-role `messages[].content`; responses `instructions` + textual `input` items. Raw-byte scanning is FORBIDDEN as the scan input: JSON decoding (including `\uXXXX` escapes and duplicate keys per `encoding/json`) happens first so a token reconstructed by normal decoding cannot be missed. Explicitly out of scope (treated as unsupported, never clean-asserted for those bytes): JSON keys, URLs as opaque strings are still scanned as text where extracted (no URL fetching), image/audio/embeddings operations, multipart file bytes, attachments, base64/hex blobs (`MaxDecodeDepth=0`, single pass), unknown/nested fields beyond the listed leaves, response/output bodies and SSE streams, request rewriting, validity checks.
- Configuration decision: optional single-label HCL block `ingress_guardrails { enabled, mode, max_text_bytes, max_strings }` (JSON form mirrors it); absent block or `enabled = false` = disabled with zero behavior change. `mode` is `audit` or `block` (default `block` when enabled without a mode). Full embedded default rule set (222 rules at v8.30.1); NO v1 support for custom rules, rule subsets, or operator allowlists (subset selection would silently drop entropy/keyword/allowlist/composite semantics; owner for any future extension: maintainer input required). Startup validation failure fails startup; reload build failure rejects the reload with the active policy intact (candidate built before `Handler.UpdateDependencies`, mirroring the payload-log reload pattern).
- Outcome matrix: clean -> forward unchanged; findings + audit -> forward unchanged + `guardrail_scans_total{operation,mode,outcome}` increment and safe server-side metadata (rule IDs/counts only); findings + block -> `400`/`secret_blocked`, zero upstream I/O (no adapter call, no retries, no alias cooldown/lease, no health mutation, no usage attribution); incomplete (oversize text/strings, canceled scan, concurrency-guard trip) + block -> `400`/`scan_incomplete`, never forwarded; incomplete + audit -> forwarded + `incomplete` outcome recorded (audit never blocks, but the gap stays visible); malformed bodies keep existing `400 invalid_model`/`invalid_body` precedence (scan not reached); out-of-scope operations bypass scanning entirely. Size skipping and cancellation are recorded outcomes, never clean scans. Client error bodies and logs carry no secret text.
- Payload-log decision: whenever guardrails are enabled (audit or block), request bodies for covered operations (chat + responses) are OMITTED from payload-log entries on all paths (success, blocked, early-rejected), because the existing deferred raw-body closure would otherwise persist flagged content. Response bodies keep existing capture behavior as an explicit v1 limitation (response scanning deferred; responses echoing request secrets are not scrubbed).
- Reload/atomicity: policy snapshot per `Dependencies`; `App.Reload` builds the candidate scanner before side effects/publication; failed candidate = reload rejected, old detector serves new requests.
- Go/no-go: GO for direct module import (`detect` + `config` packages; `sources`/repository machinery arrives transitively and is simply never invoked). Measured costs (`+23%` binary, `~30 ms` one-time startup, `~11 ms` average at the 64 KiB cap, CGO-disabled OK) are acceptable for an opt-in guardrail. Selective reuse (copying regexes/scan loop) is REJECTED: it would lose entropy, keyword prefilter, stopword/allowlist, and composite-rule behavior plus upstream-update ownership. Remaining external-policy owner: maintainer confirms the `400`/`secret_blocked` + `400`/`scan_incomplete` codes and the no-custom-rules v1 scope before release if they want different public contracts.
- Acceptance mapping: `gitleaks:allow` + synthetic token stays detectable (`IgnoreGitleaksAllow=true`); cancellation yields incomplete, not clean; concurrent + race experiments pass. No production code was added by this task.

Kind: investigation

Priority: P1; bounds compatibility, bypass, false-positive, and latency questions before introducing public behavior.

Dependencies: none

Primary ownership: this document; disposable experiment under `/tmp/opencode/`; focused Gitleaks and proxy source inspection. Keep exploratory dependency changes out of the production module until a recommendation is recorded.

Finding: An importable detector exists, but repository-oriented defaults, global configuration state, partial cancellation results, diagnostic logging, and dependency breadth require validation for a concurrent HTTP service.

References: Gitleaks `go.mod`, `LICENSE`, `detect/detect.go` (`NewDetectorDefaultConfig`, `DetectContext`, `detectRule`), `config/config.go`, `sources/`, and proxy entry points in the evidence section.

Requirements:

1. Pin a released Gitleaks v8 version and run an isolated import/build experiment. Compare the normal proxy build with an equivalent linked scanner build using matching toolchain/flags: binary bytes, dependency changes, startup/rule compilation cost, and CGO-disabled compatibility. Confirm repository release targets before claiming portability.
2. Benchmark synthetic clean/matching/false-positive-prone chat content at representative small sizes and near the 8 MiB body limit, including dense matches and concurrent scans. Record latency distribution, allocations/memory, concurrency, CPU/toolchain, and corpus construction. Set an evidence-backed acceptable scan budget; do not claim p99 from ordinary benchmark averages.
3. Verify safe detector reuse with immutable configuration, concurrent scanning, and replacement during reload. Check configuration globals and scanner-internal logging. Compare a shared detector or bounded pool only as needed; do not rebuild regexes per request. Exercise cancellation before/during scanning and distinguish partial/incomplete results from clean results.
4. Record exact v1 coverage: recommended starting point is decoded textual content in chat completions and Responses requests, including instructions, history, tool arguments, and tool results. Specify unknown/nested fields, JSON keys/values, JSON-encoded tool arguments, Unicode escapes, malformed/duplicate-key JSON, URLs, attachments, multipart bodies, and requests outside supported coverage. Raw serialized-JSON scanning alone must not miss a token reconstructed by normal JSON decoding. Do not silently claim binary/encoded content coverage.
5. Decide configuration syntax, off-by-default behavior, audit/block naming, enabled rule set, operator-owned allowlists/custom-rule support, and initialization/reload errors. Preserve rule context and allowlist/composite/entropy semantics if selecting a subset. Callers must not disable inspection with `gitleaks:allow`, filenames, or caller-controlled policy metadata.
6. Decide the blocked HTTP status and stable error code, incomplete-scan behavior, maximum inspected bytes/depth/strings, decoding depth, and bounded concurrent work. Recommend block mode never silently forwards a request whose required scan could not complete. Document the behavior of audit mode on scan failure separately. Size skipping and cancellation must be visible outcomes rather than clean scans.
7. Resolve payload-log interaction, including malformed/early-rejected requests. Recommend omitting request bodies for covered traffic whenever guardrails are enabled, or a demonstrably safe equivalent, so neither audit nor block findings are persisted by the existing raw-body closure. Define treatment of responses that echo request secrets; response scanning is outside v1 and must not be implied by request-log protection.
8. Record a go/no-go recommendation: direct module import first; if rejected, identify exact measured constraints and the minimal reuse boundary. Copying regexes alone loses entropy, keyword, allowlist, and composite-rule behavior; any reuse proposal must address parity, upstream update ownership, and MIT notices. Seek maintainer input for unresolved external-contract choices before production implementation.

Acceptance criteria:

- This document contains the pinned version, reproducible experiment commands/results, dependency/size/performance evidence, and a recommended integration strategy or evidence-backed deferral.
- A compact coverage/outcome matrix defines clean, suspected-secret, incomplete, malformed, unsupported, audit, block, logging, and reload behavior. Material open choices have a named owner rather than guessed defaults.
- A request containing a synthetic matching token plus `gitleaks:allow` remains detectable in the selected setup; cancellation does not become an asserted clean result; concurrent experiments pass the race detector.

Verification: isolated Go experiment and evidence review; record exact commands and working directories here. This task may complete with an evidence-backed no-go decision and no production code; mark downstream tasks deferred with the reason if so.

### Task SECRET-02: Implement The Scanner Policy And Runtime Configuration

Status: completed

Completion evidence (recorded 2026-09-13):

- New package `internal/guardrails/guardrails.go`: `Policy{Enabled, Mode, MaxTextBytes, MaxStrings}` with `WithDefaults` (block/65536/512) and `Validate`; `Scanner` built once per policy generation from the embedded gitleaks default config via an isolated `viper.New()` instance (never the global-Viper default constructor), `IgnoreGitleaksAllow=true`, `MaxDecodeDepth=0`; `Scan(ctx, texts)` returns `Result{Outcome, RuleIDs, FindingCount, Reason}` with outcomes `clean`/`flagged`/`incomplete` (`oversize`, `too_many_strings`, `canceled`), distinct rule IDs capped at 16, counts uncapped. Only bounded safe metadata leaves the wrapper; `Secret`/`Match`/`Line` are never retained, logged, or exposed. Scanning is synchronous in the caller goroutine (no spawned work); shared-detector use is limited to `DetectContext`, and `ctx.Err()` is checked before and after every fragment so partial/canceled scans report `incomplete`, never clean. Disabled policy yields a nil scanner (zero startup cost, detector not built).
- Runtime wiring: `config.Runtime.IngressGuardrails`, HCL `ingress_guardrails { enabled, mode, max_text_bytes, max_strings }` plus JSON form through the shared gohcl path, `convert.go` label entry (`ingress_guardrails: 0`), `Validate` rejects unknown modes and out-of-range bounds (text 1024..8MiB, strings 1..4096; negatives rejected; zeros select defaults), omitted block stays disabled with zero behavior change. `App.Build` compiles the scanner at startup (failure fails startup); `App.Reload` builds the candidate before `Handler.UpdateDependencies` and rejects the reload on error with the active scanner intact; `httpapi.Dependencies` carries `Guardrails *guardrails.Scanner` (nil-safe; enforcement is SECRET-03). Metrics: `aiproxy_guardrail_scans_total{operation,mode,outcome}` with `RecordGuardrailScan` (bounded labels only). Attribution: `THIRD-PARTY-NOTICES` carries the gitleaks MIT text; `go.mod`/`go.sum` pin `github.com/zricethezav/gitleaks/v8 v8.30.1`.
- Configuration tooling limitation (explicit): `aiproxy configure` has no subcommand for the new block; operators edit HCL/JSON directly (round-trip verified via `Convert`) and validate with `make validate`. A `configure` subcommand is a follow-up, not v1 scope.
- Changed files: `go.mod`, `go.sum`, `internal/guardrails/guardrails.go`, `internal/guardrails/guardrails_test.go`, `internal/config/types.go`, `internal/config/schema.go`, `internal/config/build.go`, `internal/config/validate.go`, `internal/config/convert.go`, `internal/config/guardrails_test.go`, `internal/app/app.go`, `internal/app/guardrails_test.go`, `internal/httpapi/handler.go` (dependency field only), `internal/observability/metrics.go`, `THIRD-PARTY-NOTICES`.
- Verification: `go test ./internal/guardrails ./internal/config ./internal/app` passes; `go test -race ./internal/guardrails ./internal/app` passes; full `make vet test` passes. Tests cover synthetic AWS/GH tokens flagged with expected rule IDs, benign samples clean, `gitleaks:allow` still flagged, canceled/oversize/oversupply incomplete (never clean), concurrent shared-scanner reuse, invalid startup config rejected, reload swaps policy on success and preserves the active scanner on failure, HCL/JSON load and HCL->JSON->HCL conversion round-trip, and absence of fixture secret text in `Result` metadata.

Kind: improvement

Priority: P2; turns the selected integration into a bounded, reloadable service dependency.

Dependencies: SECRET-01 with a go decision and resolved implementation contract.

Primary ownership: new `internal/guardrails/`; `go.mod`, `go.sum`; `internal/config/`; `internal/app/app.go`; `internal/httpapi/handler.go` dependency declarations; focused scanner/config/reload tests; third-party license notices as needed.

Finding: The proxy has no scanner abstraction or runtime policy. Directly exposing Gitleaks findings or constructing detectors per request would couple HTTP behavior to secret-bearing upstream types and initialization costs.

References: `Runtime`, `Dependencies`, `App.Reload`, `buildDependencies`, and Gitleaks configuration/detection APIs cited above.

Requirements:

1. Implement the selected detector behind a small internal API that distinguishes complete-clean, findings, and incomplete/error outcomes. Expose only bounded safe metadata to callers; do not retain request content or accumulate findings across requests.
2. Build/compile validated policies at startup and before reload publication. Keep each request on a coherent policy snapshot. Verify shared instance/pool ownership according to SECRET-01 and preserve the old policy after any failed reload.
3. Add the agreed optional configuration through HCL and JSON load/build/validate paths, including defaults, invalid values/rules, and conversion behavior. Inspect configuration editing tooling and either support the new block or explicitly document its initial limitations. Custom rule paths/extension sources, if included, are operator-controlled and bounded by the agreed contract.
4. Enforce the chosen scan bounds and suppression/logging controls. Preserve the normal CGO-disabled build and pin the dependency; include required attribution. For selective reuse, implement and test the agreed rule-semantic parity and update procedure.

Acceptance criteria:

- Synthetic known-format tokens and selected benign examples produce the expected outcomes, including multiline and decoded-text cases specified in SECRET-01.
- Suppression markers, rule initialization failures, cancellation, size/depth/concurrency boundaries, and concurrent reuse have focused tests; incomplete work never reports complete-clean.
- Omitted config preserves existing behavior. Invalid startup config fails validation. Successful reload changes new-request policy, and failed reload preserves the active policy, with concurrent request/reload coverage.
- Public/internal diagnostics contain no fixture secret, match, source line, or raw finding serialization, including enabled scanner diagnostic levels.

Verification: from repository root, `go test ./internal/guardrails ./internal/config ./internal/app` and `go test -race ./internal/guardrails ./internal/app`; shared checks below after integration. Record outcomes and any environment prerequisite failures.

### Task SECRET-03: Enforce Ingress Decisions Before Logging And Upstream I/O

Status: completed

Completion evidence (recorded 2026-09-13):

- Enforcement point: `internal/httpapi/guardrail.go` (`guardrailCovered`, `extractGuardrailTexts`, `checkGuardrails`) wired into the shared `Handler.ServeHTTP` after auth, rate limiting, body read, model extraction/authorization, alias resolution, and operation-support checks, and before direct/alias dispatch. Malformed bodies keep existing `invalid_model`/`invalid_body` precedence (scan not reached). Clean request bytes are forwarded untouched (no reserialization); audit mode forwards the original body byte-for-byte (test asserts equality).
- Extraction (decoded text only, never raw bytes): chat `messages[].content` (string and `text`/`input_text` array parts), `tool_calls[].function.arguments` (plus one-level JSON decode of stringified arguments), tool-role content; responses `instructions` plus string/array `input` items (`content` parts, `arguments`). JSON keys, non-string values, unknown fields, image/audio/embeddings operations, multipart file bytes, and base64 blobs are out of scope. Multipart bodies and unparsable JSON on covered operations report `incomplete`, never clean.
- Outcome matrix enforced uniformly for direct/alias, native/translated, and stream-requested paths: flagged/incomplete + block = `400 secret_blocked` / `400 scan_incomplete` with static messages and zero upstream I/O (adapter never invoked: no retries, leases, cooldowns, health mutations, or upstream usage); flagged/incomplete + audit = forwarded with `aiproxy_guardrail_scans_total{operation,mode,outcome}` increment and server-side safe metadata log (rule IDs/counts/reason only); clean/disabled = existing behavior. Client HTTP accounting is preserved (blocked 400s record accounting events like other 400s). Metrics use bounded labels only.
- Payload-log protection: whenever guardrails are enabled, request bodies for covered operations are omitted (`bytes:0`, empty data) on success, blocked, and early-rejected paths, closing the raw-body deferred-closure gap. Response bodies keep existing capture as an explicit v1 limitation (response scanning deferred; audit-mode upstream echoes are not scrubbed). Ordinary access logs and client error bodies carry no secret text (static messages; tests assert absence).
- Changed files: `internal/httpapi/guardrail.go`, `internal/httpapi/handler.go` (omit flag + scan gate), `internal/httpapi/guardrail_test.go`.
- Verification: `go test ./internal/httpapi ./internal/payloadlog ./internal/observability` passes; `go test -race ./internal/httpapi` passes. Tests cover block-mode zero-I/O for direct/alias/SSE-requested/responses paths, audit forwarding fidelity, clean passthrough, oversize/too-many incomplete in both modes, 9 extraction cases (history, array parts, tool args plain + nested JSON, tool results, Unicode escapes, `gitleaks:allow`, responses items), benign samples, malformed precedence, embeddings bypass, metric recording with bounded labels, payload-log omission on all three paths with leakage assertions, and disabled-mode body preservation.

Kind: improvement

Priority: P2; provides the observable filtering outcome consistently for direct and alias requests.

Dependencies: SECRET-02.

Primary ownership: `internal/httpapi/handler.go`; request-text extraction helpers; `internal/httpapi/*guardrail*_test.go`; `internal/httpapi/payloadlog_test.go`; `internal/payloadlog/` and `internal/observability/` only where the agreed logging/metrics contract requires changes.

Finding: Both routing paths share a buffered request boundary, while raw payload logging is registered before dispatch. Enforcement must cover that shared boundary and its logging lifecycle, including early exits.

References: `Handler.ServeHTTP`, `dispatchDirect`, `dispatchAlias`, `TestPayloadLogUnary`, and the body-capturing deferred closure cited above.

Requirements:

1. Extract and scan the agreed request text once before upstream dispatch, applying auth/rate-limit/model authorization and error precedence according to SECRET-01. Preserve valid clean request bytes for adapters rather than reserializing or rewriting forwarded content.
2. Apply the outcome matrix uniformly to direct/alias, native/translated, and stream-request/non-stream-request paths. A blocked request must not reach an adapter or trigger retries, alias leases/cooldowns, provider-health updates, or upstream usage attribution.
3. Implement audit-only forwarding and the safe blocked/incomplete response contract. Preserve client HTTP accounting. If metrics are added, use bounded operation/action/outcome/rule labels, never secret text, arbitrary field paths, request IDs, or secret fingerprints.
4. Apply the agreed payload-log policy before any raw request body can be persisted, including early validation failures. Test ordinary logs, payload logs/dashboard-visible records, and response error bodies for fixture-secret leakage. Keep the response-logging limitation explicit if output bodies remain captured.

Acceptance criteria:

- Synthetic secret requests in block mode produce the documented status/error and zero upstream calls for direct and alias paths, including requests asking for SSE output; retry counters, cooldown, and health remain unaffected.
- Audit mode reports safe metadata and forwards the original request; clean/disabled requests preserve existing routing and streaming behavior.
- Tests cover system/user/history/tool content, Unicode-escaped tokens, nested stringified arguments, selected benign samples, suppression markers, malformed JSON, boundary inputs, and out-of-scope operations according to the recorded matrix.
- Logging-enabled tests demonstrate the chosen request-body protection on successful, blocked, and early-rejected covered requests; no scanner finding content appears in diagnostics or client errors.

Verification: from repository root, `go test ./internal/httpapi ./internal/payloadlog ./internal/observability` and `go test -race ./internal/httpapi`; rerun benchmarks only if the integrated extraction path changes the measured scan cost materially.

### Task SECRET-04: Verify End-To-End Behavior And Publish The Contract

Status: pending

Kind: improvement

Priority: P2; verifies the assembled feature and makes coverage and tradeoffs explicit to operators.

Dependencies: SECRET-03.

Primary ownership: hermetic tests under `internal/integration/`; `README.md`, `docs/design.md`, relevant configuration docs/examples and website pages; `AGENTS.md`, `CHANGELOG.md`; this task document's completion evidence.

Finding: The new opt-in policy changes externally observable request rejection and payload-log behavior. Users need exact coverage, defaults, costs, tuning, and reload semantics rather than a blanket claim of secret prevention.

References: the agreed SECRET-01 matrix, `Makefile` verification targets, `AGENTS.md` public operation/reload contracts, and `TestPayloadLogUnary`'s existing body-preservation expectation.

Requirements:

1. Add a hermetic binary-level smoke test with fake upstreams that demonstrates clean forwarding, blocked zero-I/O behavior, and policy reload according to the supported platform/test harness.
2. Document exact supported operations/text locations, audit versus block behavior, statuses/error codes, logging behavior, false-positive tuning, incomplete-scan policy, rule version/update process, performance evidence, and deferred encoded/binary/output coverage. Update changed tests, public docs, and release notes together.
3. Run the shared checks. Review each task's acceptance evidence and inspect the final diff for secret-bearing metadata, bypass paths, dependency drift, and config/runtime/doc consistency. Record unresolved constraints and executable follow-ups only where necessary.

Acceptance criteria:

- Binary-level tests demonstrate the public behavior with synthetic data and no real provider credentials/network calls.
- Shared checks pass; documentation/configuration examples agree with runtime behavior and no local-checkout dependency remains.
- Each implemented task has changed-file and verification evidence. Any unfulfilled criterion keeps the relevant task blocked with its prerequisite/owner recorded.

Verification: shared checks below and acceptance-evidence review. Serialize build/integration commands that write `dist/aiproxy`.

## Shared Verification And Definition Of Done

All repository commands run from root directory with the repository-declared Go toolchain and module access/cache. Race tests require a working race-supported Go/CGO toolchain; production builds remain CGO-disabled. Use only synthetic secrets and local fake upstreams. Investigation artifacts belong under `/tmp/opencode/`.

After non-trivial production changes, run `make vet test`. At final integration run `make test-race`, `make integration` (includes `make build` with `CGO_ENABLED=0`), and `make docs-contract`. Verify additional release-target compatibility if the dependency experiment identifies platform-specific concerns. Production-code checks have not run during planning; the documentation-only check below does not establish implementation correctness.

The implementation is done when the recorded coverage/outcome matrix is enforced, supported blocked requests cause zero upstream I/O, request logging follows the explicit protection contract, runtime reload is tested, and required checks pass with evidence appended. If SECRET-01 recommends deferral, finish the investigation with supporting measurements and mark implementation tasks deferred rather than claiming the feature exists.

## Open Decisions And Deferred Coverage

- SECRET-01 can start immediately. Production implementation depends on its integration/coverage recommendation and resolution of configuration, HTTP errors, incomplete scans, logging, rule selection, and performance budget; maintainer input owns any remaining external-policy choices.
- Output filtering is deferred because prevention across SSE fragments requires a separately designed buffering/latency contract. Post-hoc scanning cannot prevent bytes already sent to a client.
- Request mutation/redaction is deferred because replacing tokens can change tool arguments and user intent; scan locations are not automatically safe rewrite spans.
- Multimodal, encoded attachments, other unselected operations, and validity verification remain outside the initial protection guarantee. Document residual coverage gaps in the released feature rather than treating uninspected content as clean.

## Planning Verification

- `make docs-contract` from the repository root: passed (`documentation contract matrices match`).
- Whitespace validation with `git diff --no-index --check /dev/null docs/tasks/20260913-113100-ingress-secret-guardrails.md`: passed.
- Reviewed task dependencies, cited evidence, acceptance criteria, and open decisions against the requested task-as-you-go skill. All implementation tasks remain pending; runtime tests and dependency experiments await execution.
