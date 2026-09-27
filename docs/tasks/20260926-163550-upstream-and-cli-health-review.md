# Upstream And CLI Product Health Review

Created: 2026-09-26 16:35:50 local time

## Objective And Product Context

aiproxy centralizes AI-provider credentials and routing behind an OpenAI-compatible
API. Operators need safe configuration conversion, reliable model discovery, and
predictable resource usage even when an upstream misbehaves. This review closes
four concrete gaps in those workflows through shared, testable boundaries.

Scope: upstream redirect isolation, conversion publication, compressed-response
inspection, and CLI model pagination. Public contract changes belong in relevant
help/docs and this execution record. **Do not edit `CHANGELOG.md`.**

## Analysis, Deduplication, And Limitations

- Followed `task-as-you-go skill` and AGENTS.md.
- Existing dirty worktree contains completed work recorded in
  `20260926-145524-business-boundary-health-review.md`. Preserve all of it.
- Read-only review session: `ses_f1ff02a68ffe3Cv4iHKHu1fyJE`. Reviewed README/design,
  CLI conversion/config editing/persistence, inference client construction,
  provider credential headers, model discovery, inspection helpers and tests.
  Coordinator independently inspected principal source locations and Makefile.
- Deduplicated against August health/remediation and September residual, Copilot,
  guardrail and business-boundary tasks. SAFE-02 is a missed consumer of completed
  STORE-01 persistence work, not a replacement implementation. SAFE-01 concerns
  custom API-key headers absent from Go's sensitive redirect-header list; it
  extends the narrower redirect assessment in the completed Copilot plan.
- No baseline tests run during analysis. Findings are source-confirmed; reproduce
  with local stub servers/temp files before closing. No exhaustive dependency,
  deployment, accessibility, or real-provider audit is claimed.
- Existing catalog-performance investigation remains in the September residual
  plan. New distributed billing/provider expansion and other product redesigns
  require separate requirements. These four findings do not need such decisions.

## Execution And Shared Verification

Execute SAFE-01 through SAFE-05 **sequentially**, each in a **fresh sub-agent**;
SAFE-05 must be an independent reviewer. No nested agents or commits. Each owner
sets only their task `in_progress`, then appends `Completion evidence` with files,
actual commands/results, reproduction, and limitations before marking completed.
Use `blocked` if required verification cannot run. Preserve historical findings.
Add necessary scoped discoveries explicitly to requirements before implementing.

P1 means credential/production-availability risk; P2 means bounded operator UX or
maintainability risk. Shared hotspots (app.go, models_upstream.go, docs) are serialized.
Use existing abstractions, minimal source comments per AGENTS.md, and apply_patch.

All commands run in `<repo-root>`. Targeted checks follow each
task; final gates: `make vet test`, `make test-race`, `make integration`,
`make docs-contract`, and `git diff --check`. Serialize UI builds before Go checks
because the embedded asset tree is replaced. Database tests may skip when no test
URL is supplied during focused non-DB changes; disclose that fact. Final review
must use a newly created disposable PostgreSQL fixture and `GOFLAGS=-p=1` for
database-backed suites; never use a developer database. Inspect Docker availability,
use repository documented setup, record no prerequisite skips in DB-owning packages,
and remove only that new fixture afterward. No real upstream credentials needed.

Definition of done: all five tasks have acceptance evidence and passing required
checks, public docs/help agree, prior work is preserved, CHANGELOG has no diff,
and an independent review audits the entire record.

### Task SAFE-01: Keep Upstream Credentials Within Their Configured Origin

Status: completed

Kind: defect

Priority: P1 — redirects can forward custom provider credentials and request bodies.

Suggested agent: shared HTTP policy implementer

Dependencies: none

Primary ownership: `internal/app/app.go` client construction, shared HTTP policy,
`cmd/aiproxy/models_upstream.go`, focused provider/app/CLI tests and public docs.

Finding: inference and CLI clients use default redirects; Go strips selected
Authorization/cookie headers across hosts but copies `x-api-key` and
`x-goog-api-key`. An upstream/intermediary redirect can disclose credentials to
another origin. Existing timeout/header tests cover only initial destinations.

References: `internal/app/app.go` (`newHTTPClient`),
`cmd/aiproxy/models_upstream.go` (`upstreamHTTPClient`, Anthropic/Gemini listing),
`internal/provider/{anthropic,gemini}.go` (credential headers),
`internal/provider/provider.go` (`executeUpstream`).

Requirements:

1. Share a policy denying cross-origin and HTTPS-to-HTTP redirects before follow-up
   I/O. Preserve same-origin redirects with a finite hop limit and explicit errors.
2. Apply to inference and CLI discovery; preserve pooling, header timeouts, stream
   lifetime and the stricter existing OAuth login redirect policy.
3. Document the changed transport contract; avoid treating blocked 3xx as success.

Acceptance criteria:

- Local two-origin tests cover both API-key headers, 302/307/308, downgrade, loops,
  and allowed same-origin behavior; prohibited destinations receive zero requests.
- Production client assembly and CLI actually use the policy, and blocked inference
  returns a controlled failure rather than translated success or credential leakage.

Verification: `go test -race ./internal/provider ./internal/app ./cmd/aiproxy`
plus the new shared-policy package tests, then shared final gates.

Completion evidence:

- Changed: new `internal/upstreamhttp/redirect.go` and `redirect_test.go`;
  `internal/app/app.go` (only the shared-policy import and `newHTTPClient`
  attachment in this session); new `internal/app/upstream_redirect_test.go`;
  `cmd/aiproxy/models_upstream.go` and new `models_upstream_redirect_test.go`;
  transport-contract additions in `README.md` and `docs/design.md`.
- Policy: compare every redirect to the original scheme, case-insensitive hostname
  and effective port (omitted HTTP/HTTPS ports are 80/443). Permit up to 10
  same-origin redirects; return explicit origin/limit errors before follow-up I/O.
  Returning errors rather than `ErrUseLastResponse` keeps rejected responses out
  of adapter success translation. Existing pooling, header timeout, stream lifetime,
  alias transport-error handling and the stricter OAuth policy are retained.
- Reproduced before attaching the policy using
  `go test ./internal/app ./cmd/aiproxy -run 'Test(BuildUpstreamRedirectIsolation|ListUpstreamModelsRedirectIsolation)$' -count=1`.
  Expected failure: all 21 prohibited-origin cases reached the second local
  server. Both API-key headers and custom forwarded headers leaked; 307/308 also
  replayed the synthetic prompt. Port-only redirects also forwarded bearer tokens.
  Inference returned 200 (including translated Anthropic/Gemini responses), and
  CLI discovery returned models without an error. All fixtures used synthetic
  credentials and local HTTP/TLS servers, with no real upstream access.
