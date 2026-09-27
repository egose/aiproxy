# CLI Dashboard Input And Visible-Selection Boundaries

Created: 2026-09-27 01:06:07 local time

## Objective And Scope

Continue the completed CLI dashboard operator-experience work with three residual
correctness fixes. Operator metadata must stay small enough for routine polling,
untrusted text must not masquerade as terminal rows, and take-once captures must
only open from a visibly selected row. Preserve the improved interactions and
all incoming dirty BOUNDARY/SAFE/LIFE/FLOW/TUI work. **Do not edit `CHANGELOG.md`**
or commit. No unrelated product redesign.

## Analysis And Deduplication

- Followed AGENTS.md and
  `task-as-you-go skill`.
- Read-only review session `ses_f1e1bf220ffesJtyfwIEaNgPGE` inspected lifecycle,
  metadata production/retention/RPC, recent request search/rendering, block list/
  detail transitions, final TUI acceptance evidence and relevant tests. Coordinator
  independently inspected the principal findings and current dirty worktree.
- Related completed plan: `20260926-213604-cli-dashboard-operator-experience.md`.
  TUI-06 bounded recent entry count but not entry bytes; TUI-07 escaped captured
  inspection text but not Requests rows. TUI-08 fixed the payload filter transition,
  not the Blocks refresh/error sibling. These are new residual boundaries.
- Findings are source-confirmed; no baseline tests/experiments run in this review.
  Implementers must reproduce locally. The 200 × near-8-MiB model retention scenario
  is a theoretical source-derived bound, not measured heap usage; use small safe
  experiments rather than attempting exhaustion.
- Review found no further compelling issue in inspected rate/cost, provider detail,
  generation cancellation or reconnect code. This is not a full security/terminal/
  production-load audit or fresh certification of those subsystems.

## Execution And Verification

Run EDGE-01 through EDGE-04 **sequentially**, each in a **fresh sub-agent session**;
EDGE-04 is an independent reviewer. No nested agents. Each owner sets only their
task `in_progress`, records necessary scope additions before edits, and appends
`Completion evidence` with paths, reproduction, actual commands/results and limits.
Mark completed only after acceptance and required checks pass; otherwise identify
the exact blocker. Use apply_patch and local source/module conventions.

P1 = material resource/availability risk; P2 = interactive operator correctness.
Keep metadata bounding at the shared retention boundary and literal rendering at
the presentation boundary; routing/billing identities must not silently change.
Preserve additive RPC compatibility and existing web consumer behavior.

All commands run at `<repo-root>`. Required task-focused race
checks are below; also run `make vet test` for nontrivial changes, docs-contract
when docs change and `git diff --check`. Finish UI builds before Go asset readers.
Optional DB skips during non-DB implementation checks must be disclosed.

Final gates: `make vet test`, `make test-race`, `make integration`,
`make docs-contract`, `pnpm --filter @aiproxy/web-ui test`,
`pnpm --filter @aiproxy/web-ui typecheck`, and `git diff --check`. Final reviewer
creates an OWN NEW disposable PostgreSQL 17 fixture per README, after inspecting
Docker: unique name/DB, loopback/readiness, explicit AIPROXY_TEST_DATABASE_URL,
GOFLAGS=-p=1 for serial DB packages. Prove all four DB packages run without
prerequisite skips and remove only the owned fixture. Never use developer/prior
fixtures. Local synthetic HTTP/PTY fixtures only, no real provider credentials.

Definition of done: four completed tasks, actual evidence and independent
acceptance audit, passing final gates, bounded/literal metadata and visible-action
contracts agree with docs, owned fixtures removed, baseline/CHANGELOG preserved.

### Task EDGE-01: Bound Recent Completion Metadata Before Retention

Status: completed

Kind: defect

Priority: P1 — caller-controlled rejected models can retain and retransmit near-body-sized text.

Suggested agent: bounded accounting/RPC metadata implementer

Dependencies: none

Primary ownership: `internal/accounting` recent-event retention, additive RPC/TUI
metadata flags, focused actual HTTP→accounting→RPC tests and docs; frontend schema
updates only if necessary.

Finding: extractModel decodes an entire model string from up to 8 MiB input. Failed
resolution uses a bounded accounting sentinel but deferred Event.PublicModel retains
the original string. The 200-entry ring stores it unchanged and snapshots resend it.
Entry count alone does not establish a reasonable metadata memory/transport bound.

References: `internal/httpapi/handler.go` (ServeHTTP deferred accounting/extractModel),
`internal/accounting/accounting.go` (Event, Aggregator.Record, ringBuffer.push),
`internal/dashrpc/dashrpc.go` (BuildContext recent serialization),
`internal/accounting/recent_identity_test.go` (short-field count/cardinality coverage).

Requirements:

