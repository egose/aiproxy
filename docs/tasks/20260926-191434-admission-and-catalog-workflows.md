# Admission Reliability And Catalog Editing Workflows

Created: 2026-09-26 19:14:34 local time

## Objective And Product Requirements

aiproxy centralizes provider routing, credentials and organization spending controls.
Operators need unavailable policy data to be distinguishable from unlimited access,
catalog changes to have truthful persistence/activation outcomes, and browser edits
to preserve full model identities and express requested setting changes accurately.
This round addresses four concrete gaps through shared boundaries and regression
tests, improving reliability, security, usability and testability.

Scope: quota-read admission failures, provider aggregate persistence and provider/
alias publication ordering, provider edit serialization, alias target parsing.
Preserve extensive existing dirty work. **Do not edit `CHANGELOG.md`**, commit, or
rework unrelated completed features. Document external contract changes in relevant
public docs and this execution record.

## Analysis, Deduplication And Limitations

- Followed AGENTS.md and
  `task-as-you-go skill`.
- Read-only review `ses_f1f5d762cffe7vwysYnRSZoqmX` inspected quota admission and
  storage callers, provider/alias persistence and activation/view ordering, browser
  forms/services, existing regressions and product contracts. Coordinator checked
  principal source locations and incoming worktree independently.
- Deduplicated against completed `20260926-145524-business-boundary-health-review.md`,
  `20260926-163550-upstream-and-cli-health-review.md`,
  `20260926-173216-admin-lifecycle-integrity.md`, and relevant earlier residual tasks.
  This is not a repetition of retained spend, runtime resource rollback, key PUT
  atomicity, browser auth generations, or CLI model discovery.
- No baseline tests run during analysis. Findings are source-confirmed and require
  local reproduction. This is not an exhaustive dependency/deployment, every quota
  schedule, catalog visibility, or browser accessibility audit.
- Already-documented completion-based overshoot, 30-second spend-cache lag,
  durable ledger retry and distributed activation design remain outside these
  bounded findings. No throughput/performance gain is claimed without measurement.

## Execution And Verification Rules

Execute FLOW-01 through FLOW-05 in order, each in a **fresh sub-agent session**,
never concurrently. FLOW-05 is an independent reviewer. No nested agents. Shared
hotspots include store, admin routes, frontend forms and docs. Each owner sets only
their task `in_progress`, then appends `Completion evidence` with actual files,
commands/results, reproduction and limitations. Mark `completed` only after all
acceptance and checks pass; otherwise record a precise blocker. Add necessary
scoped discoveries explicitly before implementation.

P1 = financial/runtime integrity; P2 = routine operator workflow correctness.
Use existing abstractions/style, apply_patch, and meaningful behavior regressions.
All commands run at `<repo-root>`.

Backend owners and final reviewer must create their **own new disposable PG17**
fixture using README's Docker pattern, after inspecting existing containers: unique
name/database, loopback port, readiness check, explicit `AIPROXY_TEST_DATABASE_URL`.
Never use a developer DB or another task's fixture. Execute packages serially with
`GOFLAGS=-p=1` / `-p 1`, inspect actual DB execution/no prerequisite skips, and remove
only the owned fixture afterward. Frontend-only owners need no database; disclose
any optional DB skips in their default Go checks. No real upstream credentials.

Run required focused checks per task and `make vet test` after nontrivial changes.
Final gates: `make vet test`, `make test-race`, `make integration`,
`make docs-contract`, `pnpm --filter @aiproxy/web-ui test`,
`pnpm --filter @aiproxy/web-ui typecheck`, and `git diff --check`.
Finish UI builds before Go checks consume embedded assets; never run overlapping
builds/suites against the same outputs/DB. Final backend gates use a fresh PG fixture.

Definition of done: all five tasks completed with actual acceptance evidence,
passing integrated checks, truthful docs, owned fixtures removed, baseline and
CHANGELOG preserved, and independent per-task acceptance/status audit.

### Task FLOW-01: Reject Admission When Required Quota Data Is Unavailable

Status: completed

Kind: defect

Priority: P1 — operational storage failures silently remove spending/TPM controls.

Suggested agent: quota admission reliability implementer

Dependencies: none

Primary ownership: `internal/httpapi/quota.go`, quota read callers/tests, public
quota/error documentation; narrowly scoped store/test helpers if needed.

Finding: `quotaRow` turns every GetScopeQuota error into absent quota; `scopeSpend`
turns every SumScopeSpend error into zero. Loaded keys authenticate from memory,
so DB failures do not stop dispatch elsewhere. Exhausted budgets/TPM can be bypassed
by new requests during quota lookup failure or cold/expired spend-cache failure.

References: `internal/httpapi/quota.go` (`quotaRow`, `scopeSpend`, `allowQuota`),
`internal/httpapi/handler.go` (pre-dispatch gate), `internal/store/crud.go`
(`GetScopeQuota`, `SumScopeSpend`), `internal/httpapi/admin_quotas_test.go`.

Requirements:

1. Distinguish genuine missing rows from operational failure at the shared quota
   read boundary; propagate errors rather than fabricating absent policy/zero spend.
2. Return controlled retryable JSON 503 (`quota_unavailable`) before dispatch when
   required data cannot be read. Log cause server-side without exposing DB details;
   do not mislabel as exhausted budget or TPM. Preserve normal missing/unlimited
   quotas and unaffected static credentials.
3. Keep fresh spend-cache semantics explicit: valid cached spend may be used within
   the existing TTL, but failed refresh must not publish zero or extend stale data.
   Inspect all helper callers so admin displays do not silently regain fake values.

Acceptance criteria:

- PG faults independently fail quota-row/model-quota/spend reads for user/team
  owners. Direct/alias × JSON/SSE requests make zero upstream calls on failures.
- Cold/expired cache cannot bypass an exhausted budget; exhausted TPM remains
  protected when its row lookup fails. Fresh-cache behavior and genuine absent
  quotas are tested, and recovery restores normal enforcement/admission.
- Error body is controlled, contains no DB/credential detail, and docs distinguish
  temporary policy unavailability from actual budget/TPM denial.

Verification: database-backed `go test -race -p 1 ./internal/httpapi ./internal/store`,
default sanity/docs checks and final shared gates.

Scoped discoveries before implementation:

- `admin_quotas.go` (`applyQuotaBudget`, `applyQuotaTPM`) also treats every
  `GetScopeQuota` error as absence. Propagate operational failures before writes
  so a failed read cannot erase a spend offset or TPM ceiling. Genuine
  `sql.ErrNoRows` remains valid initialization.
- `quotaView` already propagates list/spend errors, but its GET/PUT callers and
  `writeQuotaError` expose raw storage errors. Keep admin failure status semantics
  and replace those details with controlled messages plus server-side cause logs.
  A post-write view error must acknowledge saved state.
- With no budget row, `quotaView` skips aggregation entirely and displays zero
  even when recorded spend exists. Read spend once for every admin quota view;
  admission still needs spend only for a positive budget.
- Store reads already preserve `sql.ErrNoRows`/operational errors; no store schema
  or ledger changes are needed. `allowQuota` is the shared pre-dispatch gate for
  inference operations. The only additional `scopeSpend` caller is cache priming
  in `admin_offboarding_test.go`.

Reproduction before production edits:

- New `internal/httpapi/quota_unavailable_test.go` uses temporary PG views and
  a PL/pgSQL exception to fail actual SELECTs independently (budget row, model
  row, spend aggregation). On the old implementation, focused runs of
  `TestQuotaReadFailures/user/budget/cold` and
  `TestQuotaReadFailures/user/(model|spend)/cold` failed: each injected fault
  admitted all four direct/alias × JSON/SSE requests with HTTP 200 and four
  upstream calls. Independent row probes confirmed selective fault injection.

Completion evidence:

