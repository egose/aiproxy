# CLI Dashboard Operator Experience And Data Fidelity

Created: 2026-09-26 21:36:04 local time

## Objective

Make `aiproxy dashboard` a dependable interactive operator console: predictable
attach/exit/reconnect, truthful metrics, stable inspection, readable small-terminal
navigation, and enough provider/request context to investigate failures. Implement
the preceding focused review, including its recommended diagnostic interactions.
The dashboard remains a local, authenticated global operator surface.

Preserve all incoming uncommitted BOUNDARY/SAFE/LIFE/FLOW work. **Do not edit
`CHANGELOG.md` or commit.** Document changed public interaction/data contracts in
CLI help, relevant docs and this record. Avoid unrelated provider/admin rewrites.

## Analysis And Baseline

- Followed AGENTS.md and the requested
  `task-as-you-go skill`.
- Coordinator reviewed CLI attachment, TUI model/keys/layout/rendering, payload and
  block inspection, RPC schemas/conversion, accounting sources and tests. Prior
  completed plans cover web-dashboard authorization and billing retention, not
  these TUI lifecycle/data/interaction gaps. Preserve those completed contracts.
- Passed baseline `go test -race ./internal/dashboard ./internal/dashrpc` and
  `go test ./cmd/aiproxy -run 'TestDashboard|TestRunDashboard|TestFetchSnapshot|TestNormalizeBaseURL|TestValidateDashboardTransport' -count=1`.
- Temporary overlay probes (`<repo-root>/tmp/cli-dashboard-review-probe_test.go`,
  `cli-dashboard-review-overlay.json`) reproduced: 80x12 renders 23 lines; 80x24
  renders 25; help/quit footer hints disappear at widths 80/120/160; snapshot
  refresh resets both scroll offsets 5→0; paused payload arrival changes selected
  request b→a; 600 requests/minute displays 3.33/s instead of 10; two minutes idle
  displays accumulated 60,000 tokens as tok/1m; aliases costing $0.001/$0.003 both
  display $0.010 when sharing a target with direct traffic.
- Pseudo-terminal experiment (`<repo-root>/tmp/cli-dashboard-pty-review.py`) with the
  real built binary and local stub snapshot confirmed the dashboard rendered, then
  `q` left the process alive and two more polls occurred in 4.5 seconds. An initial
  invalid HCL fixture was corrected before the successful reproduction.
- Limitations: baseline was focused tests, renderer probes and one PTY lifecycle
  check, not a human usability study or cross-terminal/platform audit. No live AI
  provider, production credential, database or workspace edit during that review.

## Execution Rules And Verification

Run TUI-01 through TUI-08 **sequentially**, each in a **fresh sub-agent session**.
TUI-08 must be an independent reviewer. No nested agents. Shared model/schema/docs
changes are serialized; leave a concise integration contract for following agents.
Set only the owned task `in_progress`; record necessary scoped additions before
editing. Append `Completion evidence` with changed paths, reproductions, commands,
results and limitations. Mark completed only when acceptance and verification pass.

P1 = basic operator control/data correctness; P2 = navigation/diagnostic usability.
Keep code cohesive: focused helpers/modules for new workflows rather than extending
the already-large dashboard.go indiscriminately. Use apply_patch and local style.
Transport additions should be additive where possible, with safe unknown/unavailable
fallbacks for old snapshots; never interpret unavailable data as zero/off.

All commands run at `<repo-root>`. Each implementation runs its
focused tests, `make vet test`, `make docs-contract` as applicable and diff checks.
Frontend schema consumers must be checked if RPC changes affect them. Finish UI
builds before Go tests read embedded assets; serialize artifact builds. No actual
upstream credentials. Frontend/non-DB task owners disclose optional DB skips.

Final gates: `make vet test`, `make test-race`, `make integration`,
`make docs-contract`, `pnpm --filter @aiproxy/web-ui test`,
`pnpm --filter @aiproxy/web-ui typecheck`, and `git diff --check`. Final reviewer
creates an OWN NEW disposable PG17 fixture per README (inspect Docker first,
unique name/DB, loopback port/readiness, explicit AIPROXY_TEST_DATABASE_URL), uses
GOFLAGS=-p=1 for DB suites, proves no DB prerequisite skips, and removes only that
fixture. Never use developer/prior fixtures. Add PTY binary checks for exit and
reconnect plus rendered interaction checks at 80x12, 80x24, 120x30 and larger sizes.

Definition of done: eight completed tasks with acceptance evidence, all final
gates passing, no unresolved blocker, supported viewport/key/metric contracts agree
with docs, fixtures cleaned, existing work and CHANGELOG preserved, final audit.

### Task TUI-01: Complete CLI Lifecycle And Recover From Transient Disconnects

Status: completed

Kind: defect

Priority: P1 — quit leaves the command alive, while temporary transport failure detaches.

Suggested agent: dashboard attachment and lifecycle implementer

Dependencies: none

Primary ownership: `cmd/aiproxy/dashboard.go`, dashboard Program wrapper, focused
CLI/lifecycle tests and operator docs.

Finding: RunWithBlockFetcher discards p.Run result; runDashboard only waits for
context/ticks. TUI quit cannot end the polling loop. Every wrapped transport error,
including timeout, is classified as unreachable and exits instead of showing stale
data/retrying. Run errors are swallowed and stdout/stderr routing is inconsistent.

References: `internal/dashboard/dashboard.go` (Program, RunWithBlockFetcher),
`cmd/aiproxy/dashboard.go` (runDashboard, fetchSnapshot, isConnectionRefused).

Requirements:

1. Propagate TUI completion/errors, stop polling on quit, cancel outstanding work
   and restore terminal reliably. Respect command context and configured I/O.
2. Preserve clear initial attachment errors; after attachment retain the last
   snapshot across transient failures, show last successful refresh/reconnecting
   status and retry with bounded backoff. Provide an explicit manual retry action.
3. Distinguish transient outage from auth/config denial; do not hammer unauthorized
   endpoints or expose token values. Describe recovery/re-attach guidance truthfully.

Acceptance criteria:

- PTY/injected-program regression proves q/Esc/Ctrl+C completion terminates the CLI
  and polling, plus program initialization error/context cancellation cleanup.
- Held/failed/recovering local server tests preserve usable stale UI, bounded retry
  timing, manual retry and restored live updates without duplicate polling loops.
- Subsequent pane fetch commands have a defined shared cancellation/lifetime contract.

Verification: `go test -race ./cmd/aiproxy ./internal/dashboard ./internal/dashrpc`,
focused real-binary PTY experiment, default checks and final gates.

Necessary scope additions (recorded before implementation):

- Owned by this fresh sequential TUI-01 session; dependencies: none. Preserve the
  incoming dirty baseline and all other task statuses.
- Thread the Program lifetime through existing payload/block commands (including
  decisions): their current background contexts otherwise outlive quit. Keep old
  constructors/helper callers working; generation/selection/pause changes stay in TUI-03.
- Share a denial gate with pane RPCs so snapshot auth/config denial cannot leave a
  second pane poller hammering the endpoint. Manual retry probes only the snapshot;
  successful recovery reopens pane access. Do not automatically replay mutations.
- Preserve transport cancellation identity and suppress response-body error text;
  distinguish 401/403/404 and other permanent 4xx from transient failures. Reject
  redirects on snapshot attachment to keep the local authenticated transport contract.
- Add focused lifecycle helpers/tests and update CLI help/operator/design docs;
  use a global Ctrl+R retry action without changing existing pane-local `r` actions.

Completion evidence:

- Changed: `cmd/aiproxy/dashboard.go`, new `dashboard_lifecycle.go` and
  `dashboard_lifecycle_test.go`; `internal/dashboard/dashboard.go`, `program.go`,
  `program_test.go`, `payload.go`, `blocks.go`; new
  `internal/dashrpc/lifetime_test.go`; CLI long help, `docs/design.md` (Interactive
  Dashboard Session Lifetime), `website/docs/operations.md` (Interactive Dashboard
  Lifecycle), and this TUI-01 record. RPC schemas/production dashrpc code unchanged.
- Program now exposes completion/result, waits for terminal cleanup, cancels its
  child context on every exit, and supports configured input/output. The CLI joins
  one cancelable snapshot worker on quit, signal, parent cancellation or Program
  error. Existing exported Run/RunWithBlockFetcher/Refresh/RefreshError remain usable.
- After attach, failures retain the last snapshot; successful receipt time and
  reconnect/denial state are independent of pause. Retry delays are 2/4/8/16/30s
  (30s cap), with a two-second snapshot timeout. Ctrl+R coalesces retries; successful
  polling resets backoff. 401/403/404, redirects and other permanent endpoint errors
  suspend automatic polling and gate new pane RPCs. Snapshot error bodies are not
  rendered, and both CLI HTTP clients reject redirects.
- Persistent regressions cover actual Program q/Esc/Ctrl+C completion, closed-input
  initialization failure, all five pane-command deadlines/cancellation, stale view/
  pause/status/manual callback rendering, held HTTP poll cancellation on CLI exit or
  error, immediate init failure, local-server backoff/recovery/denial and gate reopen,
  manual retry coalescing, configured I/O, HTTP status classification/redirects,
  transport cancellation identity, and all five authenticated RPC body-read cancels.
- Passed `make build` (frontend typecheck + Vite build, then Go binary; serialized
  before asset-reading checks). Passed required
  `go test -race ./cmd/aiproxy ./internal/dashboard ./internal/dashrpc`
  (102.193s / 2.426s / 1.255s), `make vet test`, `make docs-contract`, and
  `git diff --check`. Earlier focused lifecycle race regressions also passed.
- Real binary `dist/aiproxy` (`v0.26.0-67-gbe907db-dirty`) with local synthetic HTTP
  fixture at 120x30: original `<repo-root>/tmp/cli-dashboard-pty-review.py` now reports
  rendered dashboard, q exit 0 and zero polls afterward (baseline: still alive and
  two extra polls). Expanded `<repo-root>/tmp/cli-dashboard-pty-tui01.py` passed 503
  reconnect/automatic recovery, 403 denial with no polls over 2.5s, Ctrl+R recovery,
  and q/Esc/Ctrl+C/SIGTERM exit 0 in 0.053/0.103/0.053/0.052s. All four restored
  termios exactly and produced zero polls during the subsequent 2.2s. An initial
  expanded-script assertion expected a complete repeated version string, but Bubble
  Tea sent only its changed suffix; distinct version markers corrected that fixture
  assertion before the successful run. All child processes/listeners were cleaned.
- Limitations/optional skips: no DB fixture or live upstream used;
  `AIPROXY_TEST_DATABASE_URL` confirmed unset, so optional DB prerequisite suites
  remain skipped. TUI-08 owns PG17/no-skip coverage, full-repo race/integration,
  frontend unit tests and the full terminal-size/interaction matrix. Frontend
  typecheck did run via build; no RPC schema consumer change required. Build-created
  untracked `.gitkeep` removed; ignored built assets/binary retained for review.
  No TUI-01 blockers. Dirty BOUNDARY/SAFE/LIFE/FLOW work and other task statuses
  preserved; CHANGELOG untouched; no commits or subagents.

Next-agent integration contract:

- `Program.Done()` closes after Run/terminal cleanup; `Wait()` repeatedly returns
  the result. `Close()` is idempotent cancellation + wait (its cancellation may be
  observed by Wait). Parent/signal cancellation is a normal CLI exit; genuine
  Program errors propagate. `RunOptions.SignalsHandled` delegates OS signals to
  the caller; CLI sets it and uses Cobra input/output.