1. Establish named, documented per-field/entry byte budgets for recent completion
   metadata at shared retention. Bound all relevant retained string fields and
   detach bounded copies from oversized backing strings. Preserve useful UTF-8
   prefixes/diagnostics and expose explicit field truncation metadata.
2. Keep full routing and billing/grouping identity semantics unchanged; truncate
   only the diagnostic retained copy. Ordinary identifiers remain exact. Truncated
   IDs must not be used as exact correlation keys. Preserve rate/P95/count contracts.
3. Carry truncation status through RPC to the TUI with truthful list/detail guidance
   and safe old-snapshot fallback. Bound the serialized recent component; do not
   claim a new total-heap or total-catalog snapshot limit.

Acceptance criteria:

- Real rejected-request fixtures with large models, payload logging off and ordinary
  request logs suppressed, verify retained fields/entry bytes and serialized recent
  size remain bounded. Use practical fixture sizes, not exhaustion tests.
- Boundary/Unicode/copy-detachment tests verify exact ordinary values, truncation
  indicators, no long backing-allocation retention, ring eviction and unchanged
  billing/rate cardinality/data. A meaningful allocation/retention experiment backs
  the memory claim rather than only an output-length assertion.
- RPC→TUI and affected web schema tests preserve flags/old snapshots and display
  explicit truncation without making false exact-ID correlations.

Verification: `go test -race ./internal/accounting ./internal/dashrpc ./internal/dashboard ./internal/httpapi`,
frontend tests/typecheck if schema changed, default/final gates.

Necessary scoped findings (recorded before implementation):

- This fresh sequential EDGE-01 session has no dependencies; shared rules, both
  task-as-you-go instructions, AGENTS.md and completed TUI measurement, selection,
  recent/search and correlation contracts were read. Incoming dirty work is extensive.
- `ringBuffer.push` is the shared production diagnostic retention boundary, after
  `Aggregator.Record` updates exact billing/provider/upstream/rate aggregates.
  Bound all eight Event strings there, including short slices of large backing
  allocations. MemoryRecorder is an exact event recorder used for accounting/tests,
  not the production diagnostic ring; changing its retained events would change billing.
- `Event` is comparable and Requests anchor by its whole value. Use comparable
  field flags and a ring sequence to distinguish completions whose diagnostic
  prefixes otherwise collide, including identical timestamps and caller-reused IDs.
- P95 currently groups recent events with EventProvider. Clipping Provider/Model
  could merge/change groups. Carry a numeric provider identity from the existing
  exact provider aggregate into recent events and provider summaries; the TUI must
  resolve that identity for P95 rather than treating truncated prefixes as keys.
- Add optional compatible RPC/web fields, explicit TUI list/detail guidance and
  a hard correlation gate for truncated request IDs. Old snapshots retain their
  existing fallback. Rendering controls/multiline boundaries remain EDGE-02.
- Verify source ownership with live backing-address range checks (including
  under-budget substrings), bounded allocation measurements and small rejected
  HTTP model fixtures with request logging/payload logging disabled. No total
  process/catalog heap bound follows from this diagnostic-ring change.

Completion evidence:

- Changed: `internal/accounting/accounting.go`, new `recent.go` and
  `recent_bounds_test.go`; `internal/dashrpc/dashrpc.go`, new
  `recent_bounds_test.go`; dashboard `dashboard.go`, `recent.go`, `correlation.go`,
  `layout.go`, new `recent_bounds_test.go`; `web-ui/src/types.ts` and
  `types.test.ts`; `docs/design.md` (Dashboard Recent Completions And Correlation),
  `website/docs/operations.md` (Dashboard Requests And Search), and this record.
- The existing ring now clones all eight diagnostic strings at push, after full
  aggregation: RequestID/Tenant/Client/Provider 256 bytes each;
  PublicModel/Model/UpstreamModel 512 each; Operation 64. Valid UTF-8 prefixes end
  on complete code points; ordinary values remain byte-exact. Total string content
  is <=2,624 bytes/entry and <=524,800 bytes/200 entries, excluding fixed storage
  and allocator overhead. Even a short slice of a large source is cloned. No
  truncation of the event sent to billing, upstream, provider or rate aggregation.
- Optional comparable `Truncated` boolean fields round-trip through RPC/schema.
  Requests displays `[truncated]` before row metadata; detail lists every shortened
  field and retained-text-only search guidance. Truncated RequestID blocks both l/v
  without commands/state navigation and suppresses their action hints; intact IDs
  still correlate when other fields truncate. Old missing fields keep prior fallback.
- `RecentSequence` distinguishes otherwise identical bounded completions for whole-
  Event selection; `ProviderID` joins the existing exact-name provider summaries for
  P95. Both are optional uint64 decimal strings on the wire. No hashes/prefixes are
  substituted for exact provider identity. Read recent before provider summaries so
  concurrent new provider admission cannot emit an unresolvable ID. Rates, durations,
  token/cache data, full tenant/client/model/operation/status billing identities,
  upstream attribution and lifetime counters retain their existing semantics.