- Regression coverage: 18 assembled inference cases (Anthropic, Gemini, OpenAI ×
  302/307/308 × same/cross-origin) and 24 CLI listing cases (those three plus
  Copilot), asserting zero prohibited-destination calls, credential/custom-header
  retention on allowed redirects, replayable POST bodies on 307/308, and Go's GET
  conversion on 302. Inference cross-origin failures are controlled JSON 502
  `upstream_error`; CLI failures return no models. Assembled inference and both
  paginated CLI adapters also reject redirect loops after 11 total requests.
  Shared-policy tests cover local TLS-to-HTTP downgrade after an allowed hop,
  both API-key headers plus bearer/custom headers, changed hostname/subdomain/port,
  scheme changes even on the same explicit port, default-port equivalence, IPv6,
  unsupported schemes, exact 10-redirect success, over-budget chains and loops.
- Verified: focused new tests passed after the fix. Final
  `go test -race ./internal/provider ./internal/app ./cmd/aiproxy ./internal/upstreamhttp ./internal/copilotlogin`
  passed all five packages (provider/OAuth cached; app 22.395s, CLI 14.941s,
  shared policy 1.340s). This includes existing timeout/pooling/stream and OAuth
  redirect regressions. A preceding formatting attempt caught a misplaced new
  test function inside its HCL fixture; corrected before the successful race run.
  `make vet test`, `make docs-contract`, `git diff --check`, and
  `git diff --exit-code -- CHANGELOG.md` passed.
- Limitations: `AIPROXY_TEST_DATABASE_URL` is unset (checked without printing any
  secret). DB-dependent tests skipped, including app's
  `TestBuildLateFailureClosesAdminStore` and the DB-backed store/dbmerge/httpapi
  cases in `make vet test`; these are not claimed as verified. SAFE-05 owns the
  fresh disposable PostgreSQL fixture, full race/integration gates and final
  combined audit. No UI build or real-provider test was run for this task.
- Preservation/status audit: existing business-boundary changes, app reload/resource
  code, coordinator edits and the unrelated old task file were preserved. No
  CHANGELOG changes, commits or subagents. No new blocker or independent follow-up
  discovered; SAFE-02 through SAFE-05 remain untouched by this implementer.

### Task SAFE-02: Publish Converted Configurations Securely And Atomically

Status: completed

Kind: defect

Priority: P1 — conversion materializes secrets into permissive or symlink targets.

Suggested agent: secure CLI persistence implementer

Dependencies: SAFE-01

Primary ownership: `cmd/aiproxy/convert.go`, conversion tests,
`internal/filestore` only for the missing no-clobber primitive, help/docs.

Finding: `runConvert` uses `os.Stat` then `os.WriteFile(..., 0600)`. Existing file
modes stay permissive under force; symlinks are followed, including dangling links
without force; writes directly truncate. Config conversion expands env secrets.
Current tests cover ordinary overwrite, not these persistence boundaries.

References: `cmd/aiproxy/convert.go` (`runConvert`),
`internal/config/convert.go` (`Convert`), `internal/filestore/filestore.go`
(staged secure writes), `internal/configedit/configedit.go` (existing consumer).

Requirements:

1. Reuse secure staged persistence; replace files at exact 0600, reject symlinks and
   non-regular destinations, and preserve previous contents on publication failure.
2. Preserve non-force refusal atomically even with competing publishers; extend
   the shared boundary narrowly if needed rather than relying on Stat-then-write.
3. Preserve validation-before-write and stdout conversion; explain materialized
   env secrets in help and document destination behavior.
4. Scoped discovery: `runConvert` ignores stdout write errors. Propagate conversion
   payload and confirmation write failures so failed output is not reported as
   success; a confirmation error must state that file publication already succeeded.
   This applies only to conversion, not other CLI commands.

Acceptance criteria:

- Regression tests cover 0644 replacement, live/dangling links with/without force,
  concurrent non-force publication, non-regular targets, injected publication failure,
  valid conversion and stdout. Failure does not overwrite another target/old file.
- Existing filestore/configedit consumers keep their contracts.

Verification: `go test -race ./cmd/aiproxy ./internal/config ./internal/filestore ./internal/configedit`;
shared final gates.

Completion evidence:

- Changed: `cmd/aiproxy/convert.go`; new `convert_persistence_test.go` and
  `convert_persistence_linux_test.go` in the same directory;
  `internal/filestore/filestore.go` and new `internal/filestore/create_test.go`;
  conversion-contract additions in `README.md`, `docs/design.md`, and
  `website/docs/configuration.md`. Only SAFE-02's status/requirements/evidence
  were changed in this execution record; SAFE-01 was already completed.
- Publication: conversion validates first and selects `filestore.WriteFile` for
  force or the new narrow `filestore.CreateFile` otherwise. Both share existing
  same-directory staging, exact `0600` chmod, file sync/close, destination `Lstat`
  checks, and directory sync. Force atomically renames over a regular file;
  non-force atomically hard-links the completed staged inode into an absent
  destination, then removes the staging name. `EEXIST` is preserved through the
  CLI's `--force` guidance. There is no stat-then-write or non-atomic fallback;
  lack of filesystem hard-link support fails publication. Missing parents use
  the existing filestore `0700` directory option (subject to umask).
- Reproduced before implementation with
  `go test ./cmd/aiproxy -run 'TestConvert(SecureReplacement|RejectsUnsafeDestination|ConcurrentNonForce|ValidationBeforePublication|OutputFailure)$' -count=1`:
  failed as expected because force retained `0644`, live symlinks were followed
  under force, dangling symlinks were followed with and without force, and both
  stdout payload and confirmation write errors returned success. The concurrent
  CLI case did not fail in that baseline run; the deterministic shared-boundary
  regression below supplies the race proof rather than relying on scheduling.
- Regression coverage: successful force replacement yields the exact validated
  output at `0600` while an already-open old inode still reads its original
  contents; synthetic `env()` secrets appear in the file but not the confirmation.
  CLI tests reject live/dangling links, directories, and (on Linux) FIFOs in both
  force modes without changing referents/destinations. Sixteen competing CLI
  publishers yield one complete winner. Shared-boundary tests hold all 16
  publishers after staging sync before allowing publication, require exactly
  one winner and `os.ErrExist` for every loser, and check mode/content/temp cleanup.
  A `beforeLink` hook creates a competing regular file, live/dangling symlink, or
  directory after the destination check: every link fails without overwriting it.