- Changed `internal/httpapi/quota.go`: quota reads now return presence separately
  from operational errors; spend reads propagate errors and preserve the existing
  cache TTL. Required read failure returns exactly JSON
  `{"error":{"type":"quota_unavailable","message":"quota data temporarily unavailable"}}`
  with HTTP 503 before dispatch. Cause, request ID and owner scope are logged
  server-side. Existing `403 budget_exceeded`/`429 tpm_exceeded` remain intact.
- Changed `internal/httpapi/admin_quotas.go`: inspected every quota read caller;
  budget/TPM mutations initialize only genuinely absent rows, views read actual
  spend even without budget rows, and quota storage errors use controlled 500
  messages with server-side logs. Post-save view failures explicitly report saved
  state. Existing LIFE membership gate is preserved. Updated the cache-priming
  call in `internal/httpapi/admin_offboarding_test.go` to check the returned error.
- Added `internal/httpapi/quota_unavailable_test.go` (five database-backed tests):
  - `TestQuotaReadFailures`: actual PG SELECT exceptions, independently verified
    for budget/model rows and spend; user/team × three fault stages × cold/expired
    cache × direct/alias × JSON/SSE = 48 controlled-503, zero-upstream cases.
    Each case group recovers to real budget/TPM denial, then to successful
    admission after policy headroom is restored. Exhausted TPM retains its
    rolling tokens and returns `Retry-After` after recovery. Failed refreshes
    leave cold caches empty and expired zero entries/timestamps unchanged.
  - `TestQuotaFreshSpendCache`: genuinely DB-primed fresh spend permits available
    budget and denies exhausted budget despite spend-read failure, without
    extending TTL; expiry changes this to 503 and recovery refreshes correctly.
    Fresh spend never bypasses failed budget/model policy reads.
  - `TestQuotaUnlimitedAndStaticControls`: real `sql.ErrNoRows`, explicit zero
    limits, and static credentials retain admission across direct/alias JSON/SSE;
    unlimited scopes do not require spend for admission. Admin views still show
    historical spend without a budget and fail rather than inventing zero.
  - `TestAdminQuotaReadFailures`: user/team GET/PUT and direct mutation helpers
    propagate PG causes before the affected write, preserve exact prior rows
    (including offsets/ceilings/timestamps), sanitize responses, log causes, and
    truthfully report committed edits whose view fails; recovery returns real data.
  - `TestQuotaReadFailureOtherOperations`: user/team, direct/alias, responses
    JSON/SSE, embeddings, images, speech and multipart transcription all reject
    before upstream I/O through the shared gate.
- Updated `website/docs/api-reference.md`, `website/docs/operations.md` and
  `docs/design.md` with the temporary-unavailability contract, cache behavior,
  unlimited/static controls and admin read/save outcomes.
- Owned disposable fixture (all commands from `<repo-root>`):
  inspected existing Docker containers before creating
  `aiproxy-flow01-20260926-c4a719`, container `4f6cc4c6a427`, using README's
  `docker run --detach --rm` pattern and `postgres:17` (reported PG 17.11).
  Unique database `flow01_20260926_c4a719`, user `flow01_test`, dynamically mapped
  loopback-only port `127.0.0.1:39408`; Docker health reported `healthy` and
  `pg_isready` reported accepting connections before testing. Every DB command used
  explicit `AIPROXY_TEST_DATABASE_URL='postgres://flow01_test:flow01_fixture_only@127.0.0.1:39408/flow01_20260926_c4a719?sslmode=disable'`. // pragma: allowlist secret
  No developer or prior-task fixture was used.
- Required verification passed, serialized against that fixture:
  - `go test -race -p 1 -count=1 -v ./internal/httpapi ./internal/store`:
    HTTP package 362.966s, store package 44.054s; all five new tests and existing
    quota/key-deletion/membership/invite/store regressions passed. Inspected full
    output at
    `<tool-output>/tool_0e0af5172001ISXavOHuTlT7Nt`:
    no `SKIP`, unset-URL prerequisite skips, failures or race reports. An earlier
    attempt caught a new-test multipart method typo; corrected before this pass.
  - `make vet test GOFLAGS=-p=1`: passed all packages with the explicit fixture
    URL; HTTP 80.649s, store 50.443s, app 18.898s, dbmerge 0.443s.
  - `make docs-contract`: documentation contract matrices match.
  - `git diff --check`: passed; scoped Go files formatted with `gofmt`.
- Cleanup completed: SQL checks found zero remaining `flow01_%` fault relations
  and functions. `docker stop aiproxy-flow01-20260926-c4a719` succeeded; subsequent
  exact-name `docker ps -a` returned no container (`--rm` removed the fixture).
- Preservation/limitations: reviewed final status/diff against the incoming dirty
  worktree; prior BOUNDARY/SAFE/LIFE code and task records remain, and
  `git diff -- CHANGELOG.md` is empty. Only FLOW-01 status was changed. No commits,
  subagents, frontend/asset builds or overlapping database suites. SQL faults are
  deterministic PG view/function exceptions, not a network-partition/load test.
  TPM samples and cache expiry are seeded deterministically; completion-based
  overshoot, 30-second cache lag, multi-row quota-update atomicity and durable
  ledger retry remain outside scope. Full combined `make test-race`, integration
  and frontend gates remain assigned to FLOW-05 after FLOW-02–04; no FLOW-01
  acceptance blocker remains.

### Task FLOW-02: Make Catalog Persistence And Publication Outcomes Reliable

Status: completed

Kind: defect

Priority: P1 — failed writes leave orphan/partial providers; view-read errors skip
activation after valid provider/alias commits.

Suggested agent: catalog aggregate transaction and activation implementer

Dependencies: FLOW-01

Primary ownership: provider aggregate store methods,
`internal/httpapi/admin_providers.go`, `admin_aliases.go`, store/HTTP/App regressions,
catalog API and lifecycle docs.

Finding: CreateProvider/UpdateProvider commit metadata separately from transactional
ReplaceProviderModels. Model failure leaves an orphan provider or changed metadata
with old models, despite an error. Provider and alias create/update also build views
after commit but before activateChange; a view read error returns 400 and skips
activation, allowing an unexplained later activation. Alias parent/targets already
commit together and must retain that transaction.

References: `internal/httpapi/admin_providers.go` (`createDBProvider`,
`updateDBProvider`, `providerViewByName`), `internal/httpapi/admin_aliases.go`
(`createDBAlias`, `updateDBAlias`, `aliasViewByName`), `internal/store/crud.go`
(provider CRUD/models, alias aggregate CRUD), `internal/app/app.go` (`Reload`).

Requirements:

1. Commit provider metadata plus requested model replacement as one aggregate
   transaction. Preserve omitted-model and credential-only semantics; do not
   introduce stale whole-row overwrites or mutate caller outputs on rollback.
2. Separate commit from presentation. After each valid provider/alias create/update
   commit, invoke activation once before fallible response-view reads. Preserve the
   saved-but-activation-failed contract; no DB/runtime distributed transaction claim.
3. Distinguish validation, pre-commit storage, activation and post-commit presentation
   errors accurately. Post-commit read failure must state that the edit was saved,
   not imply rollback or encourage blind duplicate creation. Protect secret values.

Acceptance criteria:

- Inject model insert failure in POST/PUT: no orphan on POST, exact old metadata/
  models after rejected PUT, no activation. Removing fault allows retry; explicit
  later reload remains usable and never activates rejected provider edits.
- Fail post-commit provider/alias view reads: activation is attempted exactly once
  and response accurately identifies saved state. Cover create and update.
- Real App tests prove successful routing changes and failed activation retaining
  old runtime plus a complete saved edit recoverable by later valid reload.
- Existing provider inheritance/credentials/models, alias validation and omitted
  field preservation tests pass; concurrency checks cover changed shared writes.

Verification: database-backed `go test -race -p 1 ./internal/store ./internal/httpapi ./internal/dbmerge ./internal/app`,
default sanity/docs checks and final shared gates.

Scoped discoveries before implementation:

- Fresh sequential FLOW-02 session confirmed FLOW-01 completion, read both requested
  task skills, AGENTS.md, README fixture rules and LIFE-04 transaction/lock contract.
  Inspected incoming dirty work and Docker containers; existing containers are unrelated.