- Reproduction/regressions: actual loopback HTTP App fixture sends 205 rejected
  models (204 approximately 32 KiB, one approximately 1 MiB), with `access_log=false`,
  `level=error`, payload logging absent. Authenticated HTTP snapshot confirms zero
  logs, payload disabled, exactly 200 completions (IDs 5–204), bounded PublicModel,
  no fabricated resolved target, and one `_unresolved_model` billing group/count 205.
  Recent JSON measured **189,003 bytes**; transported TUI list/detail exposes flags.
  The first package run caught a fixture assumption of `_model_not_found`; corrected
  the assertion to the actual existing `_unresolved_model` sentinel, without changing
  production rejection/grouping behavior. No pre-fix full baseline suite was run.
- Ownership/retention experiment: 64 field/boundary/Unicode cases check live backing
  address ranges using `unsafe.StringData` plus `runtime.KeepAlive`, including empty,
  ordinary short slices of 64-KiB parents, cap-1/cap/cap+1 and CJK/emoji boundaries.
  The slice-only control shares its source allocation; all retained nonempty strings
  lie outside that live allocation. Benchmark-driven ring pushes with **32,768** and
  **1,048,576** byte sources both measured **2,624 B/op, 8 allocs/op**. All 200 retained
  entries are checked against the source backing range. This proves owned bounded
  copies rather than relying only on lengths or a noisy process-heap delta; it does
  not measure production peak heap or claim catalog/aggregate/log memory bounds.
- Worst escaping experiment: 200 entries with all eight oversized NUL-containing
  fields serialize to **3,241,293 bytes** in the ring test and **3,249,095 bytes** in
  the full RPC fixture with timing/identity/counters. Conservative recent-array bound
  is **3,353,802 bytes** (`2+200*(6*2624+1024+1)`), independent of input text lengths.
  Other snapshot components and old-server data are outside this bound.
- Additional regressions cover all eight flags and full aggregate identity through
  JSON; ordinary values/omitted flags and old decoder compatibility; two full groups
  sharing every diagnostic prefix; exact cache/token/rate data; ring eviction and
  immutable flag snapshots; explicit and Model-derived oversized provider P95 groups;
  no false P95 prefix group if identity mapping is unavailable; selected identical
  prefixes across refresh; no truncated-ID correlation; intact-ID correlation with
  shortened models; old-snapshot Model fallback and old intact-ID correlation.
- Passed focused verbose experiments:
  `go test -v ./internal/accounting ./internal/dashrpc ./internal/dashboard -run
'TestRecent(FieldBudgets|BoundedAllocation|BoundsPreserve|WorstCase|RPCIdentity|SequenceAnchors|P95Derived|TransportBounds)|TestRejectedLargeModelHTTPRecentBounds' -count=1`.
  Passed required `go test -race ./internal/accounting ./internal/dashrpc
./internal/dashboard ./internal/httpapi` (6.279s / 2.375s / 45.343s / 15.124s),
  `make vet test`, `make docs-contract`, and `git diff --check`.
  Passed `pnpm --filter @aiproxy/web-ui test` (**11 files, 196 tests**) and
  `pnpm --filter @aiproxy/web-ui typecheck`, before Go checks. No asset build overlap.
- Limits/skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional DB prerequisites
  in app/dbmerge/httpapi/store remain skipped. No DB fixture, live provider or real
  credentials used; local HTTP/temp fixtures clean up through test lifetimes. The
  independent EDGE-04 reviewer owns fresh PG17/no-skip, final full-repo race,
  integration/build and PTY gates. No EDGE-01 blocker. Incoming dirty work and all
  other task statuses preserved; CHANGELOG untouched; no commits or subagents.

Next-agent rendering/correlation contract:

- `accounting.RecentTruncation` is a fixed comparable bool struct; optional wire
  `Truncated` uses the same eight capitalized Event field names. Do not replace it
  with a slice/map/pointer that changes whole-Event identity semantics. `Fields()`
  returns the deterministic label order. `RecentSequence` is aggregator-local
  completion identity only, never an external request/correlation key; IDs are
  decimal strings on the wire and exact uint64 values in Go.
- EDGE-02 must render literal single-row values at presentation, keeping stored
  prefixes/search values unmodified. Preserve `[truncated]` before row metadata,
  detail's per-field notice, and the truncated RequestID gate/hints. An intact ID
  remains eligible for exact correlation despite truncated model/client/provider
  text. Missing flags mean no reported truncation, not an old-server size guarantee.
- `p95WithSamples(recent, providerStats)` resolves `ProviderID` to exact full names.
  Keep recent-before-stats read order; registry IDs last for the aggregator lifetime.
  Legacy untruncated values use EventProvider. Never correlate/group by a shortened
  prefix when a required ID mapping is missing. Full billing/usage keys and rate
  counters are independent of this diagnostic copy and must stay exact.