- Fault injection is at the real shared filestore publication boundary, not a
  mocked CLI writer: write, sync, and rename/link failures cover both absent and
  existing destinations in both modes (12 cases), preserving old bytes and modes
  and removing staged files. Staged bytes/mode are checked immediately before
  publication. An injected post-link cleanup failure reports publication and
  retains only complete `0600` files. Invalid configs neither replace old files
  nor create new ones nor emit a converted stdout payload. Existing valid HCL/JSON,
  compact/stdout, default-name, overwrite, filestore recovery/concurrency, and
  configedit consumer tests pass.
- Scoped stdout correction: payload output now returns wrapped write errors.
  Confirmation failures also return errors explicitly saying the configuration
  was already published; tests assert both error identity and the complete file.
  This was recorded in requirement 4 before implementation and affects conversion
  only. Help/docs explain materialized secrets, exact file permissions, symlink/
  non-regular refusal, atomic no-clobber, and post-publication failure semantics.
- Verified in sequence (no overlapping builds):
  `go test ./cmd/aiproxy ./internal/filestore -run 'Test(Convert|CreateFile|SingleFilePublication)' -count=1`
  passed (CLI 0.287s, filestore 0.157s); after the final Linux FIFO regression,
  `go test -race ./cmd/aiproxy ./internal/config ./internal/filestore ./internal/configedit`
  passed all four packages (16.547s, 2.470s, 2.032s, 1.637s).
  `make vet test`, `make docs-contract`, `git diff --check`, and
  `git diff --exit-code -- CHANGELOG.md` passed. Changed Go files were gofmt'd.
- Limitations: `AIPROXY_TEST_DATABASE_URL` is unset (checked without exposing any
  value). DB-dependent tests in the default suite skip, including the store,
  dbmerge, httpapi, and app DB cases; DB coverage is not claimed. SAFE-05 owns the
  new disposable PostgreSQL fixture, full race/integration gates and final audit.
  Tests used only local temporary files and synthetic secrets. Filesystem checks
  ran on Linux; no cross-platform runtime or UI build was needed/run. As with
  existing filestore, parent directories use normal path traversal, and failures
  after successful publication (cleanup/directory sync/confirmation) may leave
  the complete new file in place; they are not failed rename/link operations.
- Preservation/status audit: all extensive preexisting dirty work and SAFE-01
  were preserved; no CHANGELOG edits, commits, or subagents. SAFE-03 through
  SAFE-05 statuses are unchanged. No unmet SAFE-02 criterion or blocker remains.

### Task SAFE-03: Bound Compressed Inspection Before Allocation

Status: completed

Kind: defect

Priority: P1 — opt-in alias inspection can allocate arbitrarily large expanded zstd data.

Suggested agent: bounded decoding and alias dispatch implementer

Dependencies: SAFE-02

Primary ownership: `internal/provider/provider.go` inspection helper,
`internal/httpapi/{dispatch,encrypted_reasoning}.go`, focused tests/benchmark and docs.

Finding: zstd `DecodeAll` uses default large decoder limits and only afterward
slices to 1 MiB, retaining the expanded allocation. Alias strip-and-retry dispatch
decodes before its mismatch helper rejects non-400 status. Small-payload tests
verify content but cannot establish allocation bounds.

References: `internal/provider/provider.go` (`DecodeBodyForInspection`,
`decodeContentEncoding`, `capInspectDecoded`); `internal/httpapi/dispatch.go`
(inspection calls), `internal/httpapi/encrypted_reasoning.go` (mismatch predicate).

Requirements:

1. Bound decoded output and decoder window/memory before large allocations;
   preserve original response bytes when optional inspection is unsupported/too large.
2. Gate decoding by status/request applicability first. Retain small valid mismatch
   retries, encoding-chain behavior, and client-visible pass-through bytes.
3. Record a relevant allocation benchmark/experiment rather than claiming speedup.
4. Scoped discoveries (recorded before implementation): bound Content-Encoding
   parsing/layers before constructing decoders, including repeated header fields;
   cap at four layers and 256 header-value bytes. Apply limit-plus-one overflow
   detection to the existing gzip/deflate/Brotli paths so every rejected chain
   returns the original bytes, rather than a truncated intermediate result.
5. Expose inspection success separately from fallback bytes: rejected compressed
   data may contain literal mismatch text and must not trigger strip-and-retry.
   Keep existing semantic assertions while updating the internal helper's return
   contract. Gate status, streaming, patterns and request applicability before
   calling the decoder; verify skipped decoding through allocation assertions.

Acceptance criteria:

- Cover advertised large frames, highly compressible over-budget output, exact
  boundaries, corrupt input and chains. Output-length-only tests are insufficient.
- Assembled alias regressions verify small compressed mismatch retry and safe
  over-budget handling with original bytes preserved; inapplicable responses skip decoding.
- Allocation evidence demonstrates bounded behavior as expanded payload size grows.

Verification: `go test -race ./internal/provider ./internal/httpapi`, newly named
inspection benchmark with `-benchmem -count=5`, and shared final gates.

Completion evidence:

- Changed: `internal/provider/provider.go`, `provider_test.go`, new
  `internal/provider/inspection_bounds_test.go`; `internal/httpapi/dispatch.go`,
  `encrypted_reasoning.go`, new `encrypted_reasoning_inspection_test.go`; bounded
  inspection contract/rationale in `README.md` and `docs/design.md`. Existing
  provider assertions retain their exact content expectations and now additionally
  assert inspection success/failure. Existing encrypted-reasoning tests were not
  rewritten. Only SAFE-03's status, scoped requirements and evidence changed here.
- Decoder review: inspected the pinned `github.com/klauspost/compress v1.19.2`
  `zstd/decoder_options.go`, `decoder.go` (`DecodeAll`) and `framedec.go`
  (`runDecoder`). Defaults allow 64 GiB decoded size, a large window and up to four
  decoders. `WithDecoderMaxMemory` has different streaming/non-streaming semantics
  and is not a total-heap ceiling. The implementation keeps bounded `DecodeAll`
  with explicit concurrency 1, low-memory mode, 1 MiB max window, 1 MiB max decoded
  memory and `WithDecodeAllCapLimit(true)` against a fixed 1 MiB destination.
  Known frame sizes are checked before size-driven allocation; unknown-size output
  cannot grow without the capacity bound. Decoder/block bookkeeping is additional
  bounded overhead. No asynchronous streaming or small-reader DecodeAll shortcut
  is used, so streaming-specific buffer options are not needed.