- New async pane commands must capture/pass `m.ctx` to `fetchContext` and honor
  its deadline/cancellation. Existing helper optional contexts default to a bounded
  background context for standalone old callers/tests only. Do not mutate model
  state from workers. No independent pane retry loop or automatic decision/take-once
  replay; TUI-03 adds generation/selection/pause guards atop this lifetime.
- `RunOptions.Retry` is synchronous on the model loop and MUST remain nonblocking;
  CLI supplies a capacity-one request channel, drained after an in-flight snapshot.
  Ctrl+R is reserved globally, including detail/help; pane `r` keeps its old meaning.
- `ConnectionStatus` is local receipt metadata, not RPC data or paused snapshot
  time. Preserve its visibility through TUI-04 layout changes. Denial gates reopen
  only after a successful snapshot. Existing failed pane lists require explicit
  pane-local refresh after recovery. Credential/listener changes require re-attach;
  manual retry reuses the attached credentials. The local status fields need no
  frontend schema update; later metric transport work should keep this separation.

### Task TUI-02: Display Accurate Rates, Costs And Measurement Scope

Status: completed

Kind: defect

Priority: P1 — the display undercounts busy traffic and misattributes alias costs.

Suggested agent: dashboard accounting/snapshot fidelity implementer

Dependencies: TUI-01

Primary ownership: `internal/accounting`, `internal/dashrpc`, dashboard remote/rate/
cost rendering, focused accounting/RPC/TUI tests and docs.

Finding: computeRates derives minute rates/15m graph from a 200-event ring and
falls back to rolling accumulated tokens during idle time. TUI aliasCostEntries
matches lifetime upstream aggregates without public-alias attribution, repeating
direct/other-alias costs. Provider totals, public usage and P95 use different
unlabeled windows. Prior BOUNDARY billing fixes do not reach the TUI cost code.

References: dashboard.go (computeRates, tokensPerMinuteFallback, aliasCostEntries,
renderProviders/renderUsage, p95WithSamples); dashrpc snapshot BuildContext;
accounting.Aggregator (BillingSummaries, Recent, retained/lifetime aggregates).

Requirements:

1. Expose bounded time-bucket counters independent of retained recent-request count
   for 1m/5m rates, minute token/errors/429 totals and 15m sparkline. Use deterministic
   clock/boundary tests, expire idle counters, and define precise window granularity.
2. Use the retained attributed billing snapshot for public/alias cost estimates;
   keep used-target missing-price behavior conservative and preserve tenant/client
   isolation. Share an appropriate calculation boundary or explicitly align it with
   the existing correct billing implementation; do not retain heuristic splits.
3. Label global vs selected scope, rolling vs lifetime counts, estimated costs and
   P95 sample/window. Preserve intentionally lifetime provider counters without
   pretending they reconcile to rolling cost/usage. Unknown old-RPC data is unavailable.

Acceptance criteria:

- 600+ requests/minute and all 15 minutes are counted independently of the 200-entry
  ring; idle tok/min becomes zero; exact time boundaries/late/future events and
  bounded storage have tests.
- Two aliases sharing a target plus direct traffic have isolated retained costs,
  covering tenants/clients, expiration, price gaps and provider subtotal semantics.
- RPC→TUI tests verify real transported values and honest labels/fallbacks; existing
  billing/lifetime counter tests and web consumer schemas remain correct.

Verification: `go test -race ./internal/accounting ./internal/dashrpc ./internal/dashboard ./internal/httpapi`,
default checks (serialize DB packages if fixture supplied), final gates.

Necessary scope additions (recorded before implementation):

- Owned by this fresh sequential TUI-02 session; TUI-01 is completed. Preserve its
  lifetime/cancellation contract, incoming dirty work and all other task statuses.
- Extract the correct BOUNDARY billing price/attribution calculation into a reusable
  boundary consumed by HTTP billing and dashboard estimates, retaining billing API
  behavior and existing regressions. Add transport measurement metadata and check
  the web snapshot schema consumer; these are required for truthful old-snapshot
  fallback and shared cost semantics.
- Use fixed-size global rate buckets independent of identity cardinality/recent ring;
  explicitly label global rates versus selected retained usage and lifetime provider
  counters. Document precise clock, granularity and P95 sample semantics.
- Retained upstream rows now carry public attribution, unlike the old lifetime
  upstream aggregates. Regroup the upstream usage toggle by tenant/client/resolved
  model/operation/status to preserve its aggregate meaning. Label the existing alias
  detail counters as provider-wide lifetime and suppress unavailable old counters;
  provider/alias diagnostics and selection work remain with TUI-05.

Completion evidence:

- Changed: `internal/accounting/accounting.go`, new `billing.go`, `rates.go` and
  `rates_test.go`; new `internal/usagecost/cost.go`; `internal/httpapi/response.go`;
  `internal/dashrpc/dashrpc.go` and new `measurements_test.go`;
  `internal/dashboard/dashboard.go`, `remote.go`, `cost_test.go`, new `costs.go`,
  `measurements.go`, `measurements_test.go`; `web-ui/src/types.ts`, `types.test.ts`;
  CLI dashboard long help, `docs/design.md` (Dashboard Measurement Boundaries),
  `website/docs/operations.md` (Dashboard Metrics And Cost Estimates), and this record.
- Added a fixed 901-slot global second ring, independent of billing identity keys
  and the 200-event recent ring. Snapshots count the last 60/300 complete seconds
  and expose 15 chronological one-minute request/error/429/token bins. Tests cover
  15,000 events (1,000 per graph minute), minute idle zero, 5m decay, complete idle
  expiry, exact half-open boundaries at 1m/5m/15m and nanoseconds either side,
  zero/future timestamp normalization, late events/ring-slot collision prevention,
  3,600 seconds of slot reuse with changing identities, concurrency and detached
  snapshots. Lifetime and retained billing totals survive rate expiry.
- `BillingSnapshot()` copies public and attributed upstream rows plus retention
  metadata under one lock, preserving BOUNDARY's inclusive minute-bucket cutoff.
  The shared `usagecost` boundary replaces TUI heuristic/count/even splits and
  preserves HTTP billing's exact identity/count/token reconciliation and cache-price
  rules. Provider subtotals are complete or unavailable; a different provider's
  price gap does not suppress a fully attributed/priced subtotal. Tests replace the
  previous heuristic expectations, rather than preserving those incorrect contracts.
- Production `Aggregator.Record` → `RuntimeSource/Build` → JSON marshal/unmarshal →
  `SnapshotFromTransport` → TUI regressions verify 600/min renders 10.00 req/s,
  60 errors/30 throttles/60,000 tokens, idle tok/min zero, actual P95 590ms/200 from
  the capped sample, immutable window across local clock/filter changes, and all
  15 graph minutes even with RPC `recentN=1`. Separate transported fixtures verify
  aliases a/b/direct estimates $0.0010/$0.0030/$0.0060; empty and nonempty tenant/client
  dimensions; tenant-first versus tenantless-client billing filtering; exact 24h
  cutoff then expiry; retained upstream regrouping; lifetime provider/recent retention;
  used-price gaps and unused/currently changed alias targets; provider subtotal
  rendering; and unavailable old-RPC rates, costs, counters and upstream usage.
- Labels now identify GLOBAL rates, lifetime provider counters, rolling usage and
  estimated costs, and P95/n's received positive-duration sample (global cap 200,
  no time window). Disabled providers retain recorded counters. The alias detail
  labels its existing counters provider-wide lifetime. Measurement scope uses the
  existing rate strip, preserving TUI-01's independent connection footer. An initial
  two-extra-provider-row layout caused normal-render/last-provider-scroll tests to
  fail; moving scope labels to the rate strip restored both without changing their
  assertions. Old temporary review-helper probes were not imported after API removal;
  persistent production-path tests reproduce their rate/idle/alias-cost findings.
- Passed targeted package tests, then required
  `go test -race ./internal/accounting ./internal/dashrpc ./internal/dashboard ./internal/httpapi`
  (5.121s / 1.848s / 2.325s / 17.052s). Passed `make vet test`, including existing
  BOUNDARY billing attribution/cache-price/retention and lifetime-counter regressions,
  plus CLI/TUI lifecycle tests. Passed `make docs-contract` and `git diff --check`.
  Passed `pnpm --filter @aiproxy/web-ui test` (11 files, 193 tests) and
  `pnpm --filter @aiproxy/web-ui typecheck`; schemas preserve additive fields,
  attribution/cache tokens, nil collections and absent/null measurement availability.
- Limitations/optional skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; no DB
  fixture was needed or created and optional DB-prerequisite suites remain skipped.
  No live providers/credentials, new PTY experiment or artifact build for this task.
  TUI-08 owns fresh PG17/no-skip validation, final full race/integration/artifact/PTY
  gates; TUI-04 owns the already-planned compact viewport/help layout work. No TUI-02
  blockers. Incoming dirty BOUNDARY/SAFE/LIFE/FLOW/TUI-01 work and other statuses are
  preserved; CHANGELOG untouched; no commits or subagents.

Next-agent transport/state contract:

- Optional `Snapshot.rates` is `*accounting.RateSnapshot`: `window_end` (server
  accounting clock floored to a second), `bucket_seconds=1`, `minute`,
  `five_minutes`, and `minutes[15]` oldest first. Each count has `requests`, `errors`
  (excluding 429), `throttled`, `tokens`. Windows are `[end-duration,end)`; the
  ongoing second is withheld until complete. Rates divide by full 60/300 seconds,
  including startup. Late retained events enter their original second; zero/future
  timestamps clamp to admission time. This global fixed-size state must not be
  replaced by `Recent`, keyed per identity, or recomputed from local `m.now`.
- Optional `Snapshot.billing` is `*accounting.BillingSnapshot`: `as_of`,
  `retention_seconds=86400`, `bucket_seconds=60`, `usage`, `upstream`. Accounting
  row keys remain capitalized JSON, including upstream `PublicModel`. Public and
  attributed rows are one atomic retained copy. Legacy top-level `usage` mirrors
  billing usage; `provider_stats` and top-level `upstream` remain **lifetime**.
  Do not use the latter to price or populate the retained upstream usage view.
  `BuildContext` now populates all accounting fields for both direct Build callers
  and RuntimeSource. Other measurement/lifetime snapshots have their own locks;
  there is no cross-field transaction promise beyond the paired billing rows.
- `remoteUsage` exposes optional data through `RateSnapshot`/`BillingSnapshot`;
  `retainedBilling`/`retainedUpstream` are TUI helpers. Missing pointers mean
  unavailable, not zero, and legacy upstream is never an attribution fallback.
  `Recent(n)` caps the received sample. `providerStatsAvailable` distinguishes
  unavailable old counters from known empty new counters. Transport data is owned
  by the received snapshot and treated as immutable. TUI-03 should buffer/apply
  that snapshot as a unit; rates already stay frozen despite local ticks, pause,
  stale connection time or tenant/error filters. Preserve TUI-01's live receipt
  status separately. No new async workers or cancellation/state protocols were added.
- Reuse `usagecost.Cost`/`ProviderCost` for estimates; current alias target lists
  cannot establish attribution. Matching includes **exact** empty/nonempty tenant
  and client, public model, operation and status, and reconciles all count/token
  fields. Missing used prices yield unavailable; incomplete alias attribution
  conservatively invalidates provider subtotals. Provider totals are global;
  usage uses the selected tenant/error scope. The upstream toggle regroups only
  retained rows and preserves tenant/client/operation/status dimensions.
- TUI-04 must preserve scope labels in the two-line rate strip and EST$/P95/n
  headers when redesigning layout. P95 is nearest-rank over positive durations in
  received recent completions (at most 200 globally), not a timed window; `/n`
  is per-provider sample size. Full explanatory legends are in help/operator docs.
  Keep Zod `rates`/`billing` nullish and update `web-ui/src/types.test.ts` whenever
  these transport contracts change. TUI-05 still owns provider diagnostics,
  LastChecked/affinity transport and synchronous DNS removal.