- Provider credential PUT also uses `UpdateProvider` with a previously read whole
  row. Use optimistic revision checks shared by metadata/credential/aggregate writes;
  standalone model replacement must lock/touch the parent revision. A concurrent
  change rejects with controlled 409, requiring a fresh read/retry, rather than
  writing a stale validated aggregate. Alias updates need the same revision check
  while retaining their existing parent/target transaction.
- Create preflight reads currently treat operational failure as absence. Classify
  catalog storage failures separately from validation, sanitize public errors, and
  keep activation and saved-presentation failures distinct. Commit helpers must
  return identity only; response reads belong after activation.
- Revision checks cover the edited aggregate, not a distributed transaction across
  catalog dependencies, membership, runtime or instances. Existing LIFE locks and
  App reload rollback remain authoritative and unchanged.

Completion evidence:

- Changed `internal/store/provider_aggregate.go`, the provider/alias methods in
  `internal/store/crud.go`, `internal/httpapi/admin_providers.go`,
  `internal/httpapi/admin_aliases.go`, and new scoped
  `internal/httpapi/admin_catalog_write.go`. Provider creation now inserts metadata
  and models in one transaction; update performs its revision-checked metadata
  write and optional model replacement in one transaction. Omitted/null models
  preserve existing rows, and credential PUT participates in the same revision
  protocol. Standalone model replacement locks/touches the parent before child
  writes, invalidating stale validators. Revisions advance at PostgreSQL microsecond
  precision; successful caller outputs carry a reusable revision. Rollbacks leave
  caller objects and model/target slices untouched.
- Alias aggregate transactions are retained, with a revision predicate preventing
  stale preservation of omitted fields from overwriting a newer alias. No schema
  migration was needed. Existing LIFE key/membership/invitation methods in the
  shared CRUD file were preserved.
- All four provider/alias POST/PUT commit helpers now return identity without view
  queries. Routes request activation exactly once after valid commit and only then
  perform presentation reads. Controlled public responses distinguish validation
  (400), stale aggregate conflict (409), storage (500 `could not save catalog edit`),
  activation (500 `saved but activation failed`) and saved presentation failure
  (500 `saved but response view unavailable; read current state before retrying`).
  Storage preflight errors no longer masquerade as absence. Storage, activation
  and presentation causes are logged server-side, not returned to clients.
- Added ten top-level regression tests across three files:
  - `internal/store/provider_aggregate_test.go`: model CHECK failure after an earlier
    successful insert rolls back provider creation/update, exact metadata/models
    and timestamps, and preserves input objects/slices. Retry succeeds; the returned
    revision can be reused. Shared credential/aggregate writes reject stale state,
    standalone model replacement invalidates older revisions, credential writes
    preserve models, and alias conflicts/target failures preserve rows and outputs.
  - `internal/httpapi/admin_catalog_atomic_test.go`: actual PG model CHECK failures
    exercise POST rollback/no orphan and PUT rollback with changed metadata and
    credential, zero activation for rejected edits, and fault removal/retry. Eight
    provider/alias × POST/PUT × view/activation cases verify saved outcomes and one
    activation; table renaming inside the activation callback forces a real PG
    presentation SELECT failure only after commit. GET recovers after restoration.
    Four preflight SELECT-failure cases return controlled storage errors with zero
    activation. Deterministically paused HTTP reads overlap aggregate and credential
    PUTs in both directions: the stale request gets 409 without activation, and its
    fresh retry preserves both the new models/metadata and rotated credential.
  - `internal/app/catalog_atomic_test.go`: isolated-schema real App/router/upstream
    tests prove model-insert POST/PUT rollback remains safe across explicit reload,
    retry publishes the intended upstream model, and all four provider/alias
    create/update paths retain the old live routing when invalid HCL makes activation
    fail. The complete aggregate stays saved, restoration plus Reload publishes it,
    and subsequent successful mutations change real routing again. Responses do not
    expose the fixture credential or private activation-error marker.
- Updated `docs/design.md`, `website/docs/api-reference.md` and
  `website/docs/operations.md` with atomic catalog saves, omitted-field behavior,
  conflict/retry semantics and separate persistence/activation/presentation outcomes.
- Owned fixture: after inspecting Docker containers and README, created
  `aiproxy-flow02-20260926-e82b`, container `72d0fd8ac51a`, using
  `docker run --detach --rm`, `postgres:17` (SQL reported PostgreSQL 17.11), unique
  DB `flow02_20260926_e82b`, user `flow02_test`, dynamically mapped loopback-only
  port `127.0.0.1:39444`, and `pg_isready` healthcheck (1s interval, 5s timeout,
  60 retries). Both Docker `healthy` and explicit readiness passed before tests.
  Every Go DB command used explicit
  `AIPROXY_TEST_DATABASE_URL='postgres://flow02_test:flow02_fixture_only@127.0.0.1:39444/flow02_20260926_e82b?sslmode=disable'` // pragma: allowlist secret
  and `GOFLAGS=-p=1`; no developer
  or prior-task database was used.
- Verification passed, all suites/builds serialized:
  - Focused store/HTTP catalog and existing provider/alias checks passed, followed
    by real App checks. The first App test attempt exposed a missing migration
    setup in the new isolated-schema fixture; fixed the fixture and reran to pass.
    No pre-change runtime reproduction is claimed.
  - `go test -race -p 1 -count=1 -v ./internal/store ./internal/httpapi ./internal/dbmerge ./internal/app`:
    store 72.657s, HTTP 543.335s, dbmerge 1.884s, App 43.778s. Full log
    `<tool-output>/tool_0e0c4b552001S2aP1tVIOXOfVu`
    contains all ten new passing tests and existing inheritance/credentials,
    omitted-field, alias validation and BOUNDARY/SAFE/LIFE/FLOW-01 regressions.
    Exact-file `rg -c` inspection found no SKIP, FAIL, race warning, or unset-URL
    prerequisite skip; the database cases actually executed.
  - `GOFLAGS=-p=1 make vet test` with the explicit fixture URL passed all packages:
    App 26.961s, HTTP 103.711s, store 60.911s, dbmerge 0.489s.
  - `make docs-contract` passed; scoped Go files were formatted with `gofmt`;
    `git diff --check` passed and `git diff -- CHANGELOG.md` is empty.
- Cleanup: SQL checks found zero `flow02_%` schemas, `flow02_%` fault constraints
  or `flow02_saved` relations. `docker stop aiproxy-flow02-20260926-e82b` succeeded;
  after Docker's asynchronous `--rm` cleanup, an exact-name `docker ps -a` returned
  no container. Only the owned fixture was removed.
- Acceptance/status: FLOW-02 complete; blockers: none. Incoming dirty/untracked
  BOUNDARY/SAFE/LIFE/FLOW-01 work and other task statuses were preserved. No commits,
  CHANGELOG edits, nested agents, frontend builds or overlapping suites. Revision
  checks protect the edited aggregate; raw SQL writers must maintain the revision
  protocol. Cross-resource validation, database/runtime publication and multiple
  instances are not a distributed transaction; views can reflect later commits.
  Fault tests are deterministic PG constraints/renames and local upstream tests,
  not network-partition or production-load coverage. Shared full race/integration
  and frontend final gates remain assigned to FLOW-05.

### Task FLOW-03: Make Browser Provider Edits Express Explicit State Changes

Status: completed

Kind: defect

Priority: P2 — ordinary edits report success while retaining disabled/forwarding
or override settings the operator tried to change.

Suggested agent: provider form serialization and UX implementer

Dependencies: FLOW-02

Primary ownership: `web-ui/src/pages/providers-page.tsx`, narrow reusable form
serialization helper if justified, mounted regression tests and relevant UI docs.

Finding: buildProviderBody applies create-style omission rules to edits: enabled
true, forward flags false, empty headers and cleared display/baseURL/timeout/user
agent are omitted. The API intentionally preserves omitted fields; unchecking health
send_authorization also leaves true. ProviderToFormValues correctly loads values,
so create/edit serialization is the missing boundary.