- Recorded scoped additions 4–5 before implementation. All Content-Encoding fields
  are parsed in order, before any decoder construction, with a maximum of four
  comma-separated entries and 256 bytes including joining commas. Unsupported
  inner encodings also reject the whole chain before decoding the outer layer.
  Each gzip/raw-deflate/Brotli layer uses a limit-plus-one read and rejects overflow
  and read/trailer failures. Zstd rejects oversized frames/windows/output rather
  than slicing after expansion. Any layer's failure returns the original input
  slice and `false`; alias matching requires `true`, preventing literal mismatch
  text in failed compressed input from causing a strip-and-retry.
- Applicability: dispatch retains the opt-in operation/policy and already-stripped
  guards; `isEncodedCallerMismatch` checks nil result, streaming, HTTP 400, nonempty
  body/patterns and request opaque markers before invoking decoding. Original
  response bodies/headers remain untouched. Configured status-code retries and
  cooldown semantics are separate and preserved.
- Reproduced before source implementation with
  `go test ./internal/provider -run '^TestInspectionZstdAllocationBound$' -count=1 -v`.
  All three regressions failed the 4 MiB allocation budget: 2 MiB expansion from
  227 compressed bytes allocated **11,233,614 B/op**; 16 MiB from 1,795 bytes
  allocated **94,197,598 B/op**; 64 MiB from 7,171 bytes allocated
  **364,405,256 B/op**. Measurement uses `runtime.MemStats.TotalAlloc` deltas over
  four calls after GC, with fixture construction outside the measured interval.
  Fixtures stream repeated 64 KiB chunks through a single encoder with a 1 MiB
  window; their parsed headers assert unknown frame size and an in-budget window,
  so the test exercises actual expansion rather than only advertised-size rejection.
- Provider regression acceptance: all four codecs cover 1 MiB minus one, exact
  1 MiB and plus one, immutable input, original-allocation fallback, and truncated
  boundary trailers. Zstd cases cover an advertised 1 TiB frame with a 1 MiB window,
  a 64 MiB window, unknown-size 64 MiB expansion, checksum corruption, trailing
  garbage, and concatenated frames at/above the total output limit. Chain cases
  cover reverse-order two/four-layer decoding, repeated fields, mixed case,
  identity entries, five-layer/five-field rejection, exact/over metadata limits,
  unknown/corrupt inner layers, and oversized inner/outer output. Allocation tests
  require under 4 MiB/call for expanding streams and under 2 MiB/call for huge
  advertised sizes/windows; these include runtime noise, not just output length.
  After the fix, the focused non-race allocation test measured 1,353,816 / 1,352,230 /
  1,352,258 B/op for 2/16/64 MiB expansion and 1,049,988 / 1,049,960 B/op for the
  advertised-size/window cases, all passing.
- Assembled acceptance: 26 cases use the real HTTP handler, resolver, provider
  adapter and local upstream server across chat completions and Responses. Small
  zstd and zstd+gzip mismatches retry with the same credential, preserve text and
  remove the opaque blob. Repeated mismatch retries exactly once then preserves
  the second compressed response. Known/unknown-size oversized data, corrupt and
  unsupported literal mismatch bodies, excessive layers, non-400 responses,
  no-opaque requests, disabled policy and direct models preserve original response
  bytes/encoding/marker headers with one upstream call. Seven focused guard cases
  (nil, streaming, success, other error, empty patterns, no opaque marker, empty
  body) each measured **zero allocations** over 20 calls using a valid compressed
  mismatch fixture, establishing that decoding was skipped rather than merely
  ignored after allocation.
- Benchmark command (passed, 50.586s):
  `go test ./internal/provider -run '^$' -bench '^BenchmarkInspectionZstdBounded$' -benchmem -count=5 -cpu=1`.
  Environment: Go 1.27.1, linux/amd64, Intel Core i9-13950HX; `-cpu=1` controls
  GOMAXPROCS, decoder concurrency is explicitly one, fixture generation/header
  construction are outside `b.Loop`, and every measured call constructs/closes its
  own decoder (no pool warm-up assumption). All five samples' B/op and allocs/op
  were identical within each row:

  | Fixture                  | B/op (each of 5 runs) | allocs/op | ns/op, five actual samples                 |
  | ------------------------ | --------------------: | --------: | ------------------------------------------ |
  | Unknown-size 2 MiB       |             1,344,908 |        22 | 1108968, 1036492, 932063, 1057080, 1009508 |
  | Unknown-size 16 MiB      |             1,344,908 |        22 | 1021755, 1198637, 890884, 620091, 578407   |
  | Unknown-size 64 MiB      |             1,344,908 |        22 | 654302, 683744, 726091, 1398405, 756229    |
  | Small valid mismatch     |             1,049,992 |        11 | 202583, 195859, 183649, 200810, 180730     |
  | Advertised 1 TiB         |             1,049,912 |        10 | 150731, 137950, 171335, 149647, 132923     |
  | Advertised window 64 MiB |             1,049,912 |        10 | 119420, 174138, 228349, 368646, 402421     |

  These demonstrate allocation independent of expanding payload size, not a
  throughput/speedup claim. Small valid zstd inspection deliberately reserves a
  full 1 MiB buffer. Limits are per inspection/layer, not a process-wide heap cap;
  original upstream body storage and concurrent requests have separate costs.

- Verification, serialized with no UI build: provider inspection tests passed
  (0.878s); new assembled/guard and existing strip-and-retry focused tests passed
  (0.414s). Required `go test -race ./internal/provider ./internal/httpapi` passed
  (23.625s / 26.358s). `make vet test` passed (including provider 8.727s, httpapi
  5.397s, app 17.189s and CLI 11.997s); `make docs-contract` reported matching
  matrices; `git diff --check` and `git diff --exit-code -- CHANGELOG.md` passed.
  Changed Go files were gofmt'd. All SAFE-03 acceptance criteria pass.
- Limitations/ownership: `AIPROXY_TEST_DATABASE_URL` was checked without printing a
  value and is **unset**. DB-dependent cases in httpapi/store/dbmerge/app skip and
  are not claimed as verified. SAFE-05 owns the newly created disposable PostgreSQL
  fixture, full-repository race/integration gates and independent combined audit.
  No external-provider credentials, UI build, commits or subagents were used.
  Baseline dirty files, prior task work and all other task statuses were preserved;
  CHANGELOG remains untouched. No SAFE-03 blocker or independent follow-up remains.

### Task SAFE-04: Bound And Validate Upstream Model Discovery

Status: completed

Kind: defect

Priority: P2 — repeated/unique cursors can hang discovery and grow memory indefinitely.

Suggested agent: CLI pagination and operator UX implementer

Dependencies: SAFE-03

Primary ownership: `cmd/aiproxy/models_upstream.go`, discovery tests, CLI docs/help.