### Task EDGE-02: Render Request Metadata As Literal Single-Row Data

Status: completed

Kind: defect

Priority: P2 — embedded newlines can hide selection and make Enter open an unseen event.

Suggested agent: terminal metadata rendering implementer

Dependencies: EDGE-01

Primary ownership: `internal/dashboard/recent.go`, focused shared literal-text helper
if needed, request metadata detail/correlation rendering and regression tests/docs.

Finding: renderRequests concatenates PublicModel and other metadata into a purported
single row before frame rendering. fitView splits on newlines before escaping, so
embedded lines consume physical viewport rows while cursor/Enter still use event
indices. ANSI-like metadata may also be interpreted as display styling rather than
literal text. A tiny rejected model string suffices, independent of EDGE-01 caps.

References: `internal/dashboard/recent.go` (renderRequests, metadataLines,
handleRequestKey), dashboard.go (fitView/logOneLine), layout.go (paneFrame.Render),
`internal/dashboard/search_test.go`, `layout_test.go` and `recent_http_test.go`.

Requirements:

1. Escape/normalize data-owned line/control characters before composing list rows;
   every request event occupies one logical summary row. Keep metadata literal in
   detail, preserving legitimate Unicode and application-owned formatting.
2. Apply consistently to request ID, model, provider, operation, client/tenant and
   derived correlation labels. Do not mutate stored routing/search identities or
   conflate literal backslash text with actual control bytes.
3. Preserve visible selection and exact Enter identity through refresh, filtering,
   search and compact/large layouts. Retain EDGE-01 truncation indications.

Acceptance criteria:

- Real rejected-request fixture includes newline/CR/tab/ESC, plus printable Unicode
  and literal escape-like text. 80x12/80x24/120x30 and resize tests assert selected
  identity/marker are visible and Enter opens that exact visible event.
- No data-owned ANSI/control sequences reach raw terminal output as commands/styles;
  test raw bytes as well as screen dimensions. Long escaped text is safely clipped
  in rows and inspectable in detail within retained limits.
- Search continues matching actual retained metadata and existing contextual keys,
  help/footer/correlation behavior remains intact.

Verification: `go test -race ./internal/dashboard ./internal/dashrpc`, focused
actual HTTP-to-render test, default/final gates.

Necessary scoped findings (recorded before implementation):

- Fresh sequential EDGE-02 session; EDGE-01 is completed. Read shared rules, both
  task-as-you-go instructions, AGENTS.md, and prior TUI layout/input/search/identity
  conventions plus EDGE-01's comparable flags/sequence/P95/correlation contract.
- `renderRequests` inserts raw fields before `paneFrame.Render`/`fitView` split
  lines. Escape each data field before composition, never the styled final view.
  Escape actual controls and double literal backslashes so the two stay distinct;
  retain printable Unicode, combining text and emoji joiners.
- `metadataLines` shares request and usage identity detail; its raw model/operation
  sibling needs the same literal boundary. `searchLabel` embeds raw correlated IDs.
  `payloadRow` embeds the same PublicModel/provider fallback before clipping, and
  payload detail embeds the raw ID into its title/body. These evidenced sibling
  metadata paths are included; captured-body inspection keeps its multiline contract.
- Use an explicit 512-source-byte per-value presentation cap with a visible display
  clipping notice for oversized legacy/sibling values. This accommodates every
  EDGE-01 retained field without further loss, bounds escape expansion, and keeps
  full retained request detail scrollable. Existing per-field retention notices and
  truncated-ID correlation gating remain authoritative and separate.
- Add a real loopback rejected HTTP request → authenticated snapshot → rendered
  model fixture with actual controls, Unicode and literal escape-like text; test
  raw output against a captured application-owned escape allowlist as well as
  viewport dimensions, visible selected row and exact Enter identity. Exercise
  compact/normal/resize/refresh/search/filter paths and retained/detail bounds.

Completion evidence:

- Changed: new `internal/dashboard/metadata.go` and `recent_literal_test.go`;
  dashboard `recent.go`, `search.go`, `payload.go`; `docs/design.md` (Dashboard
  Recent Completions And Correlation), `website/docs/operations.md` (Dashboard
  Requests And Search), and this EDGE-02 record. No accounting/RPC/schema changes.
- Reproduced before implementation with
  `go test ./internal/dashboard -run '^TestRejectedControlModelHTTPRenderSelection$'
-count=1`: the real rejected HTTP fixture failed its raw-output assertion with
  data-owned `ESC[38;2;13;17;19m`, erase-screen `ESC[2J`, OSC 52, BEL, NUL, DEL
  and C1 CSI in the rendered viewport. This was an actual terminal-byte failure,
  not only a strip-ANSI screen comparison. No full pre-fix baseline suite was run.