### Task TUI-03: Keep Selection And Pause Stable Across Refreshes

Status: completed

Kind: defect

Priority: P2 — live arrivals disrupt investigation and pause does not freeze lists.

Suggested agent: TUI state/async interaction implementer

Dependencies: TUI-02

Primary ownership: dashboard model state/update, payload/blocks list state and tests.

Finding: applySnapshot resets provider/usage scroll; payload/block cursors are row
indices and move to different records when new items arrive. Paused snapshots buffer
but payload/block results and tick fetches still change visible data. Tenant selection
is an index in a changing name list. Filter changes can accept stale in-flight lists.

References: dashboard.go (applySnapshot, Update tick/snapshot cases, activeTenant),
payload.go (applyPayloadList, requestPayloads), blocks.go (applyBlockList).

Requirements:

1. Preserve selected/top-visible identities for providers, usage, aliases, payloads,
   blocks and tenant filters when data changes; clamp/remove with explicit predictable
   fallback when an identity disappears. Preserve log follow/pinned-selection behavior.
2. Pause must freeze all displayed data/window clock consistently, buffer at most
   latest results and resume coherently. Connection state may remain live and must
   be labeled separately. Bound pending state and continue cancellation from TUI-01.
3. Guard asynchronous list/detail/filter responses by generation/request identity;
   obsolete results must not overwrite a newer view or selection.

Acceptance criteria:

- Repeated refresh retains scrolled position and selected identity; prepend/delete/
  reorder/tenant additions/removals have deterministic tests for each list class.
- Paused payload/block arrivals do not change visible rows/selection or rate window;
  resume applies latest results once. Out-of-order filters/details are discarded.
- Existing log follow/order, keyboard paging and take-once detail semantics survive.

Verification: `go test -race ./internal/dashboard ./cmd/aiproxy`, default/final gates.

Necessary scope clarifications (recorded before implementation):

- Owned by this fresh sequential TUI-03 session after reading both preceding
  integration contracts, AGENTS.md and the requested project task-as-you-go skill.
  Preserve the dirty baseline, TUI-01/02 and all other statuses.
- Providers/usage currently have scroll positions, not selected-row cursors: anchor
  their top row; anchor both selection and top row in existing cursor lists. Identity
  removal falls back to the old numeric position clamped to the new list; keeping
  selection visible takes precedence when reordered anchors cannot share a viewport.
  A removed selected tenant falls back explicitly to all tenants, never another name.
- Pause freezes incoming data, list/detail/error results and the display clock;
  existing local navigation/filtering of frozen data remains usable. Defer new remote
  list/filter/detail/decision actions until unpaused (no automatic mutation replay).
  In-flight results occupy at most one slot per request class; snapshot and connection
  receipt status remain separate. Detach mutable in-process viewers on pause as well
  as supporting immutable transported snapshots.
- Introduce focused reusable identity/request helpers; tag all existing pane commands,
  including decision acknowledgments, to reject late results after close/reopen of
  the same ID. Preserve take-once capture semantics and TUI-01 context deadlines.
  Future provider cursor/detail, Requests/search and decision UX remain later tasks.

Completion evidence:

- Changed: `internal/dashboard/dashboard.go`, `payload.go`, `blocks.go`, new
  `selection.go`, `requests.go`, `pause.go`, `state_test.go`, and the existing
  payload order test; CLI long help in `cmd/aiproxy/dashboard.go`, `docs/design.md`
  (Dashboard Refresh Identity And Pause), `website/docs/operations.md` (Dashboard
  Stable Inspection And Pause), and this TUI-03 record. No RPC/accounting/frontend
  schema change; TUI-02 measurement sources and TUI-01 lifetime code remain intact.
- Refresh resolves provider/usage top rows, alias/payload/block selected and top rows,
  and pinned log top rows by stable identity before viewport clamping. Usage keys
  contain all five exact grouping dimensions and exclude mutable counters. Provider
  identity survives enabled/disabled group movement. Tenant names survive snapshot
  additions/reordering/removals of other tenants; a removed selection returns to all.
  Payload order now preserves the selected request; the old order regression's
  detail expectation was updated from the new first row to the preserved identity.
- Pause detaches mutable in-process usage/log/health viewers and buffers snapshot
  data as a unit, preserving optional measurement availability and TUI-02 windows.
  Ticks retain only their latest timestamp and schedule no pane read while paused.
  All five pane result classes, including errors/disabled states and acknowledgments,
  wait in individual bounded latest-result slots. Resume drains them in one model
  update and clears references. Unvisited panes load on resume; failed lists still
  need explicit refresh as specified by TUI-01. New remote actions during pause are
  not queued, and no take-once read/decision is automatically replayed.
- `requestSlot` tags all production pane commands, cancels superseded/closed work
  under the Program context, and invalidates completed generations. Matching includes
  detail ID and decision action, so close/reopen of the same ID cannot accept an old
  response. Stale errors cannot clear newer loading state or replace newer results.
  Closing paused detail clears its captured result buffer and acknowledgment slot.
- Deterministic `state_test.go` coverage: repeat/prepend/reorder/delete-selected/
  delete-top/shrink/empty for providers, usage, aliases, payloads and blocks; conflicting
  top/selection anchors; disabled provider movement; every usage grouping dimension
  in public and retained-upstream views with changing counts/tokens; tenant additions,
  reordering, removal and empty fallback; oldest-first payload refresh/order selection;
  20 paused snapshots/ticks with latest-only resume and rate-window/clock assertions;
  mutable-viewer freeze, cached navigation, live connection status, errors/disabled
  arrivals, stale buffered lists, unvisited versus failed pane resume; filter results
  in both delivery orders; same-ID close/reopen detail rejection; late/duplicate
  decisions; channel-controlled request cancellation/deadlines; consumed take-once
  detail with no replay; log follow/pinned top/expiry in both orders. Existing paging,
  log-order/follow, detail and CLI lifecycle regressions also pass.
- Initial full-view pause regression exposed the old `PAUSED+1` snapshot-buffer
  indicator changing the display despite frozen data. Replaced it with stable
  `PAUSED`; connection metadata remains separately live. Targeted dashboard tests
  passed after this correction. No relaxed selection or pause assertions.
- Passed required `go test -race ./internal/dashboard ./cmd/aiproxy`
  (2.044s / 87.412s), `make vet test`, `make docs-contract`, and `git diff --check`.
  Checks were serialized; no artifact/frontend build or concurrent Go asset access.
- Limitations/optional skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional
  DB prerequisite suites remain skipped. No DB fixture, live provider, credential,
  PTY experiment or frontend checks required for this state-only task. TUI-08 owns
  the fresh PG17/no-skip final validation, full-repo race/integration and PTY/viewport
  gates. TUI-04 retains the planned input/help/layout work. No TUI-03 blockers.
  Incoming dirty BOUNDARY/SAFE/LIFE/FLOW/TUI-01/02 work and all other statuses preserved;
  CHANGELOG untouched; no commits or subagents.

Next-agent state integration contract:

- `anchoredIndex(before, after, index, key)` handles identity lookup and missing-row
  old-index fallback. Apply it to the actual displayed ordering, then clamp viewport
  bounds; selected-row visibility wins over a conflicting top anchor. Provider and
  usage still have only scroll positions: TUI-05 can add a provider cursor using
  `providerIdentity` without reintroducing scroll resets. Usage identity must keep
  exact tenant/client/model/operation/status. Explicit usage filter changes reset top.
- `requestSlot.start(m.ctx, command)` invokes the command builder synchronously on
  the model loop; capture fetcher/filter/ID/action inputs there. The returned worker
  tags its result without accessing model state. Add new result types to its tagging
  switch (or extend this boundary deliberately), and give each independent operation
  its own slot. Raw `fetch*Cmd` helpers retain TUI-01 optional-context compatibility,
  but production model paths MUST go through a slot; zero-generation synthetic
  messages are suitable only for a pristine model in standalone tests.
- Validate generation and identity before changing loading/error/data state or
  buffering. Completion calls `invalidate()` to reject duplicate delivery; filter
  replacement and close also invalidate/cancel. Paused accepted results keep their
  generation until resume, when ordinary apply methods validate again. Clear the
  corresponding `pausedResults` field on close/replacement. No mutation retry loop.
- `togglePause` freezes data/clock and drains the pending snapshot plus five result
  slots. Update's global `p` dispatch makes pause/resume reachable inside existing
  details and starts an unknown active pane on resume, but does not retry a failed
  list. `tickMsg` now carries the scheduled timestamp (zero still falls back to local
  time for old standalone callers). `liveNow` is latest local tick, `now` is displayed
  time, and rates/billing remain received server windows, never locally recomputed.
- TUI-04 input-mode work must preserve reachable `p`, separate live connection status,
  cached navigation while paused, and cancellation/buffer clearing on every new close
  path. Unpaused snapshot application owns anchor reconciliation. Preserve the existing
  explicit log-order→follow behavior. New Requests/search/provider-detail result classes
  must join the pause and generation boundary; do not rely only on record IDs or add
  unbounded history/pending queues. Detail/decision UX and compact layout remain their
  assigned later tasks, including help ownership and numbered decision key cleanup.

### Task TUI-04: Fit Supported Terminals And Make Keyboard Controls Discoverable

Status: completed

Kind: defect

Priority: P2 — layout overflows supported terminals and hides help/quit controls.

Suggested agent: responsive terminal layout and navigation implementer

Dependencies: TUI-03

Primary ownership: dashboard layout/view/key dispatch/help and rendered interaction tests.

Finding: minimum-size message accepts 80x12 but rendered output exceeds height;
fixed pane minima also overflow 80x24. Long shared footer truncates help/quit. Both
brackets cycle in the same direction; help does not own input and hidden pane actions
can run. Global hints advertise keys unavailable in some panes.

References: dashboard.go (relayout, splitStatsHeight, render, renderHelp,
renderFooter, handleKey, fitView, renderTabStrip).

Requirements:

1. Introduce a compact focused-pane layout at small heights, preserving bounded
   width/height and reachable tabs. Keep supported minimum 80x12 usable or explicitly
   justify/document a change rather than silently clipping essential rows.
2. Use context-specific short hints with help/back/quit always visible; searchable
   workflows added later must reuse a clear input-mode contract. Help should safely
   own input, close with expected keys, and fit/scroll at supported sizes.
3. Make [ previous / ] next consistent; Tab/Shift-Tab focus, Enter detail and z zoom
   must be predictable and documented. Reserve numbered tab keys for navigation,
   not mutation actions in details. Preserve explicit Ctrl+C exit.

Acceptance criteria:

- 80x12, 80x24, 120x30, larger and resize-during-detail/help renders stay within
  terminal cell bounds, including Unicode/wide text and long errors.
- Help/back/quit discoverable in every mode; pressing keys in help cannot silently
  mutate hidden pane state or issue a guardrail decision. Navigation is reversible.
- Tests cover compact/zoom/focus/help/detail transitions without losing selection.

Verification: `go test -race ./internal/dashboard`, rendered size/key matrix,
default/final gates.

Necessary scope clarifications (recorded before implementation):

- Owned by this fresh sequential TUI-04 session; TUI-01–03 are completed and their
  lifetime, measurement and identity/pause contracts have been read. Preserve all
  incoming dirty work and other task statuses.
- Use focused layout below 30 rows (including 80x12 and 80x24); keep stacked panes
  at larger heights with explicit minimum content budgets. Share cell-aware fitting
  across pane frames, rows, headers, errors and help so overflow cannot displace
  the footer. Keep the two-line measurement strip and independent connection status.