Finding: Anthropic/Gemini pagination loops have no cycle/page/aggregate budget and
concatenate opaque cursors into URLs. The 8 MiB per-page truncated read and
per-request timeout do not bound complete traversal. Tests cover single pages only.

References: `cmd/aiproxy/models_upstream.go` (`listAnthropicModels`,
`listGeminiModels`, other list body reads), `cmd/aiproxy/models.go` (command caller).

Requirements:

1. Introduce named finite page, model and aggregate-byte budgets and whole-list
   deadline with rationale; share traversal/read validation where it improves clarity.
2. Detect cursor cycles/inconsistent continuation metadata, escape query cursors,
   and use limit-plus-one reads to reject oversized pages explicitly.
3. Preserve cancellation/body closure/display names/config annotations. Incomplete
   retrieval returns an actionable error, never silent partial success. Document limits.

Acceptance criteria:

- Multi-page success, repeated/cyclic/continually unique cursors, empty continuation
  pages, special-character cursors, page/aggregate/model budget boundaries, deadline
  and cancellation are covered with finite request counts and controlled errors.
- Shared single-page readers also reject oversized responses and existing provider
  header/redirect/display behavior remains correct.

Verification: `go test -race ./cmd/aiproxy` and shared final gates.

Completion evidence:

- Changed: `cmd/aiproxy/models_upstream.go`; new
  `cmd/aiproxy/models_upstream_bounds_test.go`; command help in
  `cmd/aiproxy/models.go`; discovery contract/rationale in `README.md` and
  `docs/design.md`. SAFE-03 was completed before this sequential session began.
  Only SAFE-04's status/evidence changed in this task record.
- Bounds: named internal constants set inclusive maxima of **100 pages, 10,000
  model entries, 8 MiB per response body, 32 MiB aggregate response bodies, and
  two minutes per listing**. The existing page allowance accommodates rich model
  metadata; the aggregate/entry/page limits allow large catalogs while bounding
  accumulated output, metadata processing, cursor history and request counts.
  Entries include duplicate and blank-ID records before filtering. Bytes include
  the full JSON/metadata/whitespace supplied by the HTTP body reader, including
  automatic gzip decompression where applicable. These are per-listing resource
  bounds, not a total-process heap ceiling; JSON decoding has additional temporary
  storage bounded by the page input. No public configuration was added.
- Shared enforcement: a per-invocation discovery state owns the existing
  policy-wired HTTP client and page/entry/byte/cursor accounting. Every provider's
  reader uses `readPage`, which checks remaining budgets before I/O and reads at
  most `min(page limit, remaining aggregate limit) + 1` bytes. Overflow returns an
  explicit error, including for non-200 responses. Terminal pages at exact limits
  succeed; continuation after page/aggregate exhaustion makes no extra request.
  All response bodies close before decoding, another page, or return. Ordinary
  upstream status errors and Copilot's 401/403 re-login guidance are retained.
- Traversal: both paginated adapters query-escape opaque cursors and reject any
  repeated cursor, including cycles longer than one page. Anthropic continuation
  requires `has_more`, a nonempty page, and nonempty `last_id` matching the final
  entry; a terminal page can still carry `last_id`. Gemini empty continuation pages
  remain valid but consume the same page/byte/deadline budgets. All errors discard
  accumulated models, are wrapped as incomplete discovery, and produce no partial
  CLI model output. Display names/config annotations, provider credentials/version
  headers/User-Agent/OpenCode session headers, cancellation and SAFE-01's
  `CheckRedirect: upstreamhttp.CheckRedirect` assembly are preserved.
- Reproduced before source implementation with
  `go test ./cmd/aiproxy -run '^TestUpstreamDiscoveryRegression$' -count=1`:
  all three regressions failed (0.246s). Repeated Gemini cursors reached a fourth
  request, where the local fixture deliberately returned 418 to stop the old loop;
  the corrected code rejects the cycle after two calls. Anthropic `has_more:true`
  without `last_id` returned one model and no error. An OpenAI-style response with
  valid JSON followed by over 8 MiB of whitespace was truncated into valid JSON
  and returned one model without error. The retained regressions now require
  explicit errors and nil results. No external credentials/network were used.
- Acceptance coverage in the new test file:
  - Both paginated adapters: multi-page success with output annotations/display
    names; repeated and three-page cyclic cursors; unique-cursor traversal at
    99/100/101 pages (101 stops after 100 requests); exact decoded query values for
    cursors containing `+`, spaces, `/`, `?`, `&`, `#`, `%`, `=`, Unicode and CR/LF,
    with no injected query fields and headers checked on every page.
  - Empty terminal pages, Anthropic empty/missing/mismatched continuation errors,
    legitimate terminal `last_id`, Gemini empty continuation success/cycle, 100
    continually unique empty Gemini pages, malformed later JSON and token types.
  - Entry counts 9,999/10,000/10,001 for OpenAI-style and Copilot single-page lists
    and Anthropic/Gemini aggregate two-page lists. Repeated IDs across pages still
    count; blank-ID records cannot bypass the entry limit.
  - All eight provider types accept 8 MiB minus one and exact 8 MiB pages, reject
    plus one, and make exactly one request. Both paginated adapters cover aggregate
    32 MiB minus one/exact/plus one across five pages; full aggregate exhaustion
    with continuation stops after four exact-8-MiB pages. A real HTTP gzip response
    with a small wire body and oversized decoded JSON/whitespace is also rejected.
  - Instrumented bodies assert closure on success, decode/status/read errors and
    size failures; oversize reads consume exactly limit-plus-one, including a
    100-byte remaining aggregate budget and oversized non-200 response. Multi-page
    instrumentation asserts each prior body is closed before the next request.
- Deadline/cancellation evidence: `testing/synctest` runs the actual production
  two-minute deadline, without changing constants or wall-clock waits. For both
  paginated providers, 25-second simulated body reads with unique cursors and a
  30-second per-request timeout terminate at exactly 120 seconds on request five,
  with nil models and all bodies closed. Earlier parent deadlines terminate at
  40 seconds/request two, explicit cancellation at 35 seconds/request two, and a
  10-second per-request timeout still terminates request one. Error identities
  preserve `context.DeadlineExceeded`/`context.Canceled`. Local HTTP tests cover
  pre-cancelled zero-I/O requests, cancellation before headers and during bodies;
  deterministic post-read cancellation checks both terminal-success suppression
  and stopping continuation before another request.