References: `web-ui/src/pages/providers-page.tsx` (`providerToFormValues`,
`buildProviderBody`, `EditProviderDialog`), `web-ui/src/services/admin.ts`
(`updateAdminProvider`), `internal/httpapi/admin_providers.go`
(`applyProviderUpsert`, `buildHealthcheckJSON`).

Requirements:

1. Distinguish create defaults from edit patches. Explicit boolean changes and
   clearing strings/lists must be represented where supported by the existing API.
2. Preserve untouched write-only credentials/references without clearing or
   resending them. Preserve inheritance constraints and root/provider defaults:
   a local false value cannot override an independently enabled root default.
3. Cover supported scalar healthcheck edits, especially send_authorization false;
   do not invent optional-block deletion semantics. If block removal is unsupported,
   make the UI truthful rather than claiming an omitted block was deleted.
4. Resolve the default UI gate's dashboard-detail async reliability within
   `web-ui/src/dashboard-operator.test.tsx`. Earlier isolated/full runs measured
   affected one-second element waits failing after roughly 1.3–1.7s; all unchanged
   assertions passed with a diagnostic five-second budget. The inspected flow awaits
   public status, then stored operator authority, then list-query/render completion
   before a detail click; query retries are already disabled. Measure query/render
   and selector timing to distinguish scheduling/selector cost from a provider
   regression before changing the waits. Prefer condition-correct, cheap readiness
   waits, retaining the role/name, denial and auth-generation assertions. If a
   larger wait is necessary, scope it to the measured slow readiness condition;
   do not change global timeouts, skip cases or mock away authorization boundaries.
   Measured follow-up (before the permanent test fix): temporary query-cache and
   selector instrumentation on the three affected cases found role scans taking
   98–783ms each and final buttons appearing at 1.32s, 2.02s and 2.62s. Shell's
   first organization selection advances the auth generation, cancels the initial
   list fetch, and remounts/refetches through App's generation-keyed dialog provider.
   The immediate service mocks resolve between those synchronous scans; retries
   are disabled. This is test-side polling contention across legitimate async
   initialization, not a provider-form regression. Wait cheaply for the selected
   organization and visible capture text before retaining the exact button role/name
   lookup and click. Apply the organization readiness wait to the request-demotion
   case too, so its error assertion observes the settled generation. Remove timing
   instrumentation after diagnosis; keep the default global wait/test configuration.

Acceptance criteria:

- Mounted form tests cover both enabled transitions, forward_user_agent and health
  send_authorization true→false, header clearing, timeout/user-agent reset and other
  supported cleared values. Assert outgoing bodies and save/reopen state.
- Untouched credentials are absent from update bodies; create defaults and backend
  omitted-field preservation still work. Inherited provider edits remain valid.
- User-facing text explains any relevant root/default/block-preservation behavior;
  all frontend tests and real application/tooling typecheck pass.

Verification: `pnpm --filter @aiproxy/web-ui test`,
`pnpm --filter @aiproxy/web-ui typecheck`, applicable default sanity/docs checks,
final embedded-UI integration.

Scope clarification before implementation:

- Fresh sequential FLOW-03 session confirmed FLOW-01/02 completion, read AGENTS.md,
  both task skills and the current API/store merge contract, and inspected incoming
  dirty work. Changes are limited to the provider form, mounted regressions and
  relevant documentation; no backend contract changes are needed.
- Edits will send changed fields only, including supported empty/false values;
  untouched models and credential references will be omitted. Healthcheck scalar
  reset values are empty strings or numeric zero, which the API normalizes to its
  defaults. A nonempty path is required; empty path, omitted block and null do not
  delete an existing healthcheck. The UI must retain that block visibly.
- The current form requires a local model even for inherited providers. Conditional
  model validation and inherited-field presentation are necessary for edit fidelity.
  Database inheritance takes models, transport and healthcheck from the static base;
  `dbmerge.BuildProvider` explicitly retains the database row's enabled flag. Keep
  that local flag editable, suppress ineffective inherited-setting submissions, and
  preserve local display-name/credential edits and the same-type base requirement.
- Clearing local user-agent/timeout restores root/type defaults; clearing forwarding
  settings cannot opt out of independently configured root forwarding. Mounted tests
  will exercise the real page/dialog/form/services with controlled HTTP responses,
  asserting exact requests and reopened values, not backend persistence simulation.
  Frontend-only verification uses no DB; FLOW-05 owns fresh PG/full integration.
- Mounted-test discovery before the corrective edit: shadcn-theme 0.1.15's
  `ActionMenu.DefaultTrigger` accepts only `className`, dropping Radix's injected
  trigger events/ref. The real provider menu cannot open in the mounted workflow.
  Supply an explicit existing `Button` as this page's trigger; this is necessary
  to reach the form without mocking the action menu. No dependency/shared-menu
  rewrite is included.
- Mounted Copilot edits exposed stale type metadata: memoized action columns capture
  the initial undefined provider-type list. Refresh those columns when type data
  arrives and prevent create/edit while types are unavailable, so credential-reference
  forms use the actual provider contract rather than falling back to API-key mode.

Initial implementation and verification evidence (earlier blocked attempt; superseded by Completion evidence below):

- Changed `web-ui/src/pages/providers-page.tsx`: edits compare against their initial
  form values and send only changed fields, with explicit enabled true/false,
  forwarding false, empty strings/lists and healthcheck scalar reset values.
  Untouched models, inline secrets and API-key/Copilot references are omitted;
  changed reference objects retain both path and identity. Create omission/default
  behavior is retained. Existing healthchecks stay visibly selected, blank paths
  fail validation, and help text explains block retention and local/root defaults.
- Inherited providers can create/save without local models. The form hides inherited
  transport/model/health settings, preserves local display/credential edits and the
  DB-local enabled flag, and requires valid local models when Extends is cleared.
  Added the explicit existing Button action trigger and refreshed memoized type
  metadata as documented above. No service/auth-generation/backend changes.
- Added `web-ui/src/pages/providers-page.test.tsx`: 13 mounted tests use the real
  page, action menu, dialogs, hook-form components, queries and admin HTTP services.
  Axios adapters capture exact JSON bodies and return separately authored saved/list
  views; they do not implement a fake persistence/patch merger. Covered both enabled
  transitions, forwarding/auth on and off, all local string/header clears, every
  health scalar reset and normalized reopen value, invalid blank health path and
  unchanged block preservation, untouched encrypted/API-key-ref/Copilot credentials,
  changed reference path reset, create defaults, inherited create/edit/exit, model
  string/pricing/capability clearing and subsequent unchanged saves.
- Updated `website/docs/api-reference.md` and `website/docs/operations.md` with
  supported partial-update/reset values, credential preservation, inherited/local
  settings and the unsupported whole-healthcheck deletion contract.