- Centralize input ownership and detail closure (including request cancellation and
  paused buffers). Help consumes keys before pane handlers; future search must join
  this same modal boundary. Tab/number/bracket navigation closes detail first;
  Enter only opens/closes existing details, and z alone toggles pane zoom.
- Remove numbered block decision aliases now. Scope local filter/order actions to
  their focused pane and suppress pane actions while an undersized warning owns
  the screen; this is necessary to prevent invisible off-screen actions.
- Update CLI help, operator/design docs and meaningful rendered/key/resize tests.
  Provider diagnostics, search and per-finding decision redesign remain TUI-05–07.

Completion evidence:

- Changed: `internal/dashboard/dashboard.go`, `blocks.go`, `dashboard_test.go`,
  `payload_test.go`; new focused modules `input.go`, `layout.go`, `help.go` and
  rendered/key regressions `layout_test.go`; CLI long help in
  `cmd/aiproxy/dashboard.go`, `docs/design.md` (Dashboard Layout And Input Ownership,
  Input Contract For Requests/Search), `website/docs/operations.md` (Dashboard
  Layout And Keyboard Controls plus Esc lifecycle wording), and this TUI-04 record.
- Supported minimum remains 80x12. Below 30 rows, one focused pane plus a reachable
  numbered tab strip replaces the stacked layout. Larger stacked views reserve
  explicit minimum stats/bottom budgets. Zoom uses the same focused geometry;
  provider/usage visible-row counts now match the pane actually rendered in zoom.
  Initial regressions exposed Lip Gloss v2 Height including borders: corrected
  total-height allocation instead of relying on the prior incidental content growth.
- Pane frames fit content before style wrapping, preserving border/viewport bounds.
  ANSI-aware terminal-cell measurement, truncation, padding and wrapping replace
  rune counts for the shared width operations. Long errors and multi-line header
  metadata cannot grow the frame. Reserved footer controls remain visible; its second
  line preserves live receipt/retry status, and the two-line measurement strip retains
  GLOBAL/lifetime/P95/n/EST$ scope (short untimed legend at narrow widths).
- Central input ownership gives help scroll/close/quit/retry behavior without passing
  keys to hidden panes. Esc unwinds help, detail, zoom, then quits. Ctrl+C/q exit
  every existing mode. Tab/Shift-Tab reverses focus; brackets reverse through numbered
  tab order; all navigation closes/cancels detail and clears its paused result slots.
  Enter only opens/closes existing detail; z alone toggles list zoom. Removed block
  1/2/3 decision aliases; a/s/d retain their existing all-finding effect with scope
  visible in the footer while scrolling. Focus-local filters/order and undersized
  warning ownership prevent off-screen key actions. Pause/retry remain reachable in
  detail, and existing request generation/cancellation semantics are preserved.
- Persistent rendered/key matrix covers 80x12, 80x24, 120x30, 160x48 and 250x60 with
  40 populated rows per list, every focus/zoom combination, every existing detail,
  help paging to the final line, actual arrow/Tab/Shift-Tab/Esc/Enter key codes,
  CJK/combining/emoji-ZWJ text, ANSI-colored text, multi-line long errors and long
  header/detail IDs. Every supported render asserts exact terminal-cell width and
  height, footer help/back/quit and independent last-OK status.
- Additional regressions cover help ignoring decision/filter/pause/navigation keys
  without commands or hidden-state changes, all help close keys and reserved retry,
  resize through compact/large detail/help preserving selected identity and visibly
  marked selected row, pending-read close/late reply rejection on every navigation
  path, numbered-tab zero decision calls, paused buffer clearing, undersized warning
  action suppression, and q/Ctrl+C QuitMsg from browse/zoom/detail/help/warning.
  Existing lifecycle, metrics, pause/identity, paging and take-once tests pass.
  Updated old bracket expectations to genuinely inverse numbered order and moved
  two usage-filter test actions onto focused Usage; no selection assertion relaxed.
- Passed final serialized `go test -race ./internal/dashboard` (6.529s),
  `make vet test`, `make docs-contract`, and `git diff --check`. Earlier dashboard
  runs reproduced and corrected the total-height/optional-move-hint regressions.
  No artifact/frontend build or RPC/schema change was needed for this task.
- Limitations/optional skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional
  DB prerequisite suites remain skipped. No DB fixture, live provider, credential,
  PTY experiment or frontend check was used. TUI-08 retains fresh PG17/no-skip,
  real-binary PTY and final whole-repository integration/race gates. No TUI-04
  blockers. Incoming BOUNDARY/SAFE/LIFE/FLOW/TUI-01–03 work and other statuses
  preserved; CHANGELOG untouched; no commits or subagents.

Next-agent layout/key integration contract:

- `input.go` is the mode/dispatch boundary; `hasDetail` and `closeDetail` must include
  new provider/request detail state. Every close path must invalidate/cancel its
  request slots and clear paused buffers, without changing list selection. Detail
  navigation closes first; Enter closes detail; Esc preserves underlying zoom until
  the next Esc. `loadFocusedPane` loads only an unknown focused bottom pane, never a
  failed list or a mutation. Existing TUI-01 deadlines and TUI-03 generations apply.
- `focusedLayout()` means compact (<30 rows) OR explicit zoom. Normal chrome is
  five rows (header + two measurements + two footer); focused lists reserve one
  body row for tabs. `effStatsHeight` excludes that strip; `effBottomHeight` includes
  it. Provider/usage/bottom visible-row helpers must match render geometry. Details
  use the whole body. `paneBox` now returns a `paneFrame` with a bounded Render
  method and total outer dimensions, not a bare style. Optional more-row hints may
  be omitted when full; selected data and reserved footer controls take precedence.
- Keep context controls in `renderContextFooter`, with essentials budgeted before
  hints. New actions must be truthful for the visible mode. Keep measurement labels
  and live connection status independent of paused data. New tabs must join the
  displayed numbered order, `navigationKey`, bracket cycling and help together.
- TUI-06 search must insert its editor dispatch after global Ctrl+C/Ctrl+R and before
  the q/help/pause/navigation checks in `handleInput`. Add a derived inputSearch mode
  with bounded editor state; printable q/p/h/digits/brackets are text while editing.
  Esc cancels/restores prior applied filter; Enter applies/exits; no fallthrough to
  pane handlers. If help is exposed during search, preserve an explicit return mode
  rather than consuming typed `?`. Budget prompt/footer space using the same frame,
  and join pause/generation/identity boundaries for new filtered result classes.
- TUI-05 owns provider Enter detail and synchronous DNS removal; this task does not
  change provider data/networking. TUI-07 owns end-to-end long captured-text inspection
  and selected-finding decisions; a/s/d are still all findings today, now explicitly
  scoped and safely isolated from help and numbered navigation.

### Task TUI-05: Add Provider Diagnostics And Truthful Alias Details

Status: completed

Kind: improvement

Priority: P2 — operators cannot inspect the reason for provider trouble from its row.

Suggested agent: provider/alias diagnostics implementer

Dependencies: TUI-04

Primary ownership: dashboard provider selection/detail rendering, RPC provider/alias/
health metadata and conversion, focused tests/docs.

Finding: Enter in providers zooms rather than opens a selected provider. Endpoint,
type/models and probe diagnostics are inaccessible or only shown via alias targets;
LastChecked is dropped by remote conversion. Alias detail prints session_affinity
although transport drops it, and repeats whole-provider stats under each target.
Provider IP DNS lookup blocks rendering up to 1.5 seconds per hostname.

References: dashboard.go (renderProviders, providerIP, renderAliasDetail),
remote.go (SnapshotFromTransport, healthchecksFromTransport), dashrpc Provider/Alias/
HealthcheckStatus/toProviders/toAliases.

Requirements:

1. Add stable provider cursor and Enter detail, with type/display/name, sanitized
   endpoint, enabled/health state, probe path/status/reason/last-check age, configured
   model names/upstream mappings/protocol/capabilities and relevant existing settings.
   Never expose credentials or secret-bearing URL components.
2. Transport session-affinity/necessary alias metadata accurately; label provider-
   wide counters as such or show correctly attributed target counters. Unknown old
   transport fields must not be presented as explicitly disabled configuration.
3. Remove DNS/network work from synchronous rendering. Prefer configured host as
   primary endpoint; optional IP resolution must be asynchronous, bounded/cancelable
   and not prevent navigation. Keep new metadata payload bounded by existing catalog.

Acceptance criteria:

- Standalone providers with no aliases expose actionable probe diagnostics and full
  model information; successful/failed/pending/no-probe/disabled cases tested.
- Actual source→RPC→TUI conversion preserves LastChecked and affinity; old snapshots
  have honest fallbacks, and secret fixtures do not appear in detail.
- Slow DNS fixture cannot block View/keyboard/quit; provider detail respects compact
  layout, selection stability and pause contracts.

Verification: `go test -race ./internal/dashboard ./internal/dashrpc ./internal/httpapi`,
frontend schema checks if affected, default/final gates.

Necessary scope clarifications (recorded before implementation):

- Owned by this fresh sequential TUI-05 session; TUI-01–04 dependencies and their
  lifetime/measurement/selection/layout contracts have been read, along with AGENTS.md
  and the requested project task-as-you-go skill. Preserve all incoming dirty work
  and other task statuses; no CHANGELOG, commits or subagents.
- Add optional, catalog-bounded diagnostic metadata to the existing RPC and update
  its frontend schema/tests. Preserve unknown old-snapshot configuration explicitly.
  Sanitize endpoint/probe URL components and diagnostic reasons at the serialization
  boundary as well as TUI display; credentials/references never join transport.
- Remove render-time DNS entirely: configured sanitized host is the primary list
  value. No resolver worker/cache is needed. Provider detail is snapshot-local and
  scrollable, uses stable provider identity, and joins existing modal/pause/layout
  boundaries without introducing another remote request lifecycle.

Completion evidence:

- Changed: new `internal/dashboard/providers.go` and `providers_test.go`;
  dashboard `dashboard.go`, `remote.go`, `input.go`, `layout.go`, `dashboard_test.go`
  and `state_test.go`; `internal/dashrpc/dashrpc.go`, new `diagnostics.go` and
  `diagnostics_test.go`; `web-ui/src/types.ts` and `types.test.ts`; dashboard CLI
  long help, `docs/design.md` (Dashboard Provider Diagnostics And Metadata),
  `website/docs/operations.md` (Dashboard Provider Diagnostics), and this record.
  No production app/healthcheck/httpapi changes were needed: tests exercise their
  existing real source/manager/handler integration from the dashboard package.
- Providers now have a visible `>` cursor and Enter drill-down, preserving name
  identity across refresh/reorder and enabled/disabled grouping. Selection visibility
  wins over conflicting top anchors. Removal clamps the list to the old numeric
  position but keeps an open detail on its original name with an explicit removed
  message. Enter/Esc/navigation close detail through TUI-04's modal boundary; z alone
  still zooms lists. Detail wraps/pages within the full body and uses frozen snapshot
  data/display clock during pause. No asynchronous provider command or worker added.
- Standalone detail exposes name/display/type, sanitized effective endpoint, enabled
  and routing-health state, header timeout, probe configuration and stored threshold
  state/latest HTTP status/reason/last-check age, plus all configured public/upstream
  model mappings, protocol and capabilities. Latest HTTP status is unavailable when
  no response exists. Zero/missing or future check times never become fabricated
  zero-age checks. Pending, no-probe, disabled and unavailable metadata are distinct.