- Verification, serialized with no concurrent builds: focused
  `go test ./cmd/aiproxy -run '^TestUpstreamDiscovery' -count=1` passed (5.124s).
  The first full CLI race run exposed the existing `TestModelsCommandWired` help
  opening-phrase contract; the new long help was adjusted to retain that phrase,
  without weakening the old test. Final **`go test -race ./cmd/aiproxy` passed
  (85.156s)**, including the new bounds/help tests and existing redirect, provider
  header, display, conversion and other CLI regressions. **`make vet test` passed**
  (CLI 20.246s, remaining tested packages cached); **`make docs-contract`** reported
  matching matrices; **`git diff --check`** and
  **`git diff --exit-code -- CHANGELOG.md`** passed. Changed Go files were gofmt'd.
- Limitations/ownership: `AIPROXY_TEST_DATABASE_URL` is **unset**, checked without
  exposing a value. URL-dependent DB cases in app/store/dbmerge/httpapi skip and
  are not claimed as verified. SAFE-05 owns the fresh disposable PostgreSQL fixture,
  full-repository race/integration gates and independent combined audit. No UI
  build, real-provider test, commit or subagent was used. Preexisting dirty
  business-boundary work, SAFE-01..03, all other task statuses and CHANGELOG were
  preserved. All SAFE-04 acceptance criteria pass; no SAFE-04 blocker remains.

### Task SAFE-05: Independently Verify Integration And The Completed Record

Status: completed

Kind: improvement

Priority: P1 — verify shared transport, persistence and resource bounds together.

Suggested agent: independent final reviewer, not any prior implementer

Dependencies: SAFE-01, SAFE-02, SAFE-03, SAFE-04

Primary ownership: review all plan changes, scoped corrections, this task record.

Finding: isolated passing changes need a final source/contract/acceptance audit
against the existing dirty baseline and combined application.

References: preceding tasks and their Completion evidence; relevant public docs.

Requirements:

1. Read every task's evidence and verify source/tests against each acceptance
   criterion. Inspect alternate entry paths, credential isolation, allocation bounds,
   publication errors and pagination termination. Correct scoped issues with regressions.
2. Run shared final gates with a fresh disposable PG fixture, serialize builds,
   record cleanup and actual test results. Do not count skipped DB coverage as passing.
3. Confirm prior work survives, CHANGELOG is untouched, and statuses/evidence are
   honest. Record audit limitations and any unresolved blockers explicitly.
4. Scoped final-review discoveries (before corrections): SAFE-01's constructor
   wiring misses health probes (`healthcheck.New` uses a default client and can
   send bearer credentials across a port-only redirect) and provider fallback/
   injected clients (`clientFor`). Enforce the same original-origin policy at
   these execution boundaries without mutating shared clients, retaining custom
   transports/timeouts and stricter redirect callbacks. Regressions must exercise
   these alternate paths and prove zero prohibited destination requests.
5. SAFE-02 platform clarification: release targets include Windows, where Go's
   `Chmod(0600)` only controls the read-only attribute and `Rename` does not promise
   atomic replacement. The current exact-0600/atomic claim cannot hold there.
   Keep secure file conversion on Linux/macOS; reject Windows file conversion
   before staging or directory creation with an actionable error, retaining
   validated stdout conversion. Document this explicit support boundary and verify
   the platform guard plus Windows/macOS compilation. Native Windows ACL-based
   publication is outside this scoped correction. Also exercise post-publication
   directory-sync failure through the shared fault hook and report that publication
   succeeded, rather than returning an ambiguous sync error.
6. SAFE-04 reader correction: Copilot's object-first unmarshal returns immediately
   on raw arrays and attempts array unmarshal for valid empty object catalogs.
   Accept both existing intended response shapes, including empty lists; apply the
   same entry/page budgets to both and retain malformed-response errors. Add
   raw-array/empty-object and boundary regressions.

Acceptance criteria:

- Every task is completed with actual evidence and no unmet acceptance criteria;
  final gates pass and the record includes an acceptance/status audit.
- Public docs/help match implementation; fixture is cleaned up and no CHANGELOG diff.

Verification: shared final gates plus independent source/evidence review.

Completion evidence:

- Independent final reviewer, not a SAFE-01..04 implementer. Read the entire plan,
  every acceptance criterion and evidence section, repository `AGENTS.md`, and
  `task-as-you-go skill`. Confirmed all four
  dependencies completed before setting SAFE-05 in progress. Inspected the incoming
  tracked/untracked worktree and the completed business-boundary final audit before
  editing; that work includes operator authorization, expiry, ledger retention,
  reload/probe publication, billing attribution, frontend identity boundaries and CI.
- Reviewed actual source, callers, regression bodies and docs, including app client
  registry/reload assembly, every adapter's common execution boundary, health probe
  client, CLI provider switch/readers, stricter Copilot OAuth client, conversion
  validation and filestore/configedit consumers, compressed-inspection dispatch and
  pinned decoder options, listing output/annotations and cancellation. Corrections
  were recorded as requirements 4–6 above **before** implementation.

Corrected findings and reproductions:

1. **Alternate upstream clients bypassed origin isolation.** Provider fallback and
   injected clients reached the prohibited local destination in all 18 cases
   (Anthropic/Gemini/OpenAI × nil/injected client × 302/307/308). Health probes sent
   their bearer credential across a port-only origin change in all three prohibited
   cases, returning success. `upstreamhttp.Do` now shallow-copies the client and
   composes the shared origin/hop check with any stricter callback, preserving
   transport/pool/jar/timeout without mutating shared clients. Provider
   `executeUpstream` and health `probeURL` use it. Production app and CLI constructor
   checks remain present. New tests cover the alternate paths, allowed probe
   redirects, strict callback preservation over local TLS and original-vs-previous
   origin comparison. Existing assembled inference tests still require JSON 502
   `upstream_error` and zero prohibited I/O.
2. **Conversion platform contract overstated Windows guarantees.** Reviewed
   `go-release.json` and Go 1.27.1's `os.Chmod`/`Rename` contracts: Windows 0600 is
   only a read/write attribute, not an owner-only ACL. File conversion now rejects
   unsupported OS targets before staging/parent creation; Linux/macOS retain
   secure atomic publication, and Windows retains validated stdout conversion.
   `TestConvertUnsupportedPlatform` drives the actual conversion flow with Windows
   selected, both force modes and existing/absent destinations, asserting unchanged
   old content, absent new parents, no confirmation, and exact validated stdout.
   CLI help, README, design and website configuration docs state this boundary.
   Native ACL-based Windows publication is not implemented or claimed.
3. **Directory-sync errors did not clearly report completed publication.** The
   single-file writer bypassed its existing directory-sync injection hook and
   returned an ambiguous bare sync error. Both publication modes now use the hook
   and wrap the error as `published ... but sync directory`, preserving error
   identity. The new regression failed before the fix (`err=<nil>` in both modes)
   and now verifies error identity, complete 0600 output and no staging remnants.