- `metadataText` escapes LF/CR/tab/ESC, all C0/C1 controls, Unicode line/paragraph
  separators and invalid UTF-8 bytes, doubles literal backslashes and escapes quotes.
  Printable Unicode, CJK, combining accents and emoji ZWJ sequences remain intact.
  Values are escaped before Requests row composition and detail wrapping; UI-owned
  newlines, borders, selection styling and footer codes remain separate and intact.
- Named `metadataDisplayBytes=512` bounds each source value processed, preserving
  complete code points; worst escaped value is 2,048 bytes plus the explicit
  `[display clipped at 512 bytes]` suffix. Tests cover cap/cap+1, split CJK boundaries,
  invalid bytes, every C0/C1 control and a 1-MiB NUL input. Every EDGE-01 retained
  field fits without further clipping. A 712-byte submitted model retained at 512
  bytes (506 NULs plus `END界`) keeps `[truncated]`, field guidance and its entire
  escaped retained tail, visibly reachable by scrolling detail at 80x12.
- The shared Usage identity detail, correlation/filter labels, payload summary
  cells/width measurement (including provider/model fallback), and payload ID titles
  use the same presentation boundary. Captured payload bodies retain their existing
  bounded multiline inspection. Legacy oversized request values explicitly disclose
  display clipping; it never changes stored flags or the exact underlying identity.
- Production fixture sends **21 real loopback HTTP POSTs**, all rejected with 404,
  then reads bearer-authenticated HTTP snapshots. Models contain actual LF/CR/tab,
  ESC color/reset/erase/OSC, BEL/NUL/DEL/C1 plus CJK/emoji/combining text and literal
  backslash escape-like strings. Raw PublicModel survives RPC byte-exact, with no
  fabricated target, no retention truncation, zero logs and payload logging disabled.
  Mounted KeyPress tests exercise Home/Down/PageDown/End/Up, 80x12/80x24/120x30/
  160x48 and resize back, real refreshed snapshot anchoring, combined search/error
  filters and clear. Every visible event occupies its expected row; the selected
  row's marker/time/status/ID are visible, and Enter opens its exact whole Event.
- Raw-byte assertions parse only SGR sequences captured from clean application
  renders (including owned inactive-pane and log-level styling); all other ESC,
  non-newline controls and invalid bytes fail. Screen assertions additionally check
  exact terminal dimensions, row-specific selection marker, every expected visible
  ID and footer help/back/quit/live status. Test harness corrections distinguished
  the tab-strip caret from row selection and captured legitimate clean-view styles;
  the injected color/erase/OSC assertions remained strict.
- Eight field regressions cover request ID, tenant/client, public/legacy model,
  provider, resolved model and operation, with literal detail inspection and unchanged
  raw stored/selected Events. Search tests distinguish actual LF/ESC from literal
  `\n`/`\x1b` data. Correlation tests reject literal-lookalike ID decoys in both
  Logs and Payloads, fetch payload detail with the original control-containing ID,
  preserve body newlines, and return to the exact original request. Existing EDGE-01
  sequence/P95/truncation/correlation gates and prior layout/input suites pass.
- Passed focused verbose verification (five top-level tests, eight field subtests):
  `go test -v ./internal/dashboard -run
'Test(RejectedControlModelHTTPRenderSelection|MetadataLiteralEncodingAndBounds|RequestMetadataAllFieldsLiteralAndInspectable|LiteralMetadataSearchAndExactCorrelation|LiteralSiblingMetadataPresentation)$'
-count=1` (**0.512s**). Also passed `go test ./internal/dashboard ./internal/dashrpc`,
  required `go test -race ./internal/dashboard ./internal/dashrpc` (**37.113s** /
  cached), `make vet test`, `make docs-contract`, `git diff --check`; `gofmt -d`
  on all five touched Go files produced no diff.
- Limits/skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional DB prerequisites
  in app/dbmerge/httpapi/store remain skipped. No DB fixture, real provider/credentials,
  frontend build or PTY experiment was used. Local HTTP/temp fixtures clean up through
  test lifetimes. EDGE-04 retains fresh owned PG17/no-skip verification, full-repo
  race/integration/frontend and real-binary PTY gates. No EDGE-02 blocker. Incoming
  dirty work and all other task statuses preserved; CHANGELOG untouched; no commits
  or subagents.

Next-agent literal metadata contract:

- `metadataText` accepts a raw value, not styled/composed text. Apply it once before
  row composition, cell measurement/truncation or wrapping. It is deliberately not
  idempotent: an existing literal backslash must become two display backslashes.
  Never pass its output into search, identity/selection anchors, RPC or fetch keys.
- The 512-byte display cap is independent of EDGE-01 retention flags. It preserves
  all new retained fields and discloses clipping of larger legacy/sibling values.
  `[truncated]`, per-field notices, comparable Event/RecentSequence and the truncated
  RequestID gate remain unchanged; a display-clipped but intact raw ID still uses
  exact correlation. Raw search values remain the source of matches.