- RPC carries optional provider diagnostics, model details and explicit affinity
  enabled/effective-header metadata. Absent/null old fields remain unknown; an absent
  old endpoint is not replaced with an invented provider default. LastChecked now
  survives remote conversion. Alias counters explicitly remain provider-wide lifetime,
  repeated per target and not target/alias counts; wrapped metadata keeps long default
  affinity header lists inspectable. Existing accounting/rate/cost contracts preserved.
- Sanitization occurs before production serialization and again at TUI display for
  old/in-process snapshots. URLs remove userinfo/query/fragment and redact nonstandard
  path segments; invalid/opaque URLs fail closed. Raw probe errors become bounded
  transport/timeout/DNS/refused/TLS categories or safe status/body-mismatch messages.
  Credentials/references, expected bodies and authorization settings are absent from
  diagnostic transport. Removed the synchronous resolver and IP cache entirely;
  HOST uses the configured sanitized hostname without network work.
- New production-path regressions build a real App with a gated local health server,
  read the authenticated HTTP snapshot, unmarshal JSON, convert and inspect TUI detail.
  They cover pending then successful/503 standalone probes with no aliases, an actual
  stored check timestamp bounded by probe completion, equality across repeated snapshots
  (not receipt-time fabrication), exact transported timestamp and 37s TUI age. Separate
  RuntimeSource→real HTTP handler→JSON→TUI fixtures cover native/messages model metadata,
  no-probe and disabled unreferenced providers, custom/default/disabled affinity, shared
  provider counters repeated under two targets, and absence of credential/key-reference/
  OAuth-reference/userinfo/query/fragment/path/probe-body/error-text secret fixtures
  in the full response and detail. Sanitizer tests cover relative/absolute/IPv6/encoded/
  malformed URLs, idempotence, and allowlisted versus untrusted error messages.
- Interaction regressions cover actual arrows/Enter/Esc/Tab/z/help/page/home/end keys,
  80x12, 80x24, 120x30 and 160x48 detail bounds/footer/live connection status, visible
  cursor, refresh/reorder/removal/empty fallback, disabled group movement, conflicting
  anchors, and paused detail/health-age/clock followed by coherent resume. A deliberately
  blocking `net.DefaultResolver` fixture proves View, navigation, detail and QuitMsg
  completion within one second with **zero resolver calls**. Removed obsolete tests
  asserting DNS resolution/cache behavior and retained their configured-host coverage.
  Existing TUI-01–04 lifecycle/measurement/state/layout suites continue passing.
- Passed required `go test -race ./internal/dashboard ./internal/dashrpc ./internal/httpapi`
  (initial full run 9.299s / 1.553s / 18.511s; final dashboard run after old-endpoint
  fallback refinement 5.268s, other packages cached). Passed final `make vet test`,
  `make docs-contract`, and `git diff --check`. Passed frontend tests (11 files,
  194 tests) and `pnpm --filter @aiproxy/web-ui typecheck`. Frontend compatibility
  tests preserve new metadata/LastChecked and distinguish old absence/null affinity
  from explicit false; no artifact build overlapped Go asset reads.
- Limitations/optional skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional
  DB-prerequisite suites remain skipped. Tests used only local synthetic upstreams
  and temporary configs cleaned by the tests, with no real credentials or DB fixture.
  No binary build or new PTY experiment for this task. TUI-08 retains independent
  fresh PG17/no-skip validation, final whole-repo race/integration and real-binary PTY
  gates. No TUI-05 blockers. Incoming dirty work and every other task status preserved;
  CHANGELOG untouched; no commits or subagents.

Next-agent provider/key/transport contract:

- `providerCursor` selects the combined enabled-then-disabled list by `providerIdentity`
  (name). `providerScroll` independently anchors the top row. Use
  `clampProviderCursor` after reconciliation; selected-row visibility wins. Detail is
  `providerDetailName` plus `providerDetailScroll`, resolving only against the displayed
  snapshot. A removed detail stays visibly removed until close. Enabled state is
  authoritative from list membership, not a reconstructed config.Provider bool.
- Provider detail joins `hasDetail`, `closeDetail`, `inputDetail` dispatch and full-body
  rendering. It is snapshot-local: no fetcher, request slot, pending-result class or
  DNS work. Preserve global pause/retry/help/quit and TUI-04's search insertion point.
  Enter opens/closes provider or bottom detail; z toggles list zoom only; navigation
  closes detail first; Esc unwinds detail before underlying zoom. TUI-06 must keep
  these controls intact when adding Requests/search. Usage still has no Enter action.
- New optional wire objects: `Provider.diagnostics { header_timeout, probe }`, with
  probe null meaning known absent; probe fields are path/method/expected_status/
  interval/timeout/failure_threshold/success_threshold. `ModelPrice.details` contains
  display_name/upstream_name/protocol/capabilities. Empty protocol in a present object
  means provider-native/not separately configured. `Alias.session_affinity` contains
  enabled and effective headers; disabled headers may serialize null. These objects
  are nullish in Zod. No credentials or auth settings belong in these objects.
- `RuntimeSnapshot.ProviderMetadata`, `ModelMetadata` (provider/name key) and
  `AliasAffinity` preserve availability separately from config. Non-nil maps identify
  transported snapshots; missing/nil entries mean unknown. Nil maps are in-process
  config viewers with actual config available. Preserve these immutable maps through
  pause/new snapshot paths; do not infer false/defaults from old absence. Effective
  endpoint defaults are resolved on the source, never invented for missing old RPC URLs.
- `HealthcheckEntry.LastChecked` is the exact source timestamp. Age uses frozen `m.now`,
  never `ConnectionStatus.LastSuccess` or snapshot receipt. Keep status threshold state
  distinct from last attempt status/reason. `DiagnosticURL`/`DiagnosticReason` are
  idempotent shared boundaries; do not reintroduce raw errors, URL credentials or DNS
  into diagnostic View/transport. Provider-wide lifetime counters must not be relabeled
  as target/request/alias metrics by later correlation work.

### Task TUI-06: Add Searchable Recent Requests And Identity-Aware Investigation

Status: completed

Kind: improvement

Priority: P2 — logs/payloads alone make request correlation and tenant diagnosis difficult.

Suggested agent: bounded request metadata and terminal search implementer

Dependencies: TUI-05

Primary ownership: recent accounting/request metadata propagation, RPC, dashboard
request pane/detail/search and filters, focused production-path tests/docs.

Finding: recent events support P95 but are not browsable; request identity is absent
from accounting.Event. Payload browsing depends on optional disk logging. Usage
retains tenant/client dimensions but does not show them, yielding indistinguishable
rows. Existing lists have no text search or direct request-ID correlation.

References: accounting.Event/Recent ring; HTTP accounting event creation; dashrpc
Snapshot.Recent; dashboard renderUsage/filteredLogs/payload lists and key handlers.

Requirements:

1. Add a Requests pane over bounded existing recent metadata, independent of payload
   logging. Carry request ID through the real completion path plus tenant/client,
   public model, resolved provider/model, operation/status/duration/token data.
   Do not store prompts/credentials. Label completion-only retention/cap explicitly;
   do not invent unavailable attempt/in-flight data.
2. Provide request detail and search/filter by ID/client/tenant/model/provider/status.
   Reuse a bounded text-input/filter mechanism for logs/payload lists where their
   metadata supports it. Show filter state, clear/reset and no-matches messages.
3. Make usage identity visible in detail/columns or a clear aggregation choice.
   Add request→logs/payload navigation by request ID, preserving return context and
   explaining unavailable/disabled/expired data. No unbounded history fetch or export.

Acceptance criteria:

- Actual inference→accounting→RPC→TUI test verifies ID and metadata for direct/alias
  JSON/SSE and errors, without payload logging or sensitive body exposure.
- Mounted model keyboard tests cover search editing/cancel/clear, combined filters,
  stable selection during refresh, pause and compact layout, correlation success
  and absent data. Same model/status under distinct clients/tenants is distinguishable.
- Recent storage remains bounded and old snapshots render available metadata safely;
  existing tabs/shortcuts remain discoverable with the new Requests tab.

Verification: `go test -race ./internal/accounting ./internal/httpapi ./internal/dashrpc ./internal/dashboard`,
frontend compatibility if affected, default/final gates.

Necessary scoped discoveries (recorded before implementation):

- Owned by this fresh sequential TUI-06 session; TUI-01–05 and all pertinent
  lifetime/rate/identity/pause/layout/provider contracts, AGENTS.md and the requested
  project task-as-you-go skill have been read. Preserve the extensive dirty baseline.
- `handler.go` has one deferred completion accounting boundary, already carrying all
  requested dimensions except request ID. `dashrpc.Recent` aliases that event; add
  the optional ID there and check the frontend schema rather than adding a feed.
- Requests will be numbered tab 5, preserving 1–4. Snapshot-local request/detail and
  Usage top-row detail reuse pause/identity/layout contracts; no new polling worker.
- Local AND metadata search (bounded editor, field-qualified terms) will operate on
  Requests, cached Logs and the existing capped Payload list. Exact correlation is
  separate from substring search, keeps a single return context, and describes
  missing/disabled/expired records without expanding history. Payload reads retain
  existing request-slot cancellation/generation and paused-action rules.
- Usage currently only has a top-row position. Enter will inspect that exact grouped
  row with full tenant/client/model/operation/status identity, avoiding ambiguous
  truncated identity columns at 80x12.
- Log attributes are flattened unescaped text, so exact correlation cannot safely
  infer ID boundaries from them. Add an optional structured request ID from the
  logger's pinned attribute through log JSON; old logs without it stay unavailable
  for exact correlation. Search logs by structured ID/level, not guessed attributes.
- Rejected model requests use accounting sentinel `Model` values. Preserve those
  billing keys, but add optional `PublicModel` completion metadata for the actual
  submitted model; old snapshots fall back to `Model`. No inferred target on
  pre-dispatch/transport failures. Usage detail n/N traverses every exact group,
  including rows below the final top-scroll position.

Completion evidence:

- Changed: `internal/accounting/accounting.go`, new `recent_identity_test.go`;
  `internal/httpapi/handler.go`; `internal/observability/logs.go`, new
  `log_identity_test.go`; dashboard `dashboard.go`, `input.go`, `layout.go`,
  `payload.go`, `remote.go`, `layout_test.go`, `payload_test.go`, new `recent.go`,
  `search.go`, `correlation.go`, `recent_http_test.go`, `search_test.go`;
  `web-ui/src/types.ts`, `types.test.ts`; CLI dashboard long help, `docs/design.md`
  (Input Contract For Requests/Search, Dashboard Recent Completions And Correlation),
  `website/docs/operations.md` (Dashboard Requests And Search and key table), this record.
  Existing `dashrpc.Recent` aliases Event, so no extra endpoint or RPC builder is needed.
- Added optional completion `RequestID` and `PublicModel` at the one deferred HTTP
  accounting boundary. Existing accounting Model/billing keys and rates remain intact.
  Structured optional log `request_id` comes only from the pinned logger attribute;
  JSON preserves it without parsing unescaped attributes. The cap is still 200 global
  completions; no prompts, response bodies, credentials, attempt state or in-flight
  history is added. Resolved model is the existing configured target model identifier,
  not the wire upstream_name; missing targets stay unavailable. HTTP status is the
  response status, including SSE, rather than a fabricated stream outcome status.
- Tab 5 is Requests, preserving 1–4 and reversible bracket navigation. The pane
  labels completion-only/cap scope and displays newest-first metadata; Enter captures
  one immutable completion with wrapped, paged identity/target/status/timing/token
  detail. List cursor/top anchors survive matching arrivals and fall back predictably
  on disappearance. Usage Enter inspects the top exact grouped row with quoted tenant
  and client; n/N cycles all groups, including the final rows at any viewport height.