4. **Copilot listing rejected supported response shapes.** Raw arrays failed the
   first object unmarshal; valid empty `data` objects failed the subsequent array
   unmarshal. Both shapes now share one output/entry-budget path. Before the fix,
   new empty-object, empty-array, ordinary-array and exact-10,000-array cases failed.
   They now pass with preserved display names; over-budget arrays and malformed
   JSON/field types return nil models and errors. Existing all-provider byte,
   credential, redirect and display tests continue to pass.

- Reproduction command:
  `GOFLAGS=-p=1 go test ./internal/provider ./internal/healthcheck ./internal/filestore ./cmd/aiproxy -run 'Test(ProviderAlternateClientRedirectIsolation|ProbeRedirectIsolation|SingleFileDirectorySyncFailureReportsPublication|CopilotDiscoveryResponseShapes)$' -count=1`.
  The initial provider fixture omitted the OpenAI adapter's required inbound
  request and panicked after reproducing the Anthropic/Gemini leaks; supplied the
  inbound request and reran the provider regression before the source fix, proving
  all 18 prohibited calls. This was a fixture correction, not a product finding.
- After corrections and gofmt, the focused command adding
  `./internal/upstreamhttp` and tests `ConvertUnsupportedPlatform`,
  `DoPreservesClientAndStricterPolicy`, `RedirectChecksOriginalOrigin` passed all
  five packages (provider 0.496s, healthcheck 0.243s, filestore 0.034s, CLI 0.197s,
  shared policy 0.130s). Full suite/race results below supersede targeted coverage.
- Reviewer-changed source: `internal/upstreamhttp/redirect.go`,
  `internal/provider/provider.go`, `internal/healthcheck/healthcheck.go`,
  `internal/filestore/filestore.go`, `cmd/aiproxy/{convert,models_upstream}.go`.
  Regressions: new `internal/provider/redirect_boundary_test.go`,
  `internal/healthcheck/redirect_test.go`, `cmd/aiproxy/models_copilot_shapes_test.go`;
  additions to shared-policy, filestore creation and conversion persistence tests.
  Docs: README, design, website configuration and this task record.

Acceptance/status audit (all criteria checked, including the explicit clarifications):

| Task    | Final status | Independent acceptance audit                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| ------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| SAFE-01 | completed    | Policy compares every hop to `via[0]`, including scheme, case-insensitive hostname and effective port; exact ten-hop success, over-budget chains/loops, host/subdomain/port changes, IPv6/default ports and TLS downgrade are covered. Both API-key headers, bearer/custom headers and 302/307/308 body semantics are exercised by local servers; prohibited destinations receive zero calls. App client construction and all CLI readers use the policy. All adapter operations converge on guarded execution; default/injected provider clients and credential-bearing health probes are now covered too. Pool/header-timeout/stream tests and stricter OAuth tests pass; blocked inference is a controlled error rather than translated success.                                                                                                                                                                                                                                                                                          |
| SAFE-02 | completed    | Config conversion validates before file/stdout output. Same-directory temp creation, exact chmod, file sync/close, Lstat rejection and rename/link publication were inspected. Tests cover 0644 replacement and old open inode, live/dangling links with/without force, directory/FIFO refusal, one winner among sixteen concurrent publishers, late competing files/links/directories, write/sync/publication failures preserving old data, temp cleanup, invalid input and stdout/confirmation errors. Non-force link is atomic no-clobber with no fallback. Post-publication cleanup/sync/confirmation failures explicitly distinguish complete publication. Existing filestore replacement/recovery and configedit consumers pass full race tests. Linux runtime verified; macOS source contract/cross-compilation checked; Windows file output explicitly fails closed with tested control flow and documented stdout support.                                                                                                          |
| SAFE-03 | completed    | Read decoder and pinned zstd option/DecodeAll implementation: one low-memory decoder, 1 MiB window/decoded cap, fixed-capacity destination and preallocation size checks prevent expansion-sized allocation. Metadata parsing checks all repeated fields, 256 joined bytes and four entries before construction. Each gzip/deflate/Brotli read is limit-plus-one; errors/overflow in any layer return original input and false. Zstd tests include advertised 1 TiB/64 MiB window, unknown-size expansions, exact ±1 boundaries, checksum/trailer/trailing corruption and concatenated frames. Chains cover reverse order/repeated fields/four vs five layers and failed inner/outer decoding. Assembled chat/Responses tests prove valid small retry once, unchanged bytes/encoding on rejection, and one call for inapplicable cases. Status/stream/body/pattern/request guards precede decoding; allocation assertions verify skipped work. Five-run benchmark independently reproduced constant allocation across 2/16/64 MiB expansion. |
| SAFE-04 | completed    | Traced per-invocation two-minute context and shared read accounting through all eight provider types; 100 pages, 10,000 pre-filter entries, 8 MiB page and 32 MiB total are inclusive and bounded before additional I/O. Tests verify ±1 limits, duplicates/blank IDs, decoded gzip bytes, limit-plus-one read counts, success/error body closure and no partial CLI output. Anthropic matching-last-id requirements and Gemini empty continuation semantics agree with help/docs. Both providers cover repeated/multi-page cycles, continually unique cursors, query-escaped punctuation/Unicode/CRLF without injected fields, normal multi-page display/config annotations, malformed later pages, cancellation before headers/during/after reads and parent/per-request/whole-list deadlines with exact finite request counts. Copilot arrays and empty objects now share these bounds.                                                                                                                                                   |
| SAFE-05 | completed    | All preceding acceptance/evidence and public contracts audited; scoped corrections have regressions; required final gates and frontend checks pass; fresh isolated DB suites run without skips; owned fixture removed; dirty baseline preserved and CHANGELOG unchanged. No unresolved acceptance blocker.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |

Final verification (repository root; UI/integration build completed before Go
checking; no concurrent suites used the database):

- `pnpm --filter @aiproxy/web-ui test`: **passed**, 9 files / 136 tests, 14.37s.
- `GOFLAGS=-p=1 AIPROXY_TEST_DATABASE_URL=<new fixture URL> make integration`:
  **passed**, including `tsc -b --force`, Vite build (8,757 modules), CGO-disabled
  host binary and hermetic integration tests (2.056s). Typecheck ran as the build's
  required first step; no standalone extra typecheck is claimed.