- Keep `inspectionText`'s multiline captured-body behavior separate from metadata
  encoding. Payload titles are encoded metadata; payload bodies are inspection text.
  Do not escape a final frame or strip all ANSI: application-owned styling is valid.
- EDGE-03 still owns hidden block-list action gating; no Blocks handler/state edits
  were made here. Preserve the existing input/selection/lifetime boundaries when
  implementing that task. EDGE-04 should reproduce the HTTP/raw-render regression
  and independently exercise the same metadata through its real-binary PTY gate.

### Task EDGE-03: Prevent Take-Once Reads From Hidden Block Lists

Status: completed

Kind: defect

Priority: P2 — Enter during refresh/error can consume an invisible selected capture.

Suggested agent: block visibility/action-state implementer

Dependencies: EDGE-02

Primary ownership: `internal/dashboard/blocks.go`, block/state regression tests,
operator help/docs where feedback needs clarification.

Finding: manual refresh sets blockKnown=false but retains cached rows/cursor; failed
refresh preserves rows while the view shows only an error. Enter checks cached
length but not list visibility/known/error status, so arrows plus Enter can consume
a hidden row via the take-once endpoint.

References: blocks.go (handleBlockKey, applyBlockList, renderBlocks),
`internal/httpapi/dashboard.go` (writeDashboardBlock/TakeBlock), block/state tests.

Necessary scoped findings (recorded before implementation):

- Fresh isolated sequential EDGE-03 session; EDGE-01 and EDGE-02 are completed.
  Read shared rules, both task-as-you-go instructions, AGENTS.md, EDGE-01
  retention/correlation contract and EDGE-02 literal metadata/next-agent contract.
  Incoming dirty worktree is extensive and preserved; no CHANGELOG, commits, subagents.
- `renderBlocks` hides cached rows whenever `blockErr != ""` or `!blockKnown`
  (loading/disabled/unavailable), while `handleBlockKey` Enter only checks cached
  length/fetcher/pending, and `moveBlockCursor`/`blockCursorTop`/`Bottom` mutate
  cached cursor unconditionally. `scrollFocused`/`scrollTop`/`Bottom` in
  `dashboard.go` reach the same cursor mutators, so the visibility gate must live
  in a shared `blockListVisible()` predicate plus Enter gating, not only in one
  key branch. Tick background refresh (`blockKnown && err == "" && !loading`)
  keeps rows visible and must stay actionable.
- Sibling check: payload Enter already gates `!payloadKnown`/paused/pending but
  still permits detail fetch while `payloadErr != ""` with stale `payloadKnown`;
  that fetch is non-consumptive (`GetPayload` never takes), and requests/logs/
  usage/provider/alias Enter paths are local-only. No equivalent take-once
  consumption gap evidenced, so no sibling behavior change; payload pending-filter
  behavior stays fixed.
- Footer in `layout.go` offers `[j/k] move [enter] detail` for Blocks browse even
  while the list is hidden; error/loading rows lack an explicit `[r] retry` cue
  beyond the title. Plan: gate Enter/navigation on `blockListVisible()`, keep
  identity reconciliation/generation/pause/cancellation/decision state untouched,
  add explicit loading/error footer without Enter, and suffix error/loading rows
  with `[r] retry`.
- Tests: extend `blocks_test.go` with a counted consuming take-once fixture
  (GetBlock counts and consumes on success) proving zero detail RPCs and zero
  consumption across held manual refresh and list-error hiding, then exact single
  recovery open of the visible ID.

Requirements:

1. Never issue a take-once GET without a visibly selected block. Gate opening during
   hidden unknown/loading/failed states, with clear retry/loading feedback; preserve
   normal background refresh when cached rows really remain visible.
2. Keep stable identity reconciliation, generation guards, pause/cancellation and
   selected-finding decision state. Ensure navigation cannot silently change a
   hidden selection then act on it.
3. Check sibling actionable lists against the same visibility principle while
   preserving the already-fixed payload pending-filter behavior; scoped correction
   only if an equivalent acceptance gap is evidenced.

Acceptance criteria:

- Mounted keys with multiple captures: initial success → held manual refresh or
  list error → arrows/Enter produce zero detail RPCs and zero consumption while
  rows are hidden. Recovery opens exactly the visible selected ID once.
- Existing visible background-refresh, pause, cancellation, stale-result and
  per-finding decision tests pass. Error/loading footer does not falsely offer
  an immediately available take-once action.
- A local transport/take-once fixture or equivalent counted store proves capture
  availability survives rejected UI actions, not merely that detail is hidden.

Verification: `go test -race ./internal/dashboard ./internal/dashrpc ./internal/httpapi`,
default/final gates.

Completion evidence:

- Changed: `internal/dashboard/blocks.go` (new `blockListVisible()` predicate:
  fetcher non-nil, `blockKnown`, empty `blockErr`, non-empty rows; Enter gated to
  `return true, nil` while hidden so zero `GetBlock` RPCs; `moveBlockCursor`/
  `blockCursorTop`/`Bottom` return false while hidden, which also neutralizes the
  `scrollFocused`/`scrollTop`/`Bottom` fallback paths; error row now
  `fetch failed: <truncated> · [r] retry` and loading row `loading… · [r] retry`),
  `internal/dashboard/layout.go` (browse-mode Blocks footer while hidden shows
  `loading… · [r] retry` / `fetch failed · [r] retry` / `block viewer unavailable` /
  `[r] refresh`, or `PAUSED [p] resume` when paused — never `[enter]`), new
  `internal/dashboard/blocks_visibility_test.go` (counted consuming take-once
  fixture), `internal/dashboard/dashboard.go` help line clarifying Enter works
  only from a visible row, and this EDGE-03 record. No accounting/RPC/schema
  changes; EDGE-01 flags/sequence/P95 and EDGE-02 literal boundaries untouched.
- Sibling check: payload Enter already gates unknown/paused/pending and its
  detail fetch is non-consumptive; requests/logs/usage/provider/alias Enter paths
  are local-only. No equivalent take-once gap evidenced, so no sibling behavior
  change; payload pending-filter behavior preserved.
- Reproduction/regressions: `TestHiddenBlockListIgnoresTakeOnceActions` (2 captures,
  visible `j` selects 2nd ID, held manual `r` hides list, `j/k/down/up/G/g/enter`
  while hidden issue zero cmds, `getCalls` stays 0, both captures survive, cursor/
  pending/detail unchanged, loading view/footer show retry without enter, recovery
  list restores exact cursor/ID, Enter issues exactly one `GetBlock` for that ID
  and consumes only it); `TestFailedBlockListIgnoresTakeOnceActions` (same while
  `blockErr` set with rows retained, retry recovers exact ID once);
  `TestBackgroundBlockRefreshStaysActionable` (in-flight background list request
  keeps `blockListVisible()` true, Enter stays actionable, list result applies
  cleanly). Pre-fix Enter checked only cached length, so hidden Enter/arrows would
  have issued the RPC and moved the cursor — the new zero-RPC/unchanged-cursor
  assertions target exactly that gap.
- Passed: new focused tests
  `go test ./internal/dashboard -run
'Test(HiddenBlockListIgnoresTakeOnceActions|FailedBlockListIgnoresTakeOnceActions|BackgroundBlockRefreshStaysActionable)'
-count=1 -v` (3/3 PASS); required
  `go test -race ./internal/dashboard ./internal/dashrpc ./internal/httpapi`
  (dashboard 34.984s ok, others cached ok); `go vet ./...` clean;
  `make test` (all packages ok, dashboard 4.129s); `make build` ok
  (`dist/aiproxy` built); `make docs-contract` (matrices match);
  `git diff --check` clean; `gofmt` clean on touched files.
- Limits/skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional DB
  prerequisites remain skipped in non-DB checks. No DB fixture, live provider,
  credentials, frontend build or PTY experiment used. EDGE-04 owns fresh PG17/
  no-skip verification, full-repo race, integration/frontend and real-binary PTY
  gates. No EDGE-03 blocker. All incoming dirty baseline and other task statuses
  preserved; CHANGELOG untouched; no commits or subagents.

### Task EDGE-04: Independently Verify Metadata And Visible-Action Integration

Status: completed

Kind: improvement

Priority: P1 — validate resource, data-presentation and take-once boundaries together.

Suggested agent: independent final reviewer, not an EDGE-01–03 implementer

Dependencies: EDGE-01, EDGE-02, EDGE-03

Primary ownership: review all changes and task evidence, scoped corrections, this record.

Finding: entry caps and outer viewport bounds do not by themselves establish bounded
retained memory, literal data rendering or safe visible-selection actions.

References: preceding findings, source/tests and Completion evidence.

Requirements:

1. Audit actual producer→retention→RPC→render/correlation paths, copy ownership,
   cap/truncation semantics, billing independence, raw terminal bytes and action
   visibility. Verify every acceptance criterion; add scoped regressions/corrections
   only after documenting them.
2. Run all shared final gates with own fresh PG fixture and no DB prerequisite skips.
   Run real-binary PTY scenarios for multiline request metadata, search/detail/Enter,
   held/failed Blocks refresh, recovery and quit at compact and normal sizes.
3. Record per-task acceptance/status audit, actual tests/experiments and limitations;
   confirm owned fixture cleanup and previous work/CHANGELOG preservation.

Acceptance criteria:

- Four completed tasks with passing required gates and actual evidence; no remaining
  acceptance blocker. New bounds and interaction behavior agree with public docs.