- Required checks, serialized from the repo root:
  - `pnpm --filter @aiproxy/web-ui exec vitest run src/pages/providers-page.test.tsx`:
    13/13 passed under the repository setup (42.41s). Early new-test runs caught the
    trigger/type-metadata issues above, a create-button readiness race in the test,
    and a too-small per-test budget for the multi-reopen scenario; corrected locally.
  - `pnpm --filter @aiproxy/web-ui typecheck`: passed after fixing new-test-only
    Testing Library option/type errors. Production application and tooling are
    included by the existing real typecheck.
  - `pnpm --filter @aiproxy/web-ui test`: 148/149 passed; an existing dashboard
    payload-detail test exceeded Testing Library's default one-second element wait.
    Other runs hit the corresponding block-detail or pending-authority case.
    The provider suite and auth lifecycle/service generation suites passed.
    A single-worker attempt also hit an existing wait and was terminated by the
    shell's 120s overall limit; a two-thread run completed with 145/149 passing and
    four existing dashboard waits failing. The dashboard test file alone reproduced
    three waits failing, without running any new provider tests.
  - Timing diagnosis only: temporary `<repo-root>/tmp/flow03-vitest.config.mts`
    imported the repository config and setup, appending a setup file that called
    Testing Library `configure({ asyncUtilTimeout: 5000 })`. Running
    `pnpm --filter @aiproxy/web-ui test --config <repo-root>/tmp/flow03-vitest.config.mts`
    passed **149/149 tests, all 10 files** (37.07s), with no skipped tests or changed
    assertions. Host load observed during diagnosis: 51.22/53.19/53.18. This proves
    the unchanged tests pass with a longer wait budget, not that the default gate
    passed. Both temporary files were removed; repository test setup is unchanged.
  - `make web-build`: passed typecheck and Vite production build (8,757 modules;
    Vite 4.22s), completed before Go checks. Removed only its newly created untracked
    `.gitkeep`; ignored built assets remain available for embedded-UI review.
  - `env -u AIPROXY_TEST_DATABASE_URL GOFLAGS='-p=1 -v' make vet test`: passed all
    packages, including embedded web UI. Verbose log
    `<tool-output>/tool_0e0e6effb001KB8jCIbc4Jek1I`
    contains 167 explicit unset-DB prerequisite skips in App/dbmerge/HTTP/store and
    one expected pre-build-stub skip because the UI is built. No PG fixture was
    created or used; DB-backed omissions/inheritance/persistence were not rerun here.
  - `make docs-contract` and `git diff --check`: passed.
- Preservation: only this task's status changed. Incoming dirty/untracked work,
  other task records, services/auth-generation guards and CHANGELOG were preserved;
  `git diff -- CHANGELOG.md` is empty. No commits, subagents or overlapping builds.
- **Earlier blocker (resolved below):** the requested default frontend gate was not green on this
  host because of the existing dashboard-detail wait failures. FLOW-03 implementation
  and behavioral coverage are delivered, but completion is withheld. Coordinator /
  FLOW-05 must obtain a passing default run or explicitly resolve the shared test
  wait-budget policy before marking this task completed. FLOW-05 still owns a fresh
  disposable PG fixture, full race/integration gates and final embedded-UI review.
  No backend deletion semantics, database runtime persistence coverage or production
  browser performance measurement is claimed by these frontend fixture tests.

Completion evidence:

- FLOW-03 resumed in the same scope at the user's request. Added requirement 4 and
  recorded measured async-flow findings before the permanent verification fix.
  Changed only `web-ui/src/dashboard-operator.test.tsx` and this task record during
  remediation; the provider implementation, its 13 mounted regressions and public
  docs remain as detailed in the initial implementation evidence above.
- Classified the previous failures as test-side async polling contention, not a
  provider regression: Shell automatically selects `org-a`, advancing auth generation;
  App remounts and sensitive queries cancel/refetch. Immediate mocks, with retries
  already disabled, resolve between expensive whole-shell role scans. Temporary
  instrumentation measured final capture readiness at 1.32–2.62s, with individual
  slow role scans costing 98–783ms. The provider form is not mounted on these routes.
- The scoped test helper now waits cheaply for the selected organization and visible
  capture text, then performs the original exact button role/name lookup before the
  click. The request-demotion test also waits for organization selection before
  checking its error view. This retains real authority checks, automatic organization
  selection, auth-generation remount/refetch and the A-to-B transition; no cache
  pre-seeding, authorization mocks, assertion removal or skipped cases were added.
  No global or targeted timeout increase was needed in the final fix.
- Measured the condition-correct waits: organization selection 374–527ms, text
  readiness 719–850ms from start, total including the final role lookup 927–1034ms.
  Each awaited stage fits its existing default wait budget. The four affected
  scenarios passed in the focused run, and all temporary timing instrumentation
  was removed before the final default command.
- Final verification from `<repo-root>`:
  - **`pnpm --filter @aiproxy/web-ui test`: 149/149 passed, 10/10 files, no skips,
    41.68s.** This is the official default command with the repository configuration,
    superseding the earlier failed/default and diagnostic-timeout runs. Includes
    all 13 provider-form tests and existing dashboard/auth-generation regressions.
  - `pnpm --filter @aiproxy/web-ui typecheck`: passed application, tests and tooling.
  - `make docs-contract`, `git diff --check`, and explicit untracked-file whitespace
    checks: passed.
  - The earlier successful `make web-build` and serialized `make vet test` remain
    applicable: remediation changed only tests/documentation, so Go/assets were not
    rebuilt or retested. The previously disclosed 167 optional DB prerequisite
    skips remain; no DB-backed verification is newly claimed.
- Acceptance/status: **FLOW-03 completed; blockers: none.** Explicit edit values,
  create defaults, credentials, inheritance, healthcheck retention, mounted request/
  reopen regressions, truthful docs and required local gates are covered. FLOW-04
  and FLOW-05 remain pending; FLOW-05 owns fresh PostgreSQL/full race/integration
  verification. Prior work and CHANGELOG preserved; no commits or subagents.

### Task FLOW-04: Preserve Full Model Identifiers In Browser Alias Editing

Status: completed

Kind: defect

Priority: P2 — slash-containing model names are truncated, breaking or silently
retargeting valid aliases during save.

Suggested agent: alias target parser and edit-roundtrip implementer

Dependencies: FLOW-03

Primary ownership: `web-ui/src/pages/aliases-page.tsx`, focused parser/form tests,
appropriate alias-edit help/docs.

Finding: parseTargetLines destructures `split('/')` into provider/model and discards
later segments. aliasToFormValues renders complete names, then every save reparses
them. `gateway/z-ai/glm-5.2` becomes model `z-ai`; if that shorter model also exists,
an unchanged save can validly but wrongly retarget the alias.

References: `web-ui/src/pages/aliases-page.tsx` (`parseTargetLines`,
`aliasToFormValues`, `buildAliasBody`), AGENTS.md model naming contract,
`internal/config/dynamic.go` model validation, backend `buildConfigAlias`.

Requirements:

1. Split provider from model at the first slash only and preserve the full model
   suffix in both create/edit. Validate malformed identifiers/separators explicitly
   using the current provider/model naming contract, not lossy normalization.
2. Keep shorthand provider/model input consistent and display actionable field/form
   errors without submission for invalid targets. Preserve ordinary input behavior.

Acceptance criteria:

- Ordinary, slash-containing and deeper model names submit exactly, with surrounding
  whitespace handled deliberately. Missing provider/model, repeated/trailing slash,
  invalid segments and malformed separators are rejected visibly.
- Mounted create and unchanged edit round trips preserve full targets, including a
  collision fixture with both full and shortened names. No silent retargeting.
- Shorthand model inputs preserve the same supported model syntax; existing alias
  algorithm/retry/options workflows continue to pass.

Verification: `pnpm --filter @aiproxy/web-ui test`,
`pnpm --filter @aiproxy/web-ui typecheck`, applicable default sanity/docs checks,
final embedded-UI integration.

Scoped discoveries before implementation (FLOW-04):

- Fresh isolated sequential session confirmed FLOW-01–03 completed and read the
  shared rules, AGENTS.md and both task skills. Incoming dirty/untracked work was
  inspected. This frontend-only task needs no database fixture.
- `internal/config/helpers.go` defines each name segment as
  `[a-z0-9][a-z0-9._-]*`; model names allow multiple nonempty slash-separated
  segments, and provider name `alias` is reserved. Share those predicates between
  explicit targets and shorthand. Retain trimming around provider/model boundaries,
  comma-separated providers and blank target lines; reject interior whitespace,
  empty slash/comma segments and invalid punctuation rather than repairing names.
- `buildAliasBody` silently prioritizes shorthand over populated targets although
  the UI and `expandAliasTargets` forbid combining them. Add field validation for
  mutually exclusive input, both required shorthand halves, empty/duplicate shorthand
  providers and at least one target. This is necessary to avoid silent retargeting.
- The alias page uses the same broken default ActionMenu trigger documented in
  FLOW-03. Use its established explicit Button trigger to exercise real mounted
  unchanged edits. Keep algorithm/retry/affinity/reasoning serialization intact;
  optional-block reset semantics are outside this task.