- Search is local AND/case-insensitive metadata matching, optionally field-qualified,
  with 256 printable-character editing and one applied query per searchable pane.
  Unknown fields match nothing. Requests supports id/client/tenant/model/resolved/
  provider/status/op; Logs structured id/level; Payloads id/model/resolved/provider/
  status/method/path. Queries combine with pane status/level filters. Visible search,
  clear and no-match states, cell-bounded caret scrolling and apply/cancel/clear/quit
  hints share the existing footer budget; connection status stays independently live.
  Editor q/p/h/?/numbers/brackets never fall through to browse/help/pause/decisions.
- Request l/v correlation uses exact ID, bypasses target query/status/level filters
  with an explicit ID-only label, and keeps just one return context. Esc closes target
  detail then restores request detail/scroll/zoom and target list identity/filter state;
  ordinary navigation abandons correlation. Missing fetcher, disabled logging, old
  missing IDs and missing/expired/out-of-cap records are explained. Payloads remain
  bounded at 100 even for an oversized list response; existing request slots handle
  cancellation/late replies and pause. Search and snapshot-local detail work paused;
  new remote reads wait for explicit resume, with no mutation replay or history fetch.
- `TestRecentInferenceHTTPAccountingRPCAndTUI` builds a real App and local upstream,
  issues 16 actual direct/alias × JSON/SSE × 200/400 × two-client/tenant requests,
  reads authenticated HTTP snapshots and converts to the TUI. It verifies one event
  per inference, caller/response ID, public and configured resolved model/provider,
  tenant/client, operation/status, positive duration/timestamp, input/output/total/cache
  tokens, exact matching TUI detail and real structured-log correlation. Full snapshot
  and rendered detail exclude all prompt/response/inbound/upstream/dashboard secret
  fixtures with payload logging disabled. Another 205 actual rejected requests prove
  generated IDs, submitted public-model preservation, unavailable targets and the
  transported/displayed 200 cap. The accounting regression records 10,000 unique
  request/public-model values under one sentinel grouping, proving detached recent
  copies and no request-ID-driven expansion of billing identities/buckets.
- Keyboard regressions use actual KeyPress/arrow/control messages through model.Update:
  insertion/deletion/Unicode/caret movement, 256-character cap, nontext-key rejection,
  reserved retry/quit, cancel/clear, combined field/status/level filters, no matches,
  refresh during editing, anchored request/log/payload selection, cached paused search,
  frozen snapshot/clock then coherent resume, and request/usage detail at 80x12,
  80x24, 120x30 and 160x48. Tests distinguish same-model/status client/tenant groups,
  reject prefix/flattened-attribute false correlation, exercise target detail and return
  restoration, absent/disabled/expired cases, paused read suppression, late detail
  rejection, explicit navigation exit and old ID-less snapshots. Existing TUI-01–05
  lifecycle/metrics/provider/layout/pause suites pass. Updated only obsolete Usage
  Enter and four-tab wrap expectations; retained existing move-hint regressions.
- Passed required `go test -race ./internal/accounting ./internal/httpapi
./internal/dashrpc ./internal/dashboard`, plus `./internal/observability` (first
  full race: 4.693s / 20.379s / 1.425s / 9.072s / 1.129s; final dashboard 5.616s,
  other race packages cached). Passed final `make vet test`, `make docs-contract`
  and `git diff --check`. Passed frontend compatibility tests: 11 files, 195 tests,
  and `pnpm --filter @aiproxy/web-ui typecheck`; optional event/log IDs, PublicModel
  and cache tokens survive schemas, while old missing IDs remain undefined.
- Limitations/optional skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset; optional
  DB prerequisite suites remain skipped. No real DB was needed or created. Local
  test servers/temp configs clean up through test lifetimes; no actual provider or
  credential was used. No artifact build or new PTY experiment was needed; frontend
  checks completed before Go verification, with no shared-asset build overlap.
  TUI-08 owns fresh PG17/no-skip, full-repo race/integration and real-binary PTY gates.
  No TUI-06 blockers. Incoming dirty baseline and all other task statuses preserved;
  CHANGELOG untouched; no commits or subagents.

Next-agent request/search/key contract:

- `recent.go` owns snapshot-local Requests (`bottomTabRequests`, number 5), its
  requestCursor/requestOffset anchors and copied `requestDetail`; `usageDetail`
  copies one exact grouped row and n/N traverses displayed groups. Both join
  `hasDetail`/`closeDetail` and full-body wrapped metadata rendering; their common
  `metadataScroll` is detail-local. No new async slot is necessary. Event.RequestID
  and PublicModel have capitalized optional JSON keys; legacy Model remains the
  accounting grouping key. Log JSON request_id is optional snake_case, and old
  flattened Attrs must never be promoted into exact correlation metadata.
- `search.go` owns bounded `searchState`, per-pane `queries`, metadataMatch and
  editor controls. InputSearch dispatch stays after global Ctrl+C/Ctrl+R but before
  all q/help/pause/navigation. `/` starts only a searchable focused list; Enter
  applies, Esc cancels, Ctrl+U clears edit/applied query. Ctrl+A/E or Home/End and
  arrows/Backspace/Delete edit; printable ? is text, not help. Keep the editor prompt
  and `searchControls` cell budgets paired so the caret and controls remain visible.
  Searching is local, uses current immutable/cached data, and may run while paused.
- Requests use requestPublicModel (PublicModel when present, otherwise Model).
  `requestIdentity` compares immutable completion metadata, including timestamp and
  ID, to distinguish caller-reused IDs. Detail remains its captured completion even
  after ring eviction; return to the list uses normal anchored/clamped selection.
  The existing 200 cap is independent of rates. No inferred target, attempt count,
  token availability or stream outcome belongs in completion display.
- `orderedPayloads` now means displayed order AFTER local query/error/correlation
  filtering; all cursor/offset/payloadAt paths must use it. `payloads` remains the
  capped raw last-list cache. Use `IsError()` for payload error semantics. Search does
  not issue list requests; existing remote status filters still use requestSlot.
- `correlation.go` retains one `correlationContext`: captured request + detail scroll/
  zoom and target selection/top identity. l/v enters matching Logs/Payloads; ID-only
  mode bypasses target filters and suppresses target filter/order/search mutation.
  Esc from target detail uses ordinary closeDetail (cancel/clear late buffers), then
  Esc from its list calls leaveCorrelation(true). Tab/number/bracket paths call
  leaveCorrelation(false) after closing detail. Keep target filters intact; payload
  list entry/exit invalidates prior generations and clears its paused list buffer.
- TUI-07 must preserve numbered 1–5 navigation, search mode ownership, paused remote
  read suppression, correlation return state and existing close/cancel boundaries
  while changing payload/block text or finding actions. Full captured text remains
  exclusively in existing authenticated payload/block detail fetches, never recent
  completion metadata or a search index. No decision behavior was changed here.

### Task TUI-07: Make Payload Inspection Readable And Block Decisions Explicit

Status: completed

Kind: improvement

Priority: P2 — important text is truncated and decisions silently apply to all findings.

Suggested agent: terminal inspection and per-finding decision UX implementer

Dependencies: TUI-06

Primary ownership: dashboard payload/block detail views and async decision state,
focused tests/docs; RPC only if necessary (existing API accepts finding hashes).

Finding: detail rendering truncates every long line without horizontal navigation,
hiding content. requestBlockDecision always sends all finding hashes; a/s/d and
numbered tab keys can mutate every finding, with no selected-finding scope. Take-once
capture consumption and persistent effect need clear context during decisions.

References: payload.go (renderPayloadDetail), blocks.go (blockFindingSHAs,
handleBlockKey, requestBlockDecision, renderBlockDetail), dashrpc decision schema.

Requirements:

1. Support wrapped long-line inspection or horizontal scrolling with visible position;
   retain full captured text within the existing size cap and show truncation if
   upstream capture/pretty output was capped. Keep headers/actions visible and
   cell-safe rendering at supported sizes.
2. Add finding selection and make a/s/d apply to the selected finding's hash only.
   An explicit bulk action may remain only with clearly displayed scope and deliberate
   confirmation; numbered tab shortcuts must never issue decisions.
3. Show take-once semantics before opening and persistent future-match effect (no
   automatic replay). Handle pending/failure/success and repeated/late results without
   accidental duplicate or wrong-finding decisions. Preserve operator-only RPC guards.

Acceptance criteria:

- Long JSON strings/findings and Unicode content can be inspected end-to-end within
  known cap; wrap/scroll/resize does not hide action scope or corrupt navigation.
- Multi-finding fixture proves only selected hashes are sent, bulk action (if added)
  cannot happen by ordinary navigation, errors permit deliberate retry, late results
  do not update another capture, and take-once messaging is accurate.
- Server authorization/no-secret-leak regression coverage remains intact; full
  detail text is only shown to the existing authenticated operator surface.

Verification: `go test -race ./internal/dashboard ./internal/dashrpc ./internal/httpapi`,
default/final gates.

Necessary scoped additions (recorded before implementation):

- Owned by this fresh sequential TUI-07 session; TUI-01–06 are completed. Read
  shared rules, preceding integration contracts, AGENTS.md and the requested project
  task-as-you-go skill. Preserve all incoming dirty work and other statuses.
- Share a cell-safe captured-text inspection helper with wrapped paging, fixed
  metadata/scope/status chrome and explicit capture/pretty-output cap messaging.
  Keep full text exclusively in the existing authenticated detail fetches.
- Replace all-finding hash collection with n/N selected-finding navigation and
  a/s/d for that exact hash only; omit bulk actions. Bind acknowledgments to their
  generation, capture and hash; suppress pending/repeated successful actions while
  allowing deliberate retry after failure. Preserve pause and close/cancel semantics.
- Update CLI help, operator/design docs and meaningful rendering/async/authorization
  regressions. No endpoint/schema addition is needed. TUI-08 owns fresh PG17 and
  final full-repository/PTY gates; this session discloses optional DB skips.
- Source inspection confirms block snippets are capped before transport without a
  truncation flag or original length. Always label them server-capped with truncation
  unknown, rather than infer completeness or add an API. Payloads have body truncation
  flags and a pretty-output marker; surface both explicitly. Escape terminal control
  bytes and invalid UTF-8 in captured text so they remain inspectable without executing
  terminal sequences. Lock finding selection while a decision is pending.

Completion evidence:

- Changed: `internal/dashboard/blocks.go`, `payload.go`, `dashboard.go`, `input.go`,
  `layout.go`, new `inspection.go` and `inspection_test.go`; existing
  `blocks_test.go`/`layout_test.go` scope expectations; new
  `internal/httpapi/dashboard_finding_test.go` and the quarantine test adapter's
  previously omitted finding SHA/description; CLI dashboard long help,
  `docs/design.md` (Captured Text And Selected-Finding Decisions),
  `website/docs/operations.md` (Dashboard Payload And Block Inspection), this record.
  No production HTTP/RPC/schema/frontend changes were required.
- Replaced clipped payload/block lines with terminal-cell hard wrapping and line/
  page/Home/End inspection. Fixed headers retain record/finding identity, decision
  scope/status; fixed position/cap row and context footer remain visible even at
  80x12 (one block content row). Full metadata/IDs/hashes are also scrollable.
  Controls and invalid UTF-8 bytes render as printable escapes; valid CJK, combining
  characters and emoji-ZWJ graphemes remain intact. Resize rewraps/clamps without
  changing selected record/finding. Existing payload byte cap remains 64 KiB.
- Payload notices distinguish body truncation flags (request, upstream_request,
  response), pretty-output truncation, unavailable capture status and no reported
  truncation. Block notices explicitly say server-capped snippets/truncation unknown;
  no invented completeness flag or hash reconstruction. Captured text remains in
  authenticated detail state only, outside recent metadata/search indexes.