- PTY exercises visible selection and zero hidden take-once actions; previous
  navigation/metric/reconnect workflows still pass. Owned fixtures removed.

Verification: shared final gates, PTY experiments and independent source/evidence audit.

Completion evidence (EDGE-04 independent review; implemented none of EDGE-01-03):

- Per-task acceptance audit (source inspection, no changes):
  - EDGE-01 PASS: `internal/accounting/recent.go` named budgets (256x4 identity, 512x3 model, 64 op; 2,624/entry), unconditional `strings.Clone` detachment, exact-first aggregation in `internal/accounting/accounting.go`, `RecentTruncation`/`RecentSequence`/`ProviderID` wire-compatible flags, P95 exact-ID resolution in `internal/dashboard/dashboard.go`, truncated-ID correlation gate, old-snapshot fallback. Re-ran: accounting/dashrpc race PASS, `go test ./internal/accounting -run TestRecent` (2,624 B/op both sizes; ring JSON 3,241,293 vs 3,353,802 bound), dashrpc recent (3,249,095 bytes), dashboard rejected-large-model HTTP fixture (205 rejections, 200 retained, recent JSON ~189 KB), web-ui 196 tests + typecheck PASS.
  - EDGE-02 PASS: `internal/dashboard/metadata.go` escapes LF/CR/tab/ESC, all C0/C1, U+2028/2029, invalid UTF-8, doubles backslashes; applied before composition in `recent.go`, `search.go`, `payload.go`; stored/search identities unmutated; detail scrollable; 512-byte display cap with explicit notice. Real 21-request loopback rejected fixture with actual controls/Unicode/literal escapes: raw-output allowlist + viewport/Enter-identity assertions at 80x12/80x24/120x30/160x48 pass.
  - EDGE-03 PASS: `blockListVisible()` (fetcher, blockKnown, empty err, non-empty rows) gates Enter (returns true,nil while hidden: zero GetBlock RPCs) and cursor mutators; error/loading rows and footer show `[r] retry`, never `[enter]`; counted consuming take-once fixture proves zero consumption while hidden and exact single recovery open; background refresh stays actionable; payload sibling non-consumptive, no change.
- Focused gates (fresh disposable postgres:17 container per README, loopback, health-checked, serial execution, removed afterwards with `docker stop`, confirmed absent): `go test -race -p 1 -count=1 ./internal/accounting ./internal/dashrpc ./internal/dashboard ./internal/httpapi` PASS (accounting 7.015s, dashrpc 2.058s, dashboard 46.714s, httpapi 263.023s). `make vet test` PASS, `make docs-contract` (matrices match), `git diff --check` clean.
- Full gates (same disposable-fixture pattern, serial, fixture removed): `GOFLAGS=-p=1 make test-race` PASS (5m27s; app 27.6s, httpapi 253.6s, store 36.8s, dbmerge 1.4s executed), `make integration` PASS (12.7s; UI build + binary + 2.1s suite), `pnpm --filter @aiproxy/web-ui test` PASS (11 files, 196/196), `pnpm --filter @aiproxy/web-ui typecheck` PASS, `git diff --check` PASS. DB-backed suites executed against the owned fixture; no developer or prior-session fixture used.
- Real-binary PTY (rebuilt `dist/aiproxy`, synthetic loopback fixtures, temp HCL, no real provider auth): full workflows at 80x12 and 120x30 PASS - multiline request metadata stays single-row with escaped newline and no ESC[2J/OSC-52 in raw bytes, Enter opens exact `req-multi-1`; search filters to exact ID and clears; held manual refresh and 500-error refresh show retry cues while hidden arrows/Enter issue zero take-once calls; recovery opens exact visible ID once; `q` exits 0 in under 10ms with zero polls over the following 2.2s.
- Preservation: all incoming BOUNDARY/SAFE/LIFE/FLOW/TUI work and EDGE-01-03 records retained; `git diff --check` clean; `git diff --exit-code HEAD -- CHANGELOG.md` empty (CHANGELOG untouched); no commits. Temporary PTY/evidence artifacts live under `<repo-root>/tmp` only.
- Limitations: memory bound is per-entry/array (2,624 B/entry, ~3.35 MB recent-array cap), not a total-process-heap claim; PTY coverage is representative compact/normal sizes on Linux with synthetic fixtures, not a human study or cross-terminal audit; DB skip counts were not independently re-parsed beyond executed package timings in this final pass.

## Coordinator Final Closure

- All four EDGE tasks are completed with Completion evidence and no unresolved blocker. EDGE-01-03 ran in fresh sequential sub-agent sessions; EDGE-04 verification was split across fresh isolated sub-agent sessions (source audit, focused gates, full gates, PTY) after the single-session launch was blocked by a request filter, then recorded here by the coordinator. No nested agents, no commits, CHANGELOG unchanged.