- Mounted tests will capture actual admin-service HTTP bodies and use separately
  authored saved/list responses, including both full and shortened target names.
  They verify browser request/reopen behavior, not database persistence or routing.

Reproduction before production edits (FLOW-04):

- Added the mounted regression file, then ran
  `pnpm --filter @aiproxy/web-ui exec vitest run src/pages/aliases-page.test.tsx -t 'creates full and shortened collision targets'`.
  The unmodified page submitted POST model `z-ai` instead of `z-ai/glm-5.2` and
  `org_1` instead of `org_1/family.v2/3-model`. The exact HTTP-body assertion failed;
  42 other cases were excluded by this focused test-name filter. The fixture also
  includes the legitimate `gateway/z-ai` target, exposing the silent collision.

Completion evidence (FLOW-04):

- Changed `web-ui/src/pages/aliases-page.tsx`: target parsing splits at the first
  slash and preserves the entire model suffix. Shared provider/model predicates
  match the backend segment grammar, including lowercase/digit starts, dots,
  underscores, hyphens, arbitrarily deep nonempty model segments and reserved
  provider name `alias`. Normal boundary whitespace, CRLF and blank lines remain
  supported; internal whitespace and malformed names are rejected rather than
  shortened or normalized into another identity.
- Existing Zod/hook-form validation now displays actionable errors at the target,
  shorthand provider or shorthand model field before the mutation runs. Target
  errors include the original line number. Missing targets/shorthand halves,
  mixed explicit/shorthand inputs, empty comma entries and duplicate shorthand
  providers are rejected. Added full-name/whitespace help and the established
  explicit Button action-menu trigger so real unchanged edits can open.
- Added `web-ui/src/pages/aliases-page.test.tsx`, 43 mounted cases through the real
  page, action menu, dialogs, form, query client and admin HTTP services:
  - Explicit create → GET/list → reopen → unchanged PUT → reopen asserts exact
    requests with ordinary, full, shortened-collision and deeper model names;
    input includes spaces around the first slash, tabs, blank lines and CRLF.
  - Existing aliases round-trip unchanged under both algorithms with exact retry,
    affinity and encrypted-reasoning values (including passthrough false). Reopen
    assertions cover every option field as well as the full targets.
  - Three shorthand model depths create with trimmed providers/model, reopen as
    expanded targets and save unchanged without losing any model segment.
  - Eighteen explicit and seventeen shorthand malformed-input cases retain the
    input, show visible field errors and make zero HTTP writes: missing names,
    repeated/trailing slashes, uppercase/invalid segments, internal whitespace,
    reserved providers, backslash/colon/semicolon/comma misuse, empty comma entries
    and duplicate providers. A mounted edit rejects mixed input and malformed
    targets, then successfully saves corrected full names. Retry integer errors
    and server save errors retain the form and exact target input.
  - HTTP adapters return separately authored saved/list views; assertions inspect
    actual serialized request bodies rather than simulating backend patch merging.
    An early focused run passed all eight workflow cases but failed 35 new
    assertions expecting `aria-invalid`: the existing theme components render
    field errors without that attribute. Corrected those test-only assumptions
    to assert visible errors, retained values and zero writes using existing form
    conventions; no shared component or timeout changes were needed.
- Added `Editing Database Aliases In The Web UI` in `website/docs/operations.md`
  documenting full-name preservation, segment grammar, boundary whitespace,
  actionable validation and shorthand expansion on reopen.
- Verification from `<repo-root>`, with builds/suites serialized:
  - **`pnpm --filter @aiproxy/web-ui test`: 192/192 tests passed, 11/11 files,
    no skips, 31.30s.** Includes all 43 new alias tests, 13 prior provider tests
    and dashboard/auth-generation regressions under the default configuration.
  - `pnpm --filter @aiproxy/web-ui typecheck`: passed application, tests and tooling.
  - `pnpm --filter @aiproxy/web-ui build`: passed typecheck and Vite production
    build (8,757 modules, 2.41s), refreshing ignored embedded assets before Go ran.
  - `env -u AIPROXY_TEST_DATABASE_URL GOFLAGS='-p=1 -v' make vet test`: passed all
    packages; CLI 19.190s, App 16.829s, HTTP 3.653s, embedded webui 0.011s; other
    unchanged packages used Go's normal test cache. Full output:
    `<tool-output>/tool_0e0f42ab9001I0STGsVQVtq4dD`.
    Inspected output reports 167 unset-DB prerequisite skips across App/dbmerge/
    HTTP/store, plus one expected pre-build-stub skip because the UI is built.
    No database fixture was created or used; DB acceptance is not claimed here.
  - `make docs-contract`: documentation contract matrices match.
  - `git diff --check` and explicit `git diff --no-index --check /dev/null` checks
    for the new untracked test and task record passed. Reviewed scoped diffs and
    final status against incoming work; `git diff -- CHANGELOG.md` is empty.
- Acceptance/status: **FLOW-04 completed; blockers: none.** Only FLOW-04 status
  changed; prior provider/auth/form/backend work and other task records remain.
  No CHANGELOG edits, commits or subagents. Algorithm/retry/options serialization
  remains intact; optional-block deletion/reset redesign is not included. Mounted
  fixtures prove browser requests/reopen behavior, not real DB persistence or
  upstream routing. FLOW-05 retains its independent fresh-PG, full race/integration
  and combined acceptance review gates.

### Task FLOW-05: Independently Audit Admission And Editing Integration

Status: completed

Kind: improvement

Priority: P1 — verify quota failure, persistence and browser intent contracts together.

Suggested agent: independent reviewer, not a prior FLOW implementer

Dependencies: FLOW-01, FLOW-02, FLOW-03, FLOW-04

Primary ownership: review all plan changes and this execution record; narrowly
scoped corrections and regressions necessary for established acceptance.

Finding: isolated fixes must preserve truthful admission errors, committed-state
reporting, actual routing, and browser/API edit semantics in the combined product.

References: preceding tasks and source/tests/docs referenced in their evidence.

Requirements:

1. Audit every task criterion against actual source/tests/callers/docs, including
   quota read errors/caches and alternate operations, provider aggregate and
   credential updates, alias publication, form clear/false semantics/inheritance,
   write-only credentials and full alias names. Check error payloads for secrets.
2. Record scoped corrections before edits and add regressions. Run all shared final
   gates with a new disposable PG fixture; prove DB suites actually execute without
   prerequisite skips and remove the owned fixture after checks.
3. Preserve prior BOUNDARY/SAFE/LIFE work and CHANGELOG. Append per-task acceptance/
   status audit, actual checks and honest limitations; no unresolved requirement
   may be marked completed.

Acceptance criteria:

- All five tasks completed with Completion evidence and no unresolved acceptance
  blockers; final checks pass, UI/API/docs agree, fixtures removed.
- Worktree/CHANGELOG preservation checked and analysis/runtime limitations recorded.

Verification: all shared final gates and independent source/evidence audit.

Review start:

- Fresh independent FLOW-05 session; implemented none of FLOW-01–04. Read the
  entire task/evidence record, AGENTS.md and the explicitly requested project-level
  task-as-you-go skill. Dependencies are completed. Inspected the extensive incoming
  dirty/untracked work and Docker inventory; existing containers are unrelated.
- Audit actual source, callers, regressions and public contracts before accepting
  prior evidence. Record necessary acceptance corrections before implementation.
  All final suites will run serialized, with built UI assets and an owned fresh PG17
  fixture. No nested agents, commits or CHANGELOG edits.

Scoped acceptance correction recorded before implementation:

- **FLOW-05-C1 (P1, FLOW-03 requirement 2 / UI/API/docs agreement):** source audit
  found `dbmerge.MergeCatalog` receives only `rt.Catalog`; concrete database
  providers built by `BuildProvider` never receive root `user_agent`,
  `upstream_header_timeout`, `forward_user_agent` or `forward_headers`. App Build,
  Reload and CLI database validation all use this path. Static `buildProvider`
  applies these defaults, but clearing a DB-local override instead falls back to
  adapter defaults, and local false/empty values effectively bypass root forwarding.
  Existing mounted form fixtures assert requests/reopened local values, not runtime
  root behavior, so their green results do not prove this stated acceptance criterion.