- n/N cycles findings; a/s/d transmits a one-element list containing only the exact
  selected valid lowercase SHA. No bulk action exists. Pending locks selection and
  suppresses duplicate actions; per-captured-hash results preserve status across
  navigation and suppress repeated successful actions, including equivalent hashes.
  Deliberately changing action replaces its recorded effect. Errors/invalid ack
  (ok/action/count) show unknown outcome and permit deliberate retry. Missing/invalid
  hashes and empty captures issue no decisions.
- List warns before Enter that it consumes a take-once capture; detail and docs
  explain persistent GLOBAL future-match behavior, identical-hash scope, no original
  request replay, and that cancellation cannot undo server consumption/persistence.
  Acknowledgments validate generation/capture/action/hash before buffering/applying;
  close clears per-hash state and paused buffers. Existing pause, lifetime, modal,
  numbered 1–5 navigation and Requests correlation return contracts are preserved.
- New mounted keyboard regressions cover exact three-finding hashes/actions, pending
  duplicate keys/selection lock/manual retry, failure/retry, invalid acknowledgments,
  missing/malformed/uppercase hashes and no findings, equivalent-hash status, deliberate
  replacement, all numbered/tab/bracket/Enter/Esc navigation with zero mutations,
  help input ownership, wrong-hash replies, same-capture close/reopen with a new
  finding/generation, duplicate/late results and paused result drain/close.
- Rendering regressions reconstruct every character of near-64-KiB Unicode text
  across all pages with both detail header budgets at 80x12, 80x24, 120x30 and 160x48
  equivalent body sizes. Mounted long-JSON/finding tests resize repeatedly through
  those sizes, reach final content, reverse paging and retain scope/cap/action/ID
  chrome. They also check all three body-cap flags, pretty-cap notices and terminal
  control/invalid-byte escapes. Existing full viewport/help, lifetime, pause/identity,
  Requests/search/correlation and confidential HTTP completion tests pass.
- New non-DB production HTTP + authenticated dashrpc client test starts with a real
  blocked inference, adds another captured finding, proves list/denial confidentiality,
  take-once access, single-hash durable allow/redact/deny, unchanged other-hash denial,
  secret-free exception files, zero upstream calls on decisions and correct behavior
  only on a subsequent explicit request (including placeholder redaction). It tests
  missing/invalid/API-like tokens and rate-limited denial without consuming capture
  or mutating disk. Initial fixture expected six 401s; the existing global five-token
  burst correctly returned 429 on the sixth. The fixture now explicitly verifies
  that existing rate-limit boundary instead of weakening it.
- Passed focused regressions, then required
  `go test -race ./internal/dashboard ./internal/dashrpc ./internal/httpapi`
  (30.367s / cached / 13.517s), `make vet test`, `make docs-contract`, and
  `git diff --check`. All build/test/check commands were serialized; no artifact
  build or frontend schema consumer check was needed for this TUI-only contract.
- Limitations/optional skips: `AIPROXY_TEST_DATABASE_URL` confirmed unset. Optional
  DB suites, including the stored-system-admin/organization-role operator matrix,
  remain skipped; its source/guards and existing coverage are preserved. TUI-08 owns
  its fresh PG17/no-prerequisite-skip run, full-repo race/integration/frontend and
  real-binary PTY gates. No DB fixture, live provider/credential or new PTY experiment
  used here. No TUI-07 blockers. All incoming dirty work/other statuses preserved;
  CHANGELOG untouched; no commits or subagents.

Next-agent inspection/decision integration contract:

- `renderInspection` owns fixed headers plus one position/cap row inside the pane
  frame. Payload has two header rows, block three; block paging subtracts one extra
  row in `detailVisibleRows`. `blockDetailScroll`/`payloadDetailScroll` now index
  wrapped rows; resize rewraps and clamps. Never reintroduce per-line clipping as
  the only route to captured content. Header truncation has a full-body counterpart.
- `blockFindingCursor` indexes the immutable open capture. n/N wraps and resets text
  scroll; it cannot move while `blockDecisionPending` is set, including while an
  accepted acknowledgment waits paused. All normal global help/back/quit/navigation
  ownership stays in `input.go`. No new worker, endpoint or result class was added.
- `blockDecisionSHA` is the in-flight hash; `blockDecisionMsg.sha` must match it in
  addition to generation/capture/action. `blockDecisionResults` keeps only the latest
  result per captured hash during that detail session. It is cleared on detail
  application/close. Success requires ok=true/requested action/count=1; same action
  is suppressed until replaced, failures allow explicit retry. No automatic replay.
- Block truncation remains unknowable from existing transport; retain its explicit
  unknown notice. `inspectionText` escapes controls/invalid bytes without altering
  valid Unicode. Payload render retains the existing cap and surfaces the existing
  pretty marker/body flags. No text is added to snapshots, Requests, logs or search.
- Independent review must run the preserved stored-role operator matrix with its
  own PG17 fixture and exercise a real PTY at compact/large sizes. Existing Requests
  correlation close/return and search ownership tests already run with these details.

### Task TUI-08: Independently Verify The Complete Operator Workflow

Status: completed

Kind: improvement

Priority: P1 — combined data, lifecycle and interaction contracts need independent audit.

Suggested agent: independent integration reviewer, not a TUI-01–07 implementer

Dependencies: TUI-01, TUI-02, TUI-03, TUI-04, TUI-05, TUI-06, TUI-07

Primary ownership: all plan changes/acceptance evidence, scoped corrections and this record.

Finding: passing isolated changes can still leave misleading data, conflicting keys,
inaccessible panes or terminal/polling leaks when combined.

References: preceding requirements, source/test paths and Completion evidence.

Requirements:

1. Audit each criterion against actual source/data production, RPC and interaction
   tests, not just agent reports. Check old-snapshot fallbacks, windows/cost identity,
   selected-record stability, permissions, bounded retained/pending state, cancellation
   and all compact/detail/help/search/decision key modes.
2. Run final shared gates with owned fresh PG fixture/no DB prerequisite skips and
   cleanup. Exercise the real binary via PTY for attach, navigate/help/search/detail,
   disconnect/recover and quit, using synthetic local fixtures at representative sizes.
3. Record necessary scoped corrections before implementing; add meaningful regressions.
   Include per-task acceptance/status audit, actual verification and limitations.
   Preserve baseline, CHANGELOG and previous task records.

Acceptance criteria:

- Eight tasks completed with fulfilled criteria/Completion evidence, passing final
  gates and no unresolved blocker. Real CLI exits and reconnects predictably.
- Metrics/data/docs agree; no unexpected sensitive metadata exposure or unbounded
  request/pending state. All owned fixtures removed and prior work preserved.

Verification: all shared final gates, PTY checks and independent source/record audit.

Review ownership/start:

- Fresh independent TUI-08 session; implemented none of TUI-01–07. Read the entire
  record, AGENTS.md and requested project task-as-you-go skill. Dependencies completed.
- Inspected extensive incoming dirty baseline and Docker containers before work;
  existing PostgreSQL containers are unrelated and will not be used or modified.
  Audit and final gates use an owned new PG17 fixture, serialized Go DB packages
  and frontend builds completed before Go asset readers. No nested agents/commits.

Necessary scoped correction (recorded before implementation):

- TUI-03/06 integration: `handlePayloadKey` changes errors-only filtering immediately
  but leaves cursor/top indexed into the prior displayed list until its asynchronous
  response arrives. Enter during a held replacement fetch can index past the filtered
  list and panic. Existing filter-order tests use the first row and deliver replies
  before opening detail, missing this transition.
- Require synchronous identity reconciliation/clamping when the local status filter
  changes, and defer Enter while the replacement list is not yet known/visible.
  Add mounted-key regressions holding the fetch result with both removed and retained
  selected identities, then verify response application and exact opened identity.
  Preserve bounded fetch/generation/pause/correlation behavior; no new workflow.

Completion evidence — independent acceptance/source audit:

- **TUI-01: accepted.** Read `cmd/aiproxy/dashboard.go`, `dashboard_lifecycle.go`,
  `internal/dashboard/program.go`, their lifecycle tests and pane command paths.
  One snapshot worker, capacity-one manual retry channel, 2s HTTP timeout,
  2/4/8/16/30s retry delays and child-context cancellation agree with CLI help/docs.
  Program completion waits for terminal cleanup; attachment joins the canceled poll
  worker. Initial config/token/local-transport errors remain explicit. Denial suspends
  snapshot polling and gates all five pane RPCs until a successful manual snapshot;
  no decision replay. Held HTTP cancellation, initialization failure, configured I/O,
  classification/redirect, retry coalescing and every pane deadline/body-read cancel
  regressions passed. Rebuilt-binary quit/reconnect/denial evidence is below.
- **TUI-02: accepted.** Traced `Aggregator.Record` → fixed `[901]rateBucket` ring /
  atomic paired `BillingSnapshot` → `dashrpc.BuildContext/RuntimeSource` → JSON →
  `SnapshotFromTransport` → `measurements.go`, provider/usage/cost rendering. Rate
  windows are the last 60/300 complete seconds; ongoing second excluded, zero/future
  rate timestamps normalized, late expired buckets cannot overwrite live slots.
  Rate tests independently count 15,000 events and half-open nanosecond boundaries,
  idle expiry and concurrent detached copies, rather than deriving expected rates
  from the recent ring. Transport tests independently expect 600/min = 10/s and
  P95 590ms/200. Shared `usagecost` reconciles exact tenant/client/public model/
  operation/status and all token/count fields, with used-price gaps unavailable;
  aliases a/b/direct have isolated $0.001/$0.003/$0.006 estimates. Provider subtotals,
  retained upstream regrouping, inclusive minute-bucket retention/expiry and old-RPC
  unavailable fallbacks are covered. Lifetime provider counts and positive-duration
  last-200 untimed P95 are explicitly distinct from retained costs and global rates.
  Inspected web Zod optional/nullish metadata schemas and their consumer regressions.
- **TUI-03: accepted after the scoped filter-transition correction below.** Audited
  `applySnapshot`, `selection.go`, `pause.go`, all five `requestSlot` result paths,
  payload/block handlers and `state_test.go`. Stable identities, removed-index clamp,
  provider enabled-group changes, exact five-dimensional usage identities, tenant
  name fallback and log follow/pinned behavior survive refresh. Pause detaches mutable
  viewers, retains one latest snapshot/result per class and freezes display time;
  independent connection receipt remains live. Generation plus ID/action/hash checks
  precede buffering/applying; supersede/close cancels work, clears buffers and rejects
  duplicates and same-ID reopened results. No queued mutation replay. Existing tests
  include real channel-controlled cancellation, not just injected matching IDs.
- **TUI-04: accepted.** Read `input.go`, `layout.go`, `help.go`, render/layout/scroll
  helpers and mounted viewport/key tests. Compact (<30 rows), stacked and zoom budgets
  preserve the tab strip, cell-safe pane bounds and two-line footer/measurement chrome.
  Help owns keys; search precedes printable browse actions; Ctrl+C/Ctrl+R remain global.
  Esc unwinds help/detail/correlation/zoom; numbered 1–5/Tab/brackets close detail and
  never decide. Brackets are inverse in numbered order. Persistent matrices assert
  exact height and terminal-cell width at 80x12/80x24/120x30/160x48/250x60, populated
  panes, long errors, CJK/combining/ZWJ/ANSI text, selected-row visibility and resize
  through detail/help. Independent PTY screen assertions verify final footer visibility
  and meaningful navigation/detail content, not merely process survival.