- Cleared test-result cache with `go clean -testcache`. First `make vet test`
  invocation hit the tool's **120-second execution limit** after provider passed;
  this incomplete invocation was not counted as a passed gate. Reran with a
  600-second tool allowance using the same new fixture and `GOFLAGS=-p=1`:
  **`make vet test test-race` passed** (the Makefile runs all three serial targets).
  Unit DB results from the uncached first run: app 18.678s, dbmerge 0.657s,
  httpapi 19.561s; resumed store 1.048s. Previously completed unit packages were
  cached on the successful rerun, against the same fixture and final source.
  Full uncached race results include CLI 90.054s, app 23.133s, dbmerge 1.326s,
  httpapi 147.929s, store 2.349s, provider 17.537s, healthcheck 1.600s,
  filestore 1.568s, configedit 1.334s, shared policy 1.406s and OAuth 1.394s.
- Explicit DB execution/skip validation, same URL and `GOFLAGS=-p=1`:
  `go test -race -count=1 -json ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app`.
  Parsed the JSON stream with pipefail, counting pass/fail/skip events and failing
  on any skip/failure or missing package. **All four passed, zero skips/failures**:
  store 6 test/subtest pass events (2.401s), dbmerge 6 (1.410s), httpapi 434
  (161.202s), app 99 (27.382s). Read their prerequisite helpers: absent URL skips;
  connection/migration errors are fatal. This run includes business-boundary
  regressions, not only SAFE changes. The separate webui stub-without-assets test
  may intentionally skip after a UI build; it is not a DB prerequisite test.
- `GOFLAGS=-p=1 GOOS=windows GOARCH=amd64 go test -c -o <repo-root>/tmp/safe05-7c9a-windows.test.exe ./cmd/aiproxy`
  and the corresponding `GOOS=darwin GOARCH=arm64` command with output
  `<repo-root>/tmp/safe05-7c9a-darwin.test`: **passed**. These are compilation checks,
  not native Windows/macOS runtime executions.
- `GOFLAGS=-p=1 go test ./internal/provider -run '^$' -bench '^BenchmarkInspectionZstdBounded$' -benchmem -count=5 -cpu=1`:
  **passed**, 47.883s, Go 1.27.1 linux/amd64 on Intel i9-13950HX. Every one of five
  samples for each 2/16/64 MiB unknown-size expansion allocated **1,344,908 B/op,
  22 allocs/op**; small valid **1,049,992 B/op, 11 allocs/op**; advertised 1 TiB and
  64 MiB window each **1,049,912 B/op, 10 allocs/op**. Timing ranges respectively:
  963274–1283444, 966507–1538158, 1030030–1238538, 199512–524667,
  182182–315728 and 244238–933407 ns/op. This corroborates bounded allocation,
  not a speed claim. Decoder bookkeeping, other codec windows, original response
  buffers and concurrent requests remain additional costs; 1 MiB is not a total
  process or all-codec heap ceiling.
- `make docs-contract`: **passed**, matrices match. `git diff --check` and
  `git diff --exit-code HEAD -- CHANGELOG.md`: **passed**, including after cleanup.
  Staged diff is empty. Changed Go files were gofmt'd.

Disposable fixture and preservation:

- Inspected `docker version`, all existing containers, README's documented isolated
  PostgreSQL 17 command, sandbox documentation and Makefile before creating a new
  fixture. Did not use the running developer PostgreSQL or the prior-review DB.
- Created only `aiproxy-safe05-20260926-final-7c9a` (container ID
  `a31505013d51c9abacbe6947c8e102c10987c1ad5c373c93f676356fc625c229`), using
  `docker run --detach --rm`, image `postgres:17`, unique DB
  `safe05_review_7c9a`, user `safe05_review`, synthetic password
  `safe05_fixture_only`, loopback dynamic port mapping and documented pg_isready
  healthcheck. Both Docker health and `pg_isready` passed before tests. All DB gates
  explicitly used `postgres://safe05_review:safe05_fixture_only@127.0.0.1:38582/safe05_review_7c9a?sslmode=disable`. // pragma: allowlist secret
- Before disposal, container psql confirmed the intended database,
  `public.schema_migrations`, and zero leftover `ledger_*`/`boundary04_*` schemas.
  `docker stop aiproxy-safe05-20260926-final-7c9a` succeeded. The immediate list
  briefly showed asynchronous auto-removal; a subsequent exact-name `docker ps -a`
  check returned **no container**, confirming cleanup. No other fixture/container
  was stopped, reused or removed.
- Removed only this integration build's new untracked
  `internal/webui/dist/.gitkeep` with apply_patch. Ignored generated UI/host-binary
  outputs and the two explicitly named temporary cross-compilation artifacts are
  build products. Final tracked/untracked status retains all incoming business-
  boundary and SAFE-01..04 files, plus the scoped reviewer regressions. Shared
  healthcheck edits preserve the prior cancellation/publication locking correction;
  app reload/resource work and frontend/auth/accounting/store/CI changes survive.
  No CHANGELOG edits, commits or subagents.
- Limitations: local stub upstreams and Linux filesystem/runtime only; no hosted CI,
  real-provider, native Windows/macOS execution, ACL implementation, deployment or
  exhaustive dependency audit. Parent-directory traversal assumes trusted paths;
  Lstat checks are not a filesystem sandbox. Forced rename atomically replaces the
  directory entry rather than following a raced-in symlink; atomic no-clobber is
  specifically the non-force contract. Filesystems must support the documented
  POSIX permission/link/rename semantics. Post-publication errors cannot roll back
  an already visible complete file. These limitations are explicit and do not
  leave an unmet criterion under the clarified platform contract.
- Final status count: **5 completed; 0 pending, in_progress, blocked, deferred or
  cancelled**. All five tasks have acceptance and actual verification evidence.

## Coordinator Final Closure

- Reviewed the entire completed record after SAFE-05 returned, including all
  requirements, scoped clarifications, acceptance audits, gate results and fixture
  cleanup evidence. All five task statuses and Completion evidence are present;
  no execution blocker remains. Rechecked worktree, `git diff --check`, and
  `git diff --exit-code -- CHANGELOG.md` successfully.
- Fresh sub-agent sessions ran sequentially, each returning before the next began:
  SAFE-01 `ses_f1febde0dffeHhYE270JoAM6HB`;
  SAFE-02 `ses_f1fe6fa83ffejZGGRWdgY0n7SS`;
  SAFE-03 `ses_f1fe2a5b5ffet56qNVN9lRKckN`;
  SAFE-04 `ses_f1fdba1d9ffeNYPthzDpcEYoJE`;
  independent SAFE-05 `ses_f1fd1cc6dffe79ZNLyeqd0kFv7`.
- Completion includes the explicit Windows file-conversion support change and
  recorded audit limitations; it does not claim an exhaustive product audit.

Follow-up review: `20260926-173216-admin-lifecycle-integrity.md` tracks the next
separately scoped phase: invitation, membership/offboarding and key-update integrity.