- Required correction: provide the runtime root settings to the shared database
  merge, apply the same timeout/user-agent fallback, forwarding OR and root-first
  case-insensitive header union to concrete DB providers before validation and
  inheritance. Keep persisted/admin-view values local, preserve explicit overrides,
  DB-local enabled state and untouched credentials, and update all merge callers.
  Add a real App/PG/upstream regression for create/edit/reset, changed roots on
  reload, startup from saved rows, inheritance and request headers; reproduce the
  missing behavior before the fix. Document resolution in the public/design contract.
- **FLOW-05-C2 (P2, FLOW-03 explicit header clearing):** the new real App regression
  first reproduced C1 (effective agent empty, timeout zero, forwarding false and
  headers empty). After applying root defaults it reached the reset and reproduced
  a second gap: PUT `forward_headers: []` returned controlled storage 500.
  `applyProviderUpsert` clones the empty slice with a nil destination, producing nil;
  Bun's `nullzero` field then writes SQL NULL into the NOT NULL JSONB column.
  Preserve the explicit nonnil empty list at this API boundary so reset stores `[]`,
  activates normally and keeps root headers effective. The same App regression must
  verify the stored empty/local values, successful response and actual wire headers.

Completion evidence — independent final review:

- **Result: 5/5 FLOW tasks accepted/completed, two scoped acceptance corrections
  resolved, zero unresolved acceptance blockers.** Prior completion records above
  remain historical evidence; this audit supersedes the incomplete root-default/
  actual header-clear integration assumptions in FLOW-03.
- C1 changed `internal/config/dynamic.go` (`Runtime.ApplyProviderDefaults`) and
  `internal/dbmerge/dbmerge.go` (`MergeCatalog`): runtime defaults reach concrete
  database providers before validation/inheritance. Explicit timeout/user-agent
  overrides win, forwarding is root/local OR, and headers reuse the existing
  root-first case-insensitive union. Updated both App callers, CLI database
  validation in `cmd/aiproxy/main.go`, and the two existing merge test call sites.
  The key-expiration test's assertions and incoming implementation are preserved.
- C2 changed one assignment in `internal/httpapi/admin_providers.go`: a supplied
  empty forwarding-header array remains nonnil, serializing as JSON `[]` instead
  of SQL NULL. Omitted arrays still preserve the old value.
- Added `internal/app/catalog_defaults_test.go`, one real App/isolated-PG-schema/
  HTTP-upstream regression (`TestCatalogProviderRootDefaultsRoundTrip`). It checks
  defaulted create, explicit overrides, case-insensitive root/local header union,
  explicit string/false/list resets, stored and GET-view local values, unchanged
  encrypted credentials and actual upstream Authorization/User-Agent/header values.
  Changing roots on reload replaces old defaults; removing root forwarding stops
  forwarding. Restart from the saved rows preserves behavior. A disabled derived
  provider keeps its base transport/models and its own enabled state/credential.
- Reproduction used the owned URL and `GOFLAGS=-p=1` with
  `go test -count=1 -run '^TestCatalogProviderRootDefaultsRoundTrip$' -v ./internal/app`:
  before C1 it failed with empty agent, 0s timeout, false forwarding and no headers;
  after C1/before C2 it failed at the reset with storage 500. After both corrections
  it passed (App 0.882s). The final expanded regression, including removal of root
  forwarding after restart, passed in both final full Go runs below (race 0.65s).
- Added the default-application contract to `docs/design.md` and clarified startup/
  reload/local-value behavior in `website/docs/operations.md`. Existing API-reference
  reset and saved-state contracts now agree with the corrected runtime behavior.

Independent per-task acceptance/status audit:

| Task    | Final status | Independent source, caller and behavior acceptance                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| ------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| FLOW-01 | completed    | PASS. Inspected `quota.go`, all `quotaRow`/`scopeSpend`/store-read callers, `admin_quotas.go`, store SQL and `handler.go` dispatch ordering. Only `sql.ErrNoRows` becomes absent policy; operational row/model/spend errors become controlled JSON 503 with cause logged, no upstream call. Positive-budget spend reads honor fresh TTL and failed refresh never changes cold/expired entries. Admin views aggregate real spend without budget rows, failed prerequisite reads stop the affected write, and post-save view failures say saved. The five PG-fault regressions executed: 48 direct/alias × JSON/SSE cold/expired failures, recovery to real budget/TPM enforcement and admission, fresh-cache behavior, missing/zero/static controls, admin read/write/display failures, and responses JSON/SSE, embeddings, images, speech and multipart transcription through the same gate. Exact response assertions exclude DB/credential details; 403/429 denial and Retry-After survive recovery. API/operations/design docs agree.    |
| FLOW-02 | completed    | PASS. Inspected provider aggregate helpers, all production CRUD callers, alias transaction/revision code, HTTP commit helpers and all four route orderings, credential PUT and App Reload. Provider metadata and requested model replacement commit together; omitted/null models stay intact. Parent revision predicates and standalone child-write revision changes prevent stale aggregate/credential/alias preservation writes. Caller objects/slices are published only after commit. Store, HTTP and App regressions executed real CHECK/SELECT failures, exact rollback/no orphan/no activation, usable retry/later reload, two HTTP credential-versus-aggregate orderings and alias stale conflicts. All eight postcommit provider/alias × POST/PUT × activation/view cases attempt activation once and distinguish saved state. Real App routing retains old runtime after rejected activation and later publishes the complete saved edit. Controlled 400/409/500 contracts and server-side cause logging agree with public docs. |
| FLOW-03 | completed    | PASS after C1/C2. Inspected form initial values, changed-field serialization, real admin services/schema parsing, backend patch/default logic and database/runtime merge. All 13 mounted cases pass: enabled both directions, false forwarding/probe auth, local string/header clears, health scalar resets, exact request/reopen values, untouched encrypted/API-key-ref/Copilot credentials, changed reference paths, create defaults, inherited create/edit/exit and model replacements. Inherited local enabled/display/credential behavior agrees with `BuildProvider`; hidden inherited fields are not submitted. Healthcheck retention/required path is truthful. New App regression closes the actual PG/root-default/header-clear gap rather than weakening UI expectations. The default dashboard suite passes with condition-correct readiness and intact authority/denial/auth-generation assertions, detailed below.                                                                                                           |
| FLOW-04 | completed    | PASS. Inspected first-slash parsing, rendering, Zod field validation, admin serialization, backend name grammar/alias builders and all 43 mounted cases. Full/deeper names retain every suffix, including a full-versus-shortened collision. Whitespace handling is explicit; empty/repeated/trailing segments, invalid punctuation/case, malformed separators, missing halves, duplicate shorthand providers and mixed explicit/shorthand input fail visibly with zero writes. Create, unchanged edit and shorthand expansion round trips preserve exact targets; both algorithms and retry/affinity/reasoning options survive. Existing backend validation/shorthand/omitted-target tests also pass. Operations docs match.                                                                                                                                                                                                                                                                                                               |
| FLOW-05 | completed    | PASS. Independent reviewer implemented none of FLOW-01–04; audited actual source/tests/services/docs rather than accepting prior green suites alone. Recorded C1/C2 before their production edits, reproduced and corrected both with a meaningful new integration regression, completed all shared gates, inspected DB execution, preserved incoming work and removed only the owned fixture.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

Dashboard timing-fix audit:

- `web-ui/src/dashboard-operator.test.tsx` waits for `adminOrgStore.orgId ===
'org-a'`, then visible capture text, then retains the exact button role/name
  selection and click. The demotion case also waits for the settled organization.
  Inspected Shell's automatic organization-selection effect, store/auth generation,
  sensitive-query cancellation/removal/refetch, App's generation-keyed dialog
  provider and `useDataAuth`'s stored-authority checks. These are legitimate async
  prerequisites, not conditions that bypass authorization.