- **TUI-05: accepted.** Traced stored healthcheck LastChecked through app
  `healthcheckSourceFor`, `RuntimeSource.Snapshot`, remote conversion and provider detail.
  Read diagnostic serializer/sanitizers and provider tests: pending/success/503,
  standalone/no-alias/no-probe/disabled/removed provider, model mapping/protocol/
  capabilities, custom/default/disabled/unknown affinity and provider-wide counter
  semantics. URL userinfo/query/fragment/nonstandard paths and raw error reasons are
  sanitized at source and display; credentials/references/auth/body expectations are
  omitted. Old absent metadata stays unknown. The blocking resolver regression makes
  zero DNS calls during View/navigation/quit; production provider rendering contains
  no resolver. Actual App/HTTP probe tests establish timestamp equality and 37s age,
  and test full serialized response/detail absence of secret fixtures.
- **TUI-06: accepted after the filter-transition correction.** Traced deferred
  `httpapi.Handler.ServeHTTP` completion recording (ID/PublicModel/identity/resolved
  target/tokens/duration/status) and structured pinned log request IDs through the
  existing RPC to `recent.go`, `search.go`, `correlation.go` and usage detail. Recent
  cap remains 200 independently of payload logging and rates; IDs are not billing
  dimensions. Actual App/local-upstream tests cover 16 direct/alias × JSON/SSE ×
  200/400 × two-client/tenant requests, plus 205 rejected requests with generated IDs,
  submitted models, absent targets and cap enforcement. Secrets/bodies do not enter
  completion transport. Search is bounded at 256 printable characters with AND fields,
  explicit cancel/clear/no-match state; exact correlation never parses flattened attrs
  or accepts ID prefixes, has one return context and capped 100-entry payload cache.
  Usage detail distinguishes all exact grouping dimensions. Read mounted editing,
  filter/pause/refresh/late-reply/absent/disabled/expired/return-context tests; verified
  the final binary's real search input, logs/payload correlation and back navigation.
- **TUI-07: accepted.** Audited `inspection.go`, payload/block renderers and decision
  workers, plus `inspection_test.go` and real HTTP future-effect regression. Wrapped
  paging retains within-cap text; fixed ID/scope/status/cap rows survive 80x12. Tests
  reconstruct near-64KiB text across pages and separately verify mounted resize,
  final content, body/pretty caps, control-byte escaping and invalid UTF-8. Server
  snippet truncation remains explicitly unknown. n/N selects one valid lowercase SHA;
  a/s/d sends exactly one hash, with no bulk/numbered mutation. Pending locks selection;
  duplicate successful actions are suppressed, failures allow deliberate retry, and
  generation/capture/action/hash guards protect acknowledgments through pause/close.
  Real HTTP tests verify take-once, durable single-hash effects, other-hash preservation,
  no upstream replay and secret-free exception files. Inspected `requireDashboardOperator`
  and its stored-role matrix: all seven routes authenticate before read/take/write.
  The owned-PG run executed that matrix (84 route/actor cases) with current stored
  system-admin authority and denied ordinary/org-admin/demoted/disabled/deleted users.
- **TUI-08: accepted.** All seven implementation tasks retain completed status and
  historical evidence. One independently found P1 integration defect was reproduced
  and corrected; no remaining acceptance blocker or expanded roadmap work.

Correction/evidence:

- Changed only `internal/dashboard/payload.go`, new
  `internal/dashboard/filter_transition_test.go`, the pending-list paragraph in
  `website/docs/operations.md`, and this task record. Filter changes now reconcile
  selected/top identities synchronously; Enter waits while the list is unknown.
- Before fix, `GOFLAGS=-p=1 go test ./internal/dashboard -run
TestPayloadFilterPendingTransitionKeepsSafeIdentity -count=1` failed: removed-row
  case opened a hidden cached record; retained-row case panicked in `payloadAt`
  (`index out of range [2] with length 2`). The regression withholds the replacement
  result, uses mounted keys and checks actual detail identity after application.
  After fix, `GOFLAGS=-p=1 go test -race ./internal/dashboard -count=1` passed (40.292s).
  The final PTY fixture additionally holds the errors-only response for 800ms and
  presses Enter from a previously selected final row; it stays alive, waits, then
  opens the correctly clamped `req-1` detail at all four sizes.

Exact final gate and fresh-database evidence:

- Repository root for all commands: `<repo-root>`.
- First inspected `docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'`;
  unrelated PG16/PG18/developer containers were left untouched. Created only:
  `docker run --detach --rm --name aiproxy-tui08-20260926-final-c7e9
-e POSTGRES_USER=tui08_test -e POSTGRES_PASSWORD=tui08_fixture_only
-e POSTGRES_DB=tui08_20260926_c7e9 -p 127.0.0.1::5432
--health-cmd 'pg_isready -U tui08_test -d tui08_20260926_c7e9'
--health-interval 1s --health-timeout 5s --health-retries 60 postgres:17`.
- Container `28c8eb1471ab`, healthy before testing; Docker inspection showed only
  `127.0.0.1:39638→5432`. `psql ... -c 'select version(), current_database();'`
  confirmed PostgreSQL **17.11** and unique DB `tui08_20260926_c7e9`.
- Every DB/Go gate received explicit
  `AIPROXY_TEST_DATABASE_URL='postgres://tui08_test:tui08_fixture_only@127.0.0.1:39638/tui08_20260926_c7e9?sslmode=disable'` // pragma: allowlist secret
  and `GOFLAGS=-p=1`; no overlapping command against this DB.
- Passed `pnpm --filter @aiproxy/web-ui test`: **11 files / 195 tests**;
  `pnpm --filter @aiproxy/web-ui typecheck`; then `GOFLAGS=-p=1 make build`.
  UI asset builds finished before Go readers. Frontend source was unchanged by the
  review correction; integration subsequently reran frontend typecheck/build.
- Passed `make vet test`, `make test-race`, `make integration`, `make docs-contract`
  and `git diff --check`. After the correction, reran the exact serialized final
  command with the explicit DB environment:
  `GOFLAGS=-p=1 make vet test test-race integration && make docs-contract && git diff --check`.
  Final changed-package times: CLI unit 13.902s/race 61.907s, dashboard unit 2.540s/
  race 33.409s; hermetic binary integration 1.067s. Other already-verified unchanged
  packages used Go cache on this last run. No concurrent UI build/Go asset reads.
- Independently proved uncached database execution with
  `GOFLAGS=-p=1 go test -race -count=1 -json ./internal/store ./internal/dbmerge
./internal/httpapi ./internal/app`, using `<repo-root>/tmp/tui08-db-gate.py` to capture
  events and assert all four packages executed, zero skips/failures. Evidence log:
  `<repo-root>/tmp/tui08-db-gate.jsonl`. Pass events including nested subtests:
  **store 88 (29.174s), dbmerge 6 (1.374s), httpapi 621 (248.479s), app 107 (23.067s)**,
  total **822**, zero SKIP/FAIL/DATA RACE/prerequisite skips. Confirmed explicit
  `TestDashboardOperatorRoutes` PASS (10.79s) in the log. The later correction touched
  no DB/source/HTTP code; final full gates also passed with the same explicit fixture.
- Initial `make vet test` exceeded the harness's default 120s timeout while store
  tests were running; checked processes had stopped, reran with a 600s tool timeout
  and passed. This was not counted as a passing run or masked as a test skip.

Final real-binary PTY evidence:

- Tested rebuilt `dist/aiproxy` (`v0.26.0-67-gbe907db-dirty`) against synthetic
  loopback HTTP fixtures with inline temporary config, no actual upstream credentials.
  Inspected the previous lifecycle script before reuse. New independent script
  `<repo-root>/tmp/tui08-pty.py` uses a real PTY and pyte screen reconstruction from
  `<repo-root>/tmp/tui08-venv` (pyte 0.8.2) to assert current-screen content/footers;
  raw evidence is `<repo-root>/tmp/tui08-pty-{80x12,80x24,120x30,160x48}.raw`.
- Final command `<repo-root>/tmp/tui08-venv/bin/python <repo-root>/tmp/tui08-pty.py`
  passed **four complete workflows** at 80x12, 80x24, 120x30 and 160x48: attach;
  Shift-Tab/Enter provider detail/end; help input isolation/end/back; Requests tab;
  literal q/p/h/?/digits/brackets in search, cancel/apply; exact-ID log detail and
  payload detail/back; long Unicode JSON end; pending filter regression; take-once
  two-finding detail/end, selected allow/redact and repeat suppression; numbered tab
  without mutation; pause across snapshot arrival/resume; 503 stale/help navigation;
  Ctrl+R recovery; q exit. Each workflow made exactly two single-hash decisions and
  one take-once read. Exit times **0.062/0.063/0.062/0.063s**, exit code 0, zero polls
  over the subsequent 2.2s and exact termios restoration in every case.
- Final `python3 <repo-root>/tmp/cli-dashboard-pty-tui01.py` passed automatic 503
  recovery, 403 denial/no polling for 2.5s, Ctrl+R recovery, q/Esc/Ctrl+C/SIGTERM exits
  at **0.052/0.102/0.052/0.051s**, all exit 0, exact termios restoration and zero
  subsequent polls. Together: **8 final PTY sessions** against corrected code.
- Initial new-script assertions were corrected before the final passing run: screen
  drain originally stopped when the header arrived before the rest of a large frame;
  a final-content marker could legitimately hard-wrap across rows. Added frame drain
  and a line-separated fixture marker, preserving production code/viewport assertions.
  Each failed and successful script run cleaned its child processes and listener.

Cleanup, preservation and limitations:

- `docker stop aiproxy-tui08-20260926-final-c7e9` triggered owned `--rm` cleanup.
  First immediate inspect observed Docker's transient `removing` state; a subsequent
  check corrected the fixture assertion's case-sensitive error matching. Final
  inspection confirmed **no such object**, and **no such volume** for the owned
  anonymous volume `c9afb1ff20411117c9d9e601b692c3bb24ecaed236a5baf3869454d2c6b9554e`.
  No prior/developer fixture was stopped, reset or reused. Test-local listeners/
  processes/temp files clean through their lifetimes; temporary evidence/scripts
  remain under `<repo-root>/tmp`. Removed only the new build-created `.gitkeep`;
  ignored binary/assets remain available.
- Linux PTY/synthetic local servers and automated cell/key tests establish these
  contracts; this is not a human usability study, live-provider test or exhaustive
  cross-terminal/OS audit. Near-cap reconstruction/resize coverage is automated Go
  rendering; PTY scenarios use representative within-cap Unicode captures. Existing
  lifetime/retained accounting identity maps are not newly fixed-cardinality stores;
  new rate buckets, recent completions, search and pending work are explicitly bounded.
- Incoming BOUNDARY/SAFE/LIFE/FLOW and TUI-01–07 work/records preserved. Final status/diff
  review shows only the scoped reviewer additions above. **CHANGELOG.md untouched**
  (empty diff), no commits, no nested agents. All **8/8 tasks completed**; no blocker.

## Coordinator Final Closure

- Confirmed all eight tasks have completed statuses and Completion evidence. Reviewed
  the independent per-task acceptance audit, scoped filter-transition correction,
  final gate/PTY results, fixture cleanup and documented limitations. No unresolved
  acceptance blocker remains.
- Each task ran in a fresh sub-agent session, sequentially; the next task started
  only after the preceding owner returned completed. TUI-08 was independently
  reviewed by a session that implemented none of TUI-01–07.
- Rechecked final worktree, `git diff --check`, and
  `git diff --exit-code HEAD -- CHANGELOG.md`: checks passed, prior work remains
  present, and CHANGELOG is unchanged. No commits were created.

Follow-up: `20260927-010607-cli-dashboard-input-boundaries.md` tracks residual
recent-metadata byte bounds, literal request rendering and hidden Blocks refresh actions.