- Member/org-admin direct-link denial and zero dashboard reads, allowed operator
  routes, pending/failed account verification, server-side demotion, detail denial
  and hidden decision controls, removal of A's payload/navigation while B is pending,
  eventual denial of B and unchanged list-fetch count are all still asserted.
  No timeout changes, skips, cache seeding or product-flow changes were introduced
  by this reviewer; the default 192-test UI run passed without diagnostic config.

Owned fixture and exact actual final verification:

- Inspected `docker ps -a --format '{{.ID}} {{.Names}} {{.Image}} {{.Ports}}'`
  before creating a fresh fixture using README's pattern:

  ```sh
  docker run --detach --rm --name aiproxy-flow05-20260926-9e36 --env POSTGRES_USER=flow05_test --env POSTGRES_PASSWORD=flow05_fixture_only --env POSTGRES_DB=flow05_20260926_9e36 -p 127.0.0.1::5432 --health-cmd 'pg_isready -U flow05_test -d flow05_20260926_9e36' --health-interval 1s --health-timeout 5s --health-retries 60 postgres:17
  docker inspect --format '{{.State.Health.Status}} {{json .NetworkSettings.Ports}}' aiproxy-flow05-20260926-9e36
  docker exec aiproxy-flow05-20260926-9e36 pg_isready -U flow05_test -d flow05_20260926_9e36
  docker exec aiproxy-flow05-20260926-9e36 psql -U flow05_test -d flow05_20260926_9e36 -Atc 'select version()'
  ```

- Container `9c3517ac056b`; unique database `flow05_20260926_9e36`; mapped only to
  `127.0.0.1:38608`. Docker reported healthy, readiness reported accepting
  connections, SQL reported PostgreSQL 17.11. No developer/prior-task DB was used.
- All commands ran from `<repo-root>`. Database suites and builds
  were serialized. Exact final Go gate commands:

  ```sh
  AIPROXY_TEST_DATABASE_URL='postgres://flow05_test:flow05_fixture_only@127.0.0.1:38608/flow05_20260926_9e36?sslmode=disable' GOFLAGS=-p=1 make vet test # pragma: allowlist secret
  AIPROXY_TEST_DATABASE_URL='postgres://flow05_test:flow05_fixture_only@127.0.0.1:38608/flow05_20260926_9e36?sslmode=disable' GOFLAGS='-p=1 -v -count=1' make test-race # pragma: allowlist secret
  AIPROXY_TEST_DATABASE_URL='postgres://flow05_test:flow05_fixture_only@127.0.0.1:38608/flow05_20260926_9e36?sslmode=disable' GOFLAGS='-p=1 -v' make integration # pragma: allowlist secret
  ```

| Final gate / command                                                                                                                                                                                                                                                  | Actual result                                                                                                                                                                                                                                                                                                                                                |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `make vet test` with prefix above                                                                                                                                                                                                                                     | PASS after corrections. CLI 20.402s, App 20.318s, dbmerge 0.264s, HTTP 54.899s; unchanged store used its successful owned-PG cache.                                                                                                                                                                                                                          |
| `make test-race` with prefix above                                                                                                                                                                                                                                    | PASS, forced uncached across all packages: 1,286 top-level PASS lines, 26 tested packages. App 29.865s, dbmerge 1.729s, HTTP 312.001s, store 30.362s. All four DB-owning packages really executed; no unset-URL/prerequisite skip, FAIL or race warning. Only skip: `internal/webui.TestNotBuiltWithoutIndex`, expected because production assets are built. |
| `make integration` with prefix above                                                                                                                                                                                                                                  | PASS, 6/6 top-level binary integration tests (including 5 routing subcases), no skips, 1.878s. Target rebuilt UI/typechecked first (8,757 modules, Vite 1.79s), then built the CGO-disabled binary, then executed integration.                                                                                                                               |
| `pnpm --filter @aiproxy/web-ui test`                                                                                                                                                                                                                                  | PASS, 192/192 tests, 11/11 files, no skips, 27.43s; default repository configuration. Subsequent corrections changed backend/tests/docs only, so this UI result remains applicable.                                                                                                                                                                          |
| `pnpm --filter @aiproxy/web-ui typecheck && pnpm --filter @aiproxy/web-ui build`                                                                                                                                                                                      | PASS: application/tests/tooling typecheck and production Vite build (8,757 modules, 2.42s) completed before Go asset readers; integration also repeated typecheck/build in sequence.                                                                                                                                                                         |
| `make docs-contract && git diff --check`                                                                                                                                                                                                                              | PASS: documentation contract matrices match; no whitespace errors.                                                                                                                                                                                                                                                                                           |
| `git diff --no-index --check /dev/null internal/app/catalog_defaults_test.go && git diff --no-index --check /dev/null internal/dbmerge/key_expiration_test.go && git diff --no-index --check /dev/null docs/tasks/20260926-191434-admission-and-catalog-workflows.md` | PASS: explicit checks include untracked files absent from ordinary `git diff --check`.                                                                                                                                                                                                                                                                       |

- Final uncached race log:
  `<tool-output>/tool_0e10eb8ef001l3kD128e26GQUi`.
  Inspected package results, top-level PASS/SKIP/FAIL lines, quota/catalog/merge test
  execution and race/prerequisite markers with exact-file `rg`; `rg -c` counted
  1,286 top-level passes and 26 package successes. All 16 FLOW backend top-level
  regressions (five quota, ten prior catalog, one new root-default test) passed.
- Earlier pre-correction `make vet test`, full verbose race and integration also
  passed; they did not catch C1/C2 and are superseded by the corrected Go gates
  above. Earlier race log:
  `<tool-output>/tool_0e0fc9401001vw7YrZNyzd2xft`.
  No failed shared gate was hidden; the two focused failures above are deliberate
  reproductions of the acceptance gaps.

Cleanup, preservation and limitations:

- SQL inspection after final testing found zero `flow0%` schemas, relations,
  functions or constraints. `docker stop aiproxy-flow05-20260926-9e36` succeeded;
  `docker ps -a --filter 'name=^/aiproxy-flow05-20260926-9e36$' --format '{{.ID}} {{.Names}}'`
  returned no rows, confirming `--rm` cleanup. No other container was stopped.
- Removed only the `.gitkeep` newly generated by this session's `make integration`
  with apply_patch; ignored production assets remain. Reviewed status/diffs against
  the incoming dirty inventory and scoped edits. Prior BOUNDARY/SAFE/LIFE/FLOW work
  and task statuses remain; only FLOW-05 status was updated. `git diff -- CHANGELOG.md`
  remains empty. No commits, nested agents or CHANGELOG edits.
- These are local PG17 deterministic fault/concurrency tests, mounted UI HTTP
  fixtures and local stub-backed App/binary integration tests. No hosted CI,
  real-provider traffic/credentials, browser performance/load measurement or
  network-partition/distributed-transaction testing is claimed. UI fixtures prove
  browser requests/reopen behavior; the new real App regression specifically proves
  DB/root-default/reset integration. Existing completion-based quota overshoot,
  30-second cache lag, multi-row quota-update atomicity, durable ledger retry and
  cross-resource/multi-instance activation remain the documented scope limits.

## Coordinator Final Closure

- Read the entire completed record, including scoped corrections, superseded
  verification failures, final acceptance audit and fixture cleanup. All five
  tasks are completed with Completion evidence and no unresolved execution blocker.
- Each task used a fresh sub-agent sequentially. FLOW-03 resumed in its same session
  to resolve the frontend gate before FLOW-04 began. FLOW-05 was independently
  reviewed by a session that implemented none of FLOW-01–04.
- Rechecked the final worktree, `git diff --check`, and
  `git diff --exit-code HEAD -- CHANGELOG.md`; both checks passed. Prior task work
  remains present. Verification and audit limits are recorded above; no hosted CI
  or real-provider execution is claimed.
