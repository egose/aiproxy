# Business Boundary And Codebase Health Review

Created: 2026-09-26 14:55:24 local time

## Objective And Product Context

aiproxy provides one OpenAI-compatible gateway over multiple upstream providers,
with aliases/failover, operator dashboards, and optional organization-owned keys
and quotas. Its business requirements include isolating tenants and credentials,
preserving spend accountability, trustworthy cost estimates, reliable live reload,
and predictable browser sessions. This plan closes evidence-backed gaps in those
workflows and improves shared-boundary encapsulation and regression coverage.

Scope: dashboard authorization, key expiry, durable spend, reload ownership,
alias billing retention/attribution, browser authentication lifecycle, and CI.
No edits to `CHANGELOG.md`. Contract changes are documented in the relevant
public documentation and in this task file. No provider expansion or broad rewrite.

## Analysis And Baseline

- Worktree was clean at review start. Read the requested skill at
  `task-as-you-go skill` and repository instructions.
- Read-only review session: `ses_f204be853ffeiRF7PF8V0z7TMr`.
- Reviewed product README/design, admin/OIDC/key/quota/store boundaries,
  authentication, accounting, runtime assembly, dashboard, browser services/hooks,
  and test workflow; coordinator inspected the principal finding locations.
- Deduplicated against `20260804-125911-codebase-health-review-remediation.md`,
  `20260823-112437-codebase-health-follow-up.md`,
  `20260908-074558-codebase-health-residual-gaps.md`, and
  `20260913-113100-ingress-secret-guardrails.md`. These are residual/new boundary
  defects, not reimplementations of their completed tasks.
- No baseline test suite run during analysis. Findings below are source-confirmed;
  regression reproduction is required during implementation. Docker is available
  for isolated PostgreSQL tests; never use an existing developer database.
- Limitations: no exhaustive provider, deployment/IaC, dependency, or accessibility
  audit; no real-provider experiments. Performance claims require measurement.
- Existing catalog-performance follow-up stays in the September residual plan;
  ingress SECRET-04 stays in its existing plan. New durable distributed billing,
  tenant-specific operator dashboards, and additional provider features need
  separate product design and are outside this bounded remediation objective.

## Execution And Verification Rules

Run tasks in listed order, each in a **fresh sub-agent session**, never concurrently.
Shared hotspots include `internal/httpapi`, `web-ui/src/hooks.ts`, documentation,
and this file. Agents set their task `in_progress` before work, then `completed`
only after acceptance and required checks pass. Append `Completion evidence`
with changed paths, actual commands/results, and limitations. Preserve historical
findings. Necessary follow-ups must be scoped explicitly, not hidden in evidence.

Priorities: P0 = cross-user confidentiality/control exposure; P1 = credential,
financial, or runtime-integrity defect; P2 = accuracy/UX/maintainability gap.

All commands run from `<repo-root>`. Database tests require
`AIPROXY_TEST_DATABASE_URL` pointing to a disposable PostgreSQL database. Test
packages that reset shared tables must run serially (`go test -p 1 ...`). Record
the disposable fixture and cleanup in the final evidence. Do not count skipped
database tests as verification. Use `apply_patch` for file edits and match local
style. No commits or CHANGELOG edits.

Final gates: `make vet test`, `make test-race` (serialize Go packages with
`GOFLAGS=-p=1` when sharing the fixture), `make integration`, `make docs-contract`,
`pnpm --filter @aiproxy/web-ui test`, `pnpm --filter @aiproxy/web-ui typecheck`,
`make lint-workflows`, and `git diff --check`. Integration builds the embedded UI
and host binary; builds must remain serialized. CI validation may use the repo's
actionlint tool or install that declared tool into a temporary location if absent.

### Task BOUNDARY-01: Restrict Global Dashboard Surfaces To Operators

Status: completed

Kind: defect

Priority: P0 — ordinary accounts can access global request data and decisions.

Suggested agent: dashboard authorization and operator UX implementer

Dependencies: none

Primary ownership: `internal/httpapi/dashboard.go`, dashboard/admin tests,
relevant web navigation/permission UI and public documentation.

Finding: `dashboardAuthenticated` accepts any valid admin-service JWT; handlers
return unscoped logs, payloads, quarantine captures and accept global exception
decisions. `TestDashboardAcceptsAdminJWTWithStore` explicitly expects a non-admin
JWT to succeed. This crosses organization boundaries when those features are enabled.

References: `internal/httpapi/dashboard.go` (`dashboardAuthenticated`,
`writeDashboardBlock`, `writeDashboardBlockDecision`);
`internal/httpapi/admin_oidc_e2e_test.go` (`TestDashboardAcceptsAdminJWTWithStore`).

Requirements:

1. Use a shared gate permitting the configured dashboard secret or a currently
   authorized system administrator, checking current stored authority consistently
   with existing admin routes. Ordinary/organization-admin JWTs must be denied.
2. Apply to every global dashboard route before data reads or writes; preserve
   token authentication and invalid-token throttling. Make UI access/error handling
   reflect the operator-only contract and document the changed JWT contract.

Acceptance criteria:

- Cover multiple organizations, ordinary/org-admin/system-admin identities,
  revoked/demoted authority, and every dashboard route with meaningful negative tests.
- Denial neither consumes take-once captures nor writes persistent decisions.
- Dashboard secret and authorized system-admin access still work.

Verification: database-backed focused `go test -p 1 ./internal/httpapi` and affected
frontend tests/typecheck; final shared gates.

Completion evidence:

- Changed server/auth/store: `internal/httpapi/dashboard.go`, `admin.go`,
  `internal/store/crud.go`; shared identity resolution uses current stored user
  authority, including promotion/demotion and disabled/deleted users.
- Regression coverage: `internal/httpapi/dashboard_operator_test.go`,
  `admin_oidc_e2e_test.go`, `admin_validate_test.go`,
  `admin_users_invites_test.go`. All seven dashboard routes are exercised across
  12 credential/authority cases, including members/admins of two organizations;
  denials perform zero dashboard reads, preserve take-once captures, and leave
  in-memory/persistent decisions unchanged. Temporarily restoring non-operator
  acceptance made every route fail, including capture consumption and disk writes.
- Changed UI: `web-ui/src/{app.tsx,hooks.ts,components/shell.tsx}`, pages
  `{requests,payloads,blocks}-page.tsx`, `services/dashboard.ts`, and new
  `dashboard-operator.test.tsx` / `services/dashboard.test.ts`. Navigation,
  direct links, query enablement, credential selection, and denial messages now
  reflect operator access; organization management remains available.
- Public contract updated in `README.md`, `docs/design.md`, and `AGENTS.md`.
- Passed with the supplied `AIPROXY_TEST_DATABASE_URL`:
  `go test -p 1 ./internal/httpapi -count=1` (14.325s) and
  `go test -race -p 1 ./internal/httpapi -run 'TestDashboard|TestBlockDecision|TestInviteValidation' -count=1`
  (14.091s; initial 120s compilation timeout passed on a longer-timeout retry).
  Database tests ran against disposable `aiproxy_review` at `127.0.0.1:39586`,
  with no prerequisite skips; test cleanup handles temporary files/new fixture
  rows, and the supplied PostgreSQL fixture remains running.
- Passed `pnpm --filter @aiproxy/web-ui test` (6 files, 34 tests),
  `pnpm --filter @aiproxy/web-ui typecheck`,
  `go vet ./internal/httpapi ./internal/store`, `make docs-contract`, and
  `git diff --check`.
- Necessary test reconciliation: validation fixtures now create a stored admin;
  the pre-existing invite test's missing-role rejection expectation was corrected
  to verify the handler's ordinary-member default, without changing invite behavior.
- Scope: final full gates remain BOUNDARY-08; authentication-generation cache
  isolation remains BOUNDARY-06. No CHANGELOG edits, commits, or spawned agents.

### Task BOUNDARY-02: Enforce Key Expiration During Live Authentication

Status: completed

Kind: defect

Priority: P1 — expiration currently waits for a catalog reload.

Suggested agent: authentication and key lifecycle implementer

Dependencies: BOUNDARY-01

Primary ownership: `internal/auth`, `internal/dbmerge`, focused HTTP tests and key docs.

Finding: `mergeKeys` filters already-expired keys at load time but discards expiry
when creating `DynamicClient`; `Authenticate` subsequently only checks token hashes.
Existing admin sharing tests cover expiry metadata, not live expiry without reload.

References: `internal/dbmerge/dbmerge.go` (`mergeKeys`);
`internal/auth/auth.go` (`DynamicClient`, `NewAuthenticatorWithClients`, `Authenticate`).

Requirements:

1. Carry immutable expiry into the shared auth boundary and reject at or after the
   deadline without database I/O or a scheduler per request.
2. Use deterministic time injection for boundary tests; preserve static/non-expiring
   credentials, rotation/revocation behavior, and standard auth error responses.

Acceptance criteria:

- Before/exactly-at/after expiry and expiry without reload are covered.
- Listing, billing, JSON inference and SSE requests reject expired keys before
  upstream I/O; merge tests prove persisted expiry reaches live authentication.

Verification: `go test -race -p 1 ./internal/auth ./internal/dbmerge ./internal/httpapi`
with the disposable database; final shared gates.

Completion evidence:

- Changed implementation: `internal/auth/auth.go`, `internal/dbmerge/dbmerge.go`.
  Merge copies persisted expiry into a value-owned dynamic credential; the live
  authenticator rejects at/after the deadline with `ErrInvalidToken`, using a
  constructor-injected clock (production default `time.Now`). No request-time
  database lookup, scheduler, or mutable expiry pointer is introduced.
- Changed tests: `internal/auth/auth_test.go`, new
  `internal/dbmerge/key_expiration_test.go` and
  `internal/httpapi/key_expiration_test.go`. Deterministic before/exactly-at/after
  boundaries use one live snapshot without reload. Coverage includes input and
  returned-principal mutation isolation, static/non-expiring keys, default clock,
  persisted PostgreSQL expiry, rotation retaining expiry, disabled/deleted keys
  disappearing from new snapshots, and existing snapshots remaining isolated.
- Public regression matrix covers models, billing, chat JSON/SSE (including an
  alias), and responses JSON/SSE across three clock boundaries and four credentials
  (72 requests). Expired keys receive the standard JSON `401` / `auth_failed` /
  `invalid client token` response with zero upstream calls; valid controls succeed.
- Reproduction: the focused command below, run before enabling the live expiry
  check, failed in all three packages, with expired-key data access and JSON/SSE
  upstream calls observed:
  `go test -p 1 ./internal/auth ./internal/dbmerge ./internal/httpapi -run 'TestDynamicClientExpiration|TestMergedKeyExpirationAndLifecycle|TestPublicEndpointsKeyExpirationWithoutReload' -count=1`.
  The HTTP assertion was also corrected to use the existing error `type` field.
- Passed with the supplied `AIPROXY_TEST_DATABASE_URL`:
  `go test -race -p 1 ./internal/auth ./internal/dbmerge ./internal/httpapi -count=1`
  (`auth` 1.045s, `dbmerge` 1.227s, `httpapi` 109.275s). Database tests used the
  disposable `aiproxy_review` fixture at `127.0.0.1:39586`; the new database
  regression ran without prerequisite skips and deletes its fixture rows and
  closes its store connection through test cleanup. The supplied fixture remains
  running; no other database was used.
- Passed `go vet ./internal/auth ./internal/dbmerge ./internal/httpapi`,
  `make docs-contract`, and `git diff --check`.
- Public contract documented in `website/docs/operations.md`; snapshot ownership,
  clock semantics, and lifecycle boundaries documented in `docs/design.md`.
- Limitations: expiry gates new authentication, not already-admitted requests or
  streams. Rotation, disabling/deletion, and expiry edits still require activation
  through the existing reload path. Final full gates remain BOUNDARY-08. Existing
  BOUNDARY-01 edits are preserved; no CHANGELOG edits, commits, or spawned agents.

### Task BOUNDARY-03: Preserve Spend Across Credential Deletion

Status: completed

Kind: defect

Priority: P1 — deleting a spent key erases ledger entries and restores budget headroom.

Suggested agent: PostgreSQL ledger integrity implementer

Dependencies: BOUNDARY-02

Primary ownership: new `internal/store/migrations` migration, ledger CRUD/models,
`internal/httpapi` quota/key regression tests, spend lifecycle documentation.

Finding: key owners may delete their keys, but the spend ledger foreign key uses
`ON DELETE CASCADE`; budget sums then lose those costs once cached totals refresh.

References: `internal/store/migrations/000007_quotas_ledger.sql` (`spend_ledger`);
`internal/store/crud.go` (`SumScopeSpend`);
`internal/httpapi/admin_keys.go` (DELETE, `canManageKey`);
`internal/httpapi/quota.go` (spend cache/budget comparison).

Requirements:

1. Add a forward-compatible migration preserving historical spend and ownership
   when keys are deleted (nullable historical reference or tombstone); do not edit
   applied migration history. Preserve explicit quota-reset behavior.
2. Handle an admitted request finishing after key deletion without dropping its
   charge. Keep persistence/query behavior encapsulated in the store layer.

Acceptance criteria:

- User/team key spend survives deletion, replacement key creation, cache refresh,
  and a new tracker/store instance; exhausted scope remains blocked.
- Upgrade from the previous migration retains existing rows and totals.
- Request-completion/key-deletion ordering has regression coverage against real PG.

Verification: database-backed `go test -race -p 1 ./internal/store ./internal/httpapi`;
final shared gates.

Completion evidence:

- Added `internal/store/migrations/000008_preserve_key_spend.sql`. Durable
  `spend_key_identities` retain only key/organization UUIDs. Backfill and an atomic
  key-insert trigger cover both existing and future credentials; a key-table lock
  closes the backfill/write gap. The ledger's composite foreign key preserves
  attribution after deletion and rejects unknown/cross-organization identities.
  Applied migration history is unchanged. Storage integrity is database-owned;
  existing store models/CRUD and HTTP completion writes need no fallback or
  swallowed foreign-key errors.
- Added `internal/store/spend_ledger_test.go`: real PostgreSQL upgrade from 000007
  retains row IDs, timestamps, ownership, models, tokens, costs, totals and reset
  offsets; repeated migration is a no-op. Covers late charges for migrated keys
  with/without previous spend, charge-first/delete-first/concurrent orderings,
  invalid identity rejection, and organization deletion cleanup.
- Added `internal/httpapi/quota_key_deletion_test.go`: user/team ownership ×
  JSON/SSE regressions hold two admitted upstream requests, complete one before
  owner-authorized key deletion and the other after deletion and explicit reset.
  Same-name replacement keys remain budget-blocked after cache expiry and with a
  fresh store/tracker. Checks exact durable usage, zero upstream calls on budget
  denial, shared-access non-owner isolation, organization filtering, denied
  non-org-admin resets, and preserved history plus post-reset late spend.
- Reproduced before the migration with
  `go test -p 1 ./internal/store -run TestSpendLedger -count=1 -v`: charge-first
  lost its row; delete-first/concurrent completion failed the old foreign key;
  the old schema also accepted a mismatched key/organization pair. After the fix,
  the same focused command passed (0.632s), and
  `go test -p 1 ./internal/httpapi -run TestQuotaSpendSurvivesKeyDeletion -count=1 -v`
  passed all four scenarios (2.208s).
- Required verification passed with the supplied `AIPROXY_TEST_DATABASE_URL`:
  `go test -race -p 1 ./internal/store ./internal/httpapi -count=1`
  (`store` 2.308s, `httpapi` 111.277s), including the final expanded upgrade and
  organization-cleanup assertions. No database prerequisite skips in the focused
  regressions. Passed `go vet ./internal/store ./internal/httpapi`,
  `make docs-contract`, `git diff --check`, and clean `gofmt -d` output for both
  new test files.
- Documented ownership, identity lifetime, quota reset ordering and limitations
  in `docs/design.md` and `website/docs/operations.md`. A reset changes only the
  spend offset; charges recorded after its sum count against the reset budget.
- Used only disposable `aiproxy_review` at `127.0.0.1:39586`. Store tests create
  unique schemas in that database and drop them via cleanup; HTTP fixtures delete
  their organizations/users and close their connections. The supplied service
  remains running. Optional external cleanup inspection could not use `psql`
  because that host executable is absent; actual database verification used the
  Go PostgreSQL driver.
- Limitations: already-erased historical spend cannot be recovered. Identity
  rows persist for the organization's lifetime, including unspent keys, and
  contain no credentials. Organization deletion still removes its ledger;
  user/team deletion retains existing nullable ownership semantics. Existing
  30-second cache lag, admitted-request budget overshoot and lack of durable
  retry on unrelated database failures remain. Inconsistent legacy
  key/organization ledger pairs reject migration atomically. Final full gates
  remain BOUNDARY-08. Preserved BOUNDARY-01/02 edits; no CHANGELOG edits, commits,
  spawned agents or other task-status changes.

### Task BOUNDARY-04: Prepare Reload Resources Before Active Mutation

Status: completed

Kind: defect

Priority: P1 — failed reloads can mutate live probes and leak candidate resources.

Suggested agent: runtime lifecycle and resource-ownership implementer

Dependencies: BOUNDARY-03

Primary ownership: `internal/app/app.go`, app lifecycle/guardrail reload tests,
`internal/healthcheck` only if needed, lifecycle design documentation.

Finding: `App.Reload` invokes `SetTracker`/`SetProviders` before fallible sink,
scanner and exception-file preparation. Probes start/stop immediately. `Build`
likewise starts resources before later initialization can fail. Existing early
validation-failure tests do not exercise late preparation failures.

References: `internal/app/app.go` (`Build`, `Reload`, `reloadPayloadSinks`,
`reloadHealthTracker`); `internal/healthcheck/healthcheck.go` (`Manager.SetProviders`);
`internal/app/guardrails_test.go` (failed reload tests).

Requirements:

1. Stage fallible dependencies with explicit candidate cleanup; commit active
   mutations only after preparation succeeds. Never close reused active resources
   during rollback. Apply failure cleanup ownership to Build as well.
2. Preserve successful reload behavior, existing token persistence and unchanged
   routing state; explain lifecycle boundaries in design docs instead of broad rewrites.
3. Scoped lifecycle finding during implementation: a canceled replaced probe can
   overwrite the replacement's health with its cancellation failure. Discard canceled
   results and cover a held old probe completing after a healthy replacement.

Acceptance criteria:

- A changed probe endpoint plus corrupt/unreadable exceptions or sink failure
  rejects reload while old routing/probes/health remain usable; no candidate probes
  survive and newly opened resources are closed.
- Successful reload and concurrent requests pass race-enabled tests.
- Build failure after resource acquisition demonstrates cleanup.

Verification: `go test -race ./internal/app ./internal/healthcheck ./internal/providerhealth`;
final shared gates.

Completion evidence:

- Changed `internal/app/app.go`: explicit candidate ownership and deferred failure
  cleanup cover new health backends, disk/MongoDB sinks, and Build's database store.
  Reused live resources are excluded from rollback ownership. Original preparation
  errors survive cleanup errors. Build starts probes only after construction succeeds;
  reload delays live health/probe/metrics mutation until all preparation succeeds.
  Required dashboard-token persistence is the final fallible preparation step.
- Added `internal/app/reload_resources_test.go`: changed endpoint/path plus corrupt
  exceptions (reused/new resources), unreadable exceptions, invalid disk directory,
  MongoDB timeout after disk acquisition, and dashboard-token persistence failure.
  Checks retained config/resolver/scanner/exceptions, unchanged health membership,
  old-probe failure/recovery and readiness, direct/alias routing, active payload
  recording, zero rejected-candidate calls, and candidate retention-worker cleanup.
  Also covers counted health-backend ownership/close errors, successful resource
  reuse, concurrent requests across endpoint/tracker activation, subsequent sink
  replacement/retirement, and successful app shutdown.
- Build regressions verify worker cleanup after exception/MongoDB failure and zero
  probe calls. The PostgreSQL regression observes `pg_stat_activity` for a unique
  application name: failed Build leaves no connection, successful Build retains a
  usable connection, and `App.Close` closes the store.
- Necessary scoped correction in `internal/healthcheck/{healthcheck.go,healthcheck_test.go}`:
  discard results from canceled probe lifetimes. The deterministic held-old-probe
  test failed before the fix, replacing the healthy new path with the canceled old
  path and marking its tracker unhealthy. This extends the successful-reload
  lifecycle coverage described in requirement 3; no other task was implemented.
- Reproduction: temporarily restoring early live health/probe mutation and disabling
  candidate cleanup made
  `go test ./internal/app -run 'TestReloadLateFailurePreservesLiveResources/(corrupt-reused|corrupt-new)$|TestBuildLateFailureCleansResources/exceptions$' -count=1`
  fail on live health-catalog mutation and leaked reload/Build payload workers.
  `go test ./internal/healthcheck -run TestReplacedProbeCannotOverwriteActiveHealth -count=1`
  independently reproduced the retired-probe overwrite before its fix.
- Passed focused late-failure/concurrent-success app tests (12.907s), and the
  database/cleanup focused tests with `-v` (0.557s, no prerequisite skips).
  Final required command passed with the supplied `AIPROXY_TEST_DATABASE_URL`:
  `go test -race ./internal/app ./internal/healthcheck ./internal/providerhealth -count=1`
  (`app` 23.109s, `healthcheck` 1.594s, `providerhealth` 1.385s).
  Passed `go vet ./internal/app ./internal/healthcheck ./internal/providerhealth`,
  `make docs-contract`, `git diff --check`, and clean `gofmt -d` output for all
  four changed Go files.
- `docs/design.md` documents preparation, activation, ownership transfer, cleanup,
  token ordering, and canceled-probe handling. Existing request-snapshot semantics
  are retained; successful activation does not drain admitted requests or atomically
  switch every metrics/probe observation. Preparation can create directories and
  perform sink initialization/retention work; it is not an external transaction.
  MongoDB failure tests use a local nonresponding listener, not a live MongoDB service.
- Used only disposable PostgreSQL `aiproxy_review` at `127.0.0.1:39586`; the Build
  regression creates a unique schema, migrates it, and drops it via cleanup while
  closing fixture connections. The supplied service remains running. Preserved
  all BOUNDARY-01–03 changes. Full shared gates remain BOUNDARY-08; no CHANGELOG
  edits, commits, subagents, or other task-status changes.

### Task BOUNDARY-05: Attribute Alias Estimates To Retained Alias Traffic

Status: completed

Kind: defect

Priority: P2 — shared targets and lifetime counters inflate alias estimates.

Suggested agent: usage accounting and cost attribution implementer

Dependencies: BOUNDARY-04

Primary ownership: `internal/accounting`, `internal/httpapi/response.go`,
billing tests and accounting documentation.

Finding: upstream aggregate keys omit the requested alias/public model and retain
lifetime totals. `aliasCostEntries` applies matching target totals to each alias
row, mixing direct and other-alias traffic and outliving the rolling billing window.

References: `internal/accounting/accounting.go` (`upstreamKey`, `UpstreamSummaries`,
recording/pruning); `internal/httpapi/response.go` (`billingAliasCost`, `aliasCostEntries`);
`internal/httpapi/handler_test.go` (existing isolated alias cost tests).

Requirements:

1. Add public-model attribution and the same retention window as billing summaries
   at a shared accounting boundary. Preserve intentionally lifetime dashboard counters.
2. Keep missing-price behavior explicit; avoid misleading partial estimates.
   Bound new retention state and preserve tenant/client filtering. Do not claim an
   unmeasured performance improvement.

Acceptance criteria:

- Two aliases sharing a target plus direct traffic each receive only their own
  cost; cover distinct tenants, multiple target prices, and missing prices.
- Advancing a deterministic clock beyond the rolling window removes expired cost
  along with usage, without resetting lifetime dashboard counters.

Verification: `go test -race ./internal/accounting ./internal/httpapi` (with `-p 1`
and the database when running DB tests); final shared gates.

Completion evidence:

- Changed `internal/accounting/accounting.go`: retained bucket keys include public
  model plus resolved provider/configured target model. `BillingSummaries` returns
  both usage views under one lock and one pruning boundary. The existing one-minute
  buckets own all retained attribution; expired keys are removed together, including
  late events, and future event timestamps are clamped for bucket placement. Added
  a constructor-injected clock and an attributed snapshot for the memory recorder.
  Lifetime dashboard provider/upstream counters and their grouping remain intact.
- Changed `internal/httpapi/{handler.go,response.go}`: billing uses the retained
  snapshot, matches exact public model/tenant/client/operation/status, and sums actual
  target token costs. It no longer redistributes current target totals or guesses
  even/request-weighted splits. All request/token counters must reconcile; a missing
  used-target price or required token-category rate omits the entire estimate.
  Unused unpriced targets do not affect estimates. Current catalog model prices
  apply even if the recorded target is no longer a member of the alias.
- Scoped identity correction: tenantless client filtering now excludes same-named
  clients in other tenants. Scoped pricing decision: config cannot distinguish an
  omitted rate from explicit zero, so billing conservatively omits estimates when
  a consumed category has no positive rate. Existing explicit cache-read/write
  input-rate fallbacks are honored. Dashboard and durable ledger pricing are unchanged.
- Added `internal/accounting/billing_test.go` and
  `internal/httpapi/billing_attribution_test.go`; updated the upstream-token fixture
  in `internal/httpapi/handler_test.go`. Coverage includes two shared aliases plus
  direct traffic, two tenants/multiple clients/tenantless identities, differing
  prices, missing used/unused target prices, missing category rates, attribution
  mismatches, cache-rate fallbacks, concurrent atomic/detached snapshots, and actual
  inference followed by billing. Deterministic inclusive-cutoff/just-after/fully
  expired checks remove usage and costs together while retaining lifetime counters.
  Rolling-state tests exercise 2,000 future/expired timestamp pairs and three days
  of minute buckets, checking the 1,441-bucket bound and expired-key reclamation.
- Reproduced before implementation:
  `go test -p 1 ./internal/httpapi -run TestBillingRetainedAliasAttribution -count=1`
  failed for inflated shared-alias costs across every identity and emitted partial
  estimates with missing prices. After implementation, focused billing/accounting
  checks passed (`accounting` 0.138s, `httpapi` 0.043s). An intermediate test compile
  failure referenced nonexistent `provider.Request.Model`; corrected the assertion
  to the actual `UpstreamModel` field before verification.
- Required verification passed with the supplied `AIPROXY_TEST_DATABASE_URL`:
  `go test -race -p 1 ./internal/accounting ./internal/httpapi -count=1`
  (`accounting` 2.306s, `httpapi` 121.970s). Database prerequisites were supplied;
  HTTP fixture helpers skip only when the URL is absent and fail on connection or
  migration errors. Used only disposable `aiproxy_review` at `127.0.0.1:39586`.
  Existing database tests own row/connection cleanup; this task adds no database
  resources. `docker ps` confirmed `aiproxy-review-20260926-145524` remains running
  on that port after checks.
- Passed `go vet ./internal/accounting ./internal/httpapi`, `make docs-contract`,
  `go test ./internal/dashboard ./internal/dashrpc -count=1`
  (`dashboard` 0.381s, `dashrpc` 0.228s), `git diff --check`, and clean `gofmt -d`
  output for all six changed/new Go files.
- Public semantics documented in `website/docs/api-reference.md`; accounting
  snapshot ownership, retention bounds, pricing completeness and lifetime separation
  documented in `docs/design.md`. Limitations: process-local rolling estimates use
  current rather than historical prices; bucket granularity can expire an event up
  to one minute early. Retained entries scale with distinct dimensions in the window,
  idle expired state is reclaimed on the next read/write, and upstream usage reports
  are not guaranteed complete. No performance improvement is claimed.
- Preserved BOUNDARY-01–04 edits. Final shared gates remain BOUNDARY-08. No CHANGELOG
  edits, commits, subagents, or other task-status changes.

### Task BOUNDARY-06: Isolate Browser Data Across Authentication Changes

Status: completed

Kind: defect

Priority: P2 — account replacement and refresh failure can reuse previous-user data.

Suggested agent: browser authentication/cache lifecycle implementer

Dependencies: BOUNDARY-05

Primary ownership: `web-ui/src/store.ts`, `hooks.ts`, admin/dashboard services,
login/logout flows and focused frontend regression tests.

Finding: sensitive query keys use only authed/anon or session/token state. Logout
clears queries, but refresh failure/account replacement does not isolate cached
identity and data, and delayed old requests may arrive after replacement.

References: `web-ui/src/hooks.ts` (query keys); `web-ui/src/store.ts`
(`setAdminSession`, `clearAdminSession`, `setDashboardToken`);
`web-ui/src/services/admin.ts` (refresh failure); login/logout pages.

Requirements:

1. Centralize authentication-generation isolation for sensitive queries, cancel/
   remove obsolete queries, and reject stale async session updates. Never put raw
   tokens in query keys. Reset stale organization selection on identity replacement.
2. Keep normal same-user token refresh functional without unnecessary cache churn;
   handle dashboard-token replacement and all login/logout entry points consistently.

Acceptance criteria:

- A → refresh failure → B, direct A → B, delayed A data/refresh responses,
  dashboard-token replacement and ordinary logout cannot expose A data under B.
- Same-session refresh preserves valid state and permission/navigation tests pass.

Verification: `pnpm --filter @aiproxy/web-ui test` and
`pnpm --filter @aiproxy/web-ui typecheck`; final embedded-UI integration build.

Verification finding for the later BOUNDARY-07/08 gate owners: the declared
`typecheck` script runs `tsc --noEmit` on a reference-root config with `files: []`
and does not check the application projects. Explicit application checking first
fails on the existing TS7-removed `baseUrl` option. With only that option disabled
in a temporary config, nine existing diagnostics remain in
`src/components/data-table.tsx` (unconstrained `TData` versus `RowData`) and
`src/pages/admin-keys-page.tsx` (form input/output types for defaulted
`allowed_models`). The later verification work must resolve this gate gap before
claiming a full application typecheck. BOUNDARY-06 checks its boundary/services,
hooks, authentication pages and new regressions explicitly; it does not repair
unrelated table/form types or change the later tasks' statuses.

Completion evidence:

- Added `web-ui/src/{auth-lifecycle.ts,sensitive-query.ts}`; changed `store.ts`,
  `hooks.ts`, and `services/{admin,client,dashboard}.ts`. One non-secret generation
  scopes every sensitive query, cancels/removes obsolete active and inactive
  entries, clears obsolete mutation-cache entries, and aborts authenticated HTTP
  requests. Synchronous request credential/generation capture and response/query
  guards reject delayed results. Existing query prefixes still support invalidation;
  the public status query is retained. Weak query-client references avoid owning
  abandoned clients; `web-ui/tsconfig.app.json` adds the corresponding WeakRef lib.
- Account installation/clearing resets organization selection and advances a
  separate session epoch. Refreshes are single-flight per session; late old-token
  `401`s reuse the refreshed token. Same-session refresh preserves generation,
  cached data and selection. Old refresh success/failure cannot overwrite/clear B
  or retry using B's credential. Login results are session-guarded; logout clears
  locally before separately revoking captured credentials, including the dashboard
  token. Dashboard-token validation commits only after success in its generation,
  without temporarily installing or later restoring an old credential.
- Changed `web-ui/src/{app.tsx,main.tsx,components/shell.tsx}` and pages
  `{login,logout,oidc-callback,token}-page.tsx`. The app/dialog provider is keyed by
  generation to discard local detail/form/secret state. Organization-creation
  continuations check their generation before and after refetch; OIDC consumes its
  fragment once under Strict Mode. Audited all query hooks, service exports, store
  writes, password/OIDC/token/logout flows, organization selectors, and the public
  registration/status paths (registration installs no session).
- Added `web-ui/src/{auth-lifecycle.test.tsx,auth-entrypoints.test.tsx}` and
  `services/auth-lifecycle.test.ts`; updated `dashboard-operator.test.tsx` and
  `app.test.tsx` for provider ownership. Coverage exercises all 18 sensitive query
  hooks, all 48 authenticated admin service functions, and all six dashboard
  service functions with both credential sources. Includes cached and held A data,
  A → refresh failure → B with a mounted real-service query, direct replacement,
  delayed refresh success/failure/401s, concurrent refresh and late 401 retry,
  logout success/failure, org selection, token validation/sign-out, raw-token-free
  keys, inactive cache cleanup, password/OIDC entry points, and removal of A's
  visible payload/operator navigation while B authority is pending/denied.
- Reproduction by temporarily bypassing the sensitive-query wrapper:
  `pnpm --filter @aiproxy/web-ui exec vitest run src/auth-lifecycle.test.tsx -t 'me isolates|providers isolates'`
  failed both selected cases with A's cached data surviving replacement. Temporarily
  disabling the session-epoch assertion made
  `pnpm --filter @aiproxy/web-ui exec vitest run src/services/auth-lifecycle.test.ts -t 'ignores delayed A refresh'`
  fail both selected cases: late success installed A, late failure cleared B.
  Restored both guards before final verification. Initial operator-test assertions
  also needed to observe navigation atomically across the intentional org-selection
  remount; original denial/access expectations are preserved.
- Passed `pnpm --filter @aiproxy/web-ui test` (9 files, 136 tests, 11.02s),
  `pnpm --filter @aiproxy/web-ui typecheck`, `make docs-contract`, and
  `git diff --check`. Additionally passed an actual focused application check:
  `pnpm --filter @aiproxy/web-ui exec tsc -p <repo-root>/tmp/boundary-06-tsconfig.json --noEmit`.
  That temporary config extends the application config, disables only `baseUrl`,
  and includes the boundary, store, hooks, all services, authentication entry-point
  pages via their new tests, and test setup. The full-project temporary-config
  check fails only on the nine pre-existing table/form diagnostics recorded above;
  it is not claimed as a passing full application typecheck.
- Contract documented in `docs/design.md` and `website/docs/operations.md`.
  Limits: generations are per loaded tab, without cross-tab storage synchronization;
  aborting/discarding a result cannot undo server-admitted mutations. Real providers,
  database and embedded-UI integration builds were not needed/run for this task.
  Final integration/shared gates remain later tasks. No database resources touched,
  CHANGELOG edits, commits, or subagents. Preserved BOUNDARY-01–05 work, including
  operator UI, and changed only BOUNDARY-06's status.

### Task BOUNDARY-07: Exercise Database And Browser Boundaries In CI

Status: completed

Owner: fresh isolated sequential BOUNDARY-07 session; BOUNDARY-01–06 completed.

Kind: improvement

Priority: P2 — the current default workflow silently skips DB coverage and omits UI tests.

Suggested agent: CI regression-gate implementer

Dependencies: BOUNDARY-06

Primary ownership: `.github/workflows/test.yml`, focused developer testing docs;
`web-ui/package.json`, TypeScript project configs and narrowly scoped source
compatibility fixes needed for a real frontend typecheck; test fixture helpers
only if essential to reliable CI.

Finding: the Test workflow has no PostgreSQL service or test URL and does not run
web-ui tests/typecheck. Store/admin helpers skip when the database URL is missing.
Thus the security, ledger, and browser regressions above would not be enforced.

References: `.github/workflows/test.yml` (`unit-test`, `integration-test`);
`web-ui/package.json` (test/typecheck scripts); DB test helpers.

Requirements:

1. Add an isolated PostgreSQL-backed job/gate for the relevant Go packages, with
   readiness checks and serial package execution where fixtures share state.
2. Run frontend tests/typecheck using frozen lockfile dependencies and existing
   setup conventions; document equivalent local verification. No real-provider access.
3. Scope addition from BOUNDARY-06 verification: the existing `tsc --noEmit`
   command does not traverse the empty reference-root project. Make typecheck
   and build check the actual application and tooling projects, fix the exposed
   configuration/source compatibility errors without disabling type safety, and
   prove a deliberate source type error causes the gate to fail before restoration.

Acceptance criteria:

- Workflow syntax/actionlint passes; execute the same DB and browser commands
  locally against a disposable database with no prerequisite-related skips.
- Existing unit/race/integration jobs remain valid; no production secrets needed.
- Real frontend application/tooling typechecking passes and rejects a temporary
  deliberate type-error mutation; no empty-root false-positive remains.

Verification: `make lint-workflows`, job-equivalent DB/browser commands,
and final shared gates. Actual hosted CI execution is outside the local session.

Completion evidence:

- Changed `.github/workflows/test.yml`: added isolated PostgreSQL 17 service with
  `pg_isready` health checks (5s interval/timeout, 12 retries), dynamically mapped
  port, fixture-only credentials and an explicitly supplied test URL. The new job
  runs all four DB-owning packages with `-race -p 1 -count=1 -v`. Added frontend
  tests/typecheck using the existing pinned checkout/shared asdf setup and
  `pnpm install --frozen-lockfile`. The existing integration job now also installs
  frozen dependencies before its embedded-UI build; existing unit/race/build and
  integration commands remain in place. No database fixture-helper changes needed.
- Changed `web-ui/package.json` and `tsconfig.app.json`: `typecheck` uses
  `tsc -b --force`, traversing both application and tooling references; `build`
  invokes that same check before Vite. Removed the TS7-unsupported `baseUrl` while
  retaining relative path mappings and BOUNDARY-06's WeakRef library. Strict,
  unused-code and other safety checks, project includes and `noEmit` remain intact.
- Reproduced the nine existing diagnostics under the real gate. Narrow source
  fixes in `web-ui/src/components/data-table.tsx` constrain generic rows to the
  library's `RowData`; `src/pages/admin-keys-page.tsx` distinguishes Zod input from
  parsed output in form/resolver/submission types. Schema defaults, validation and
  runtime behavior are preserved; no suppression, unsafe cast or dependency change.
- Negative verification: temporarily passed `7` to `useState<string>` in the
  actual table source and a string to Vite's numeric server port. Both
  `pnpm --filter @aiproxy/web-ui typecheck` and
  `pnpm --filter @aiproxy/web-ui build` exited 1 with application `TS2345` and
  tooling `TS2769`; build stopped before bundling. Restored both mutations, then
  passed frontend tests (9 files, 136 tests, 27.77s), typecheck and build (8,757
  modules, Vite 720ms). `vite.config.ts` has no remaining diff.
- Passed the exact DB job command with the supplied `AIPROXY_TEST_DATABASE_URL`:
  `go test -race -p 1 -count=1 -v ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app`
  (`store` 2.517s, `dbmerge` 1.344s, `httpapi` 185.382s, `app` 27.265s). Inspected
  verbose output: all four packages passed and no tests skipped. Used only the
  disposable `aiproxy_review` fixture at `127.0.0.1:39586`. Tests own their fixture
  cleanup; no additional database service was created. Final `docker ps` and
  `docker exec aiproxy-review-20260926-145524 pg_isready -U aiproxy_review -d aiproxy_review`
  confirmed the supplied PostgreSQL 17 service remains running and accepting connections.
- Passed `pnpm install --frozen-lockfile` (lockfile unchanged), actual
  `make lint-workflows` using installed actionlint 1.7.12, `make docs-contract`,
  `AIPROXY_TEST_DATABASE_URL=<supplied fixture URL> GOFLAGS=-p=1 make vet test`,
  and `git diff --check`. Local versions match declarations: Node 26.8.1,
  pnpm 11.24.0, Go 1.27.1 and frontend TypeScript 7.0.2. Actionlint initially
  rejected job-level service-port interpolation; moved it to step env with a
  string port key and reran successfully. The first local vet/test run overlapped
  Vite output replacement and failed on removed embedded asset filenames; reran
  after the UI build finished and all packages passed.
- Updated README's developer testing section with disposable PG creation/readiness/
  cleanup, exact DB/browser CI commands, shared-table serialization, real project
  checking, and the requirement to finish UI builds before Go vet/tests read embedded
  assets. Reviewed final worktree and confirmed CHANGELOG and lockfile are untouched.
  Preserved BOUNDARY-01–06 edits; no commits, subagents or other task-status changes.
- Limitations: hosted Actions execution and real-provider access were not performed.
  Full-plan race/integration verification and independent review remain
  BOUNDARY-08; this session verified the new DB/frontend gates and local UI build,
  plus the required default Go sanity check. The supplied fixture is intentionally
  left running for the next sequential session.

### Task BOUNDARY-08: Independently Review And Verify The Complete Plan

Status: completed

Owner: fresh independent BOUNDARY-08 reviewer; dependencies 01–07 report completed.

Kind: improvement

Priority: P1 — verify cross-boundary behavior and completion claims together.

Suggested agent: independent integration reviewer (not a previous implementer)

Dependencies: BOUNDARY-01, BOUNDARY-02, BOUNDARY-03, BOUNDARY-04, BOUNDARY-05,
BOUNDARY-06, BOUNDARY-07

Primary ownership: review entire change set and this execution record; focused
corrections only when needed to meet already-defined acceptance criteria.

Finding: independently passing patches must jointly uphold product contracts,
particularly operator access, session transitions, ledger retention and reload rollback.

References: preceding tasks, their changed paths/tests and Completion evidence.

Requirements:

1. Inspect every task's acceptance evidence, source changes, docs, alternate
   access paths, retained state bounds, and cleanup behavior. Correct scoped issues
   and add meaningful regressions if needed; explicitly document scope changes.
2. Run all final shared gates with a real disposable database for DB tests.
   Verify `CHANGELOG.md` is untouched. Review `git diff --check` and worktree.

Acceptance criteria:

- Every task has fulfilled acceptance criteria and actual Completion evidence;
  all required checks pass and no unresolved blocker is described as completed.
- Public docs agree with new contracts; no inappropriate cross-identity data,
  unbounded billing state, or candidate-resource leaks found in inspected changes.
- Record limitations (hosted CI, external providers, audit scope) honestly.

Verification: all final shared gates and manual source/evidence review.

Scoped review addition (BOUNDARY-04 requirement 3): the canceled-context check
protects manager status but releases the lock before tracker/metric publication.
A held old backend write can finish after replacement success and overwrite live
health. Add a deterministic held-publication regression and serialize publication
with probe replacement; verify the successful replacement ultimately stays healthy.

Completion evidence:

- Independent review: fresh reviewer, not an earlier implementer. Read the entire
  plan, shared rules, all acceptance/Completion evidence, and `AGENTS.md`; set only
  BOUNDARY-08 in progress before inspection. Reviewed the tracked diff and all new
  implementation/regression files against their surrounding source and public
  contracts. No remaining acceptance blocker found after the correction below.
- Correction: `internal/healthcheck/{healthcheck.go,healthcheck_test.go}` now keeps
  status, metrics and tracker publication inside the manager lock. The new
  `TestProbePublicationCompletesBeforeReplacement` holds a real tracker backend
  failure write while replacement is attempted, then verifies replacement health.
  Before the fix, `GOFLAGS=-p=1 go test ./internal/healthcheck -run
TestProbePublicationCompletesBeforeReplacement -count=1` failed with
  `replacement activated while old health publication was outstanding`.
  Afterward `GOFLAGS=-p=1 go test -race ./internal/healthcheck -count=1` passed
  (1.525s), including the existing canceled-transport regression. `docs/design.md`
  explicitly records serialization and its tradeoff: replacement/status reads can
  wait for a backend write. This is the scoped BOUNDARY-04 addition above.

Acceptance/status audit:

| Task        | Final status | Independently checked acceptance                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| ----------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| BOUNDARY-01 | completed    | All seven dashboard routes gate before reads/writes; 12 credential cases cover two organizations, members/org-admins, stored promotion/demotion, disabled/deleted users, secret and JWT success, invalid/missing credentials. Denial preserves take-once captures and disk/in-memory decisions. Traced the sole HTTP `VerifyAccess` call through shared `adminClaims`, including admin/org/team/quota/provider/key/alias/user/invite/OIDC-config routes; authority is fetched by stored user ID. Existing throttling and UI direct-link/navigation/error tests remain effective.                                                |
| BOUNDARY-02 | completed    | Persisted expiry is copied by merge, supplied by app dependency assembly, and checked in shared live auth at/beyond the deadline. Merge/lifecycle regression and 72 public requests cover before/at/after expiry, models, billing, JSON/SSE chat/responses, alias routing, standard 401 and zero rejected upstream calls; static/non-expiring credentials survive.                                                                                                                                                                                                                                                              |
| BOUNDARY-03 | completed    | Reviewed transaction-wrapped migration 000008, key-write lock before backfill, atomic insert trigger and composite historical identity FK. Applied 000001–000007 files are unchanged against HEAD. Real PG tests preserve rows/ownership/timestamps/totals/reset offsets upgrading from 000007, and cover repeat migration, unknown/cross-org identities, charge-first/delete-first/concurrent completion and organization cleanup. User/team × JSON/SSE tests verify owner-authorized deletion, replacement keys, expired cache/fresh store, budget denial before I/O, explicit reset and post-reset late spend.               |
| BOUNDARY-04 | completed    | Candidate ownership excludes reused resources; partial sink constructors clean their acquisitions; named-error defers close newly acquired resources on late failure. Token persistence is the last fallible reload preparation; Build starts probes last. Tests cover corrupt/unreadable exceptions, disk/Mongo failure, token failure, live routing/readiness/probe recovery, worker cleanup, PG connection cleanup, reused ownership, close errors and concurrent successful activation. Corrected the remaining publication race above.                                                                                     |
| BOUNDARY-05 | completed    | Public-model/provider/target dimensions share one pruned billing snapshot; exact tenant/client/operation/status matching excludes direct/other-alias traffic and same-name cross-tenant clients. Tests cover distinct target prices, missing used/unused prices, category completeness/cache fallbacks, mismatched attribution and actual inference-to-billing flow. Clock tests verify inclusive cutoff, expired/future timestamps, 1,441-bucket bound and key reclamation while lifetime dashboard counters survive. No new lifetime billing-attribution map exists.                                                          |
| BOUNDARY-06 | completed    | All 18 sensitive query hooks use generation keys; all 48 authenticated admin and six dashboard services use guarded clients. Reviewed synchronous credential capture, cancellation/removal, detached stale response guards, separate session epoch, single-flight refresh, delayed 401 retry, login/OIDC/logout/token entry points, organization reset and generation-keyed local dialogs. Regressions cover A→failure→B, direct replacement, late data/refresh success/failure, token changes, immediate logout, same-session cache preservation and pending/denied B operator UI. Raw credentials are absent from query keys. |
| BOUNDARY-07 | completed    | PostgreSQL 17 service readiness, mapped fixture URL and `-race -p 1 -count=1 -v` DB job cover all four DB-owning packages; frozen-lockfile frontend and integration dependency installs follow shared setup conventions. Existing unit/race/build/integration jobs remain present. Actual `tsc -b --force` traverses application/test and Vite/Vitest projects with strict/noEmit checks; build invokes it. Reviewed prior deliberate application/tooling error failures and narrow RowData/Zod fixes; independent full typecheck and embedded build pass. Actionlint passes.                                                   |
| BOUNDARY-08 | completed    | All criteria above, public docs, complete final gates, fixture execution, migration/CHANGELOG history and tracked/untracked artifact review completed. No unresolved blocker is labeled complete.                                                                                                                                                                                                                                                                                                                                                                                                                               |

Final command results (serialized, no tests/builds overlapped):

- Cleared Go test-result cache with `go clean -testcache`. For every following Go
  gate used `AIPROXY_TEST_DATABASE_URL=postgres://aiproxy_review:review_fixture_only@127.0.0.1:39586/aiproxy_review?sslmode=disable` // pragma: allowlist secret
  and `GOFLAGS=-p=1`.
- `make vet test`: passed all packages; DB packages ran uncached (`store` 1.160s,
  `dbmerge` 0.224s, `httpapi` 19.892s, `app` 16.065s).
- `make test-race`: passed all packages; DB packages ran uncached (`store` 1.942s,
  `dbmerge` 1.320s, `httpapi` 133.941s, `app` 24.537s).
- `make integration`: passed; actual application/tooling check, Vite embedded UI
  build (8,757 modules), CGO-disabled host binary and hermetic binary tests (1.625s).
- `make docs-contract`: passed, documentation contract matrices match.
- `pnpm --filter @aiproxy/web-ui test`: passed, 9 files / 136 tests (13.37s).
- `pnpm --filter @aiproxy/web-ui typecheck`: passed, actual `tsc -b --force`.
- `make lint-workflows`: passed using actionlint 1.7.12.
- `git diff --check`: passed. `gofmt -d` on both reviewer-changed Go files was empty.
- Additional explicit DB execution evidence, with the same URL/GOFLAGS:
  `go test -race -count=1 -v ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app -run 'TestSpendLedger|TestMergedKeyExpirationAndLifecycle|TestDashboardOperatorRoutes|TestPublicEndpointsKeyExpirationWithoutReload|TestQuotaSpendSurvivesKeyDeletion|TestBuildLateFailureClosesAdminStore'`
  passed (1.876s / 1.152s / 26.484s / 2.381s), with no skipped tests. Inspected
  DB helper skip branches: the only prerequisite skip is absent URL, and connection/
  migration failures are fatal. The separate webui pre-build-stub test intentionally
  skips when embedded assets exist; that is not a database prerequisite skip.

Fixture, worktree and tools:

- `docker ps` and `docker exec aiproxy-review-20260926-145524 pg_isready -U
aiproxy_review -d aiproxy_review` confirm the supplied service remains running
  and accepting connections at `127.0.0.1:39586`. Container `psql` verified all eight
  applied migrations and zero leftover `ledger_*`/`boundary04_*` temporary schemas.
  Tests own connection/schema/row cleanup; disposable shared fixture data/retained
  identities remain until coordinator disposal. No additional database was created.
- Reviewed `git status --short --untracked-files=all`, staged diff (empty) and all
  untracked paths. Removed only the untracked `internal/webui/dist/.gitkeep`
  produced by this integration build; remaining changed/untracked paths match the
  plan's incoming set. Generated ignored UI/binary outputs are expected build
  products. `git diff --exit-code HEAD -- CHANGELOG.md pnpm-lock.yaml
internal/store/migrations` passes for tracked history; new migration 000008 is
  the sole untracked migration. CHANGELOG is unchanged against HEAD.
- Toolchain from `PATH`: `actionlint` 1.7.12, Go 1.27.1, `pnpm` 11.24.0, Node.js 26.8.1, Docker.
- Limitations: no hosted Actions run, real-provider access, live MongoDB deployment,
  exhaustive security/dependency/deployment audit or performance measurement.
  Expiry does not terminate admitted streams; quota caching/overshoot and lack of
  durable retry remain documented. Migration cannot recover previously erased
  spend; review covers concurrent key/request operations, not concurrent migration
  runners. Billing is process-local/current-price, with minute-granularity retention
  and cardinality proportional to distinct dimensions within the window. Browser
  generations remain per-tab and cannot undo server-admitted mutations. Reload
  preparation is not an external transaction and successful activation does not
  drain admitted requests. No commits, CHANGELOG edits or subagents.

## Definition Of Done

All eight tasks completed with evidence after sequential isolated sessions;
coordinator checks the final record and reports exact path/results. Any blocked
verification keeps the relevant task blocked with prerequisites. Dispose only
the test resources created for this plan. `CHANGELOG.md` remains unchanged.

## Coordinator Final Review

- Read the full final task record and independent acceptance audit: all eight
  statuses are `completed`, each with Completion evidence and no unresolved blocker.
- Each task used a distinct sequential sub-agent session:
  01 `ses_f2046e049ffeiaxdg2rXnh8rBL`,
  02 `ses_f203adba9ffegi5bLTQQFOtoZh`,
  03 `ses_f2035723fffeVh4o5Rn0S0J4gI`,
  04 `ses_f202ee2d1ffe5kFHUsW4WA7da3`,
  05 `ses_f20240d90ffeYqIuIUIaane8vz`,
  06 `ses_f201c9f48ffeGvqLyOuJAnqnPC`,
  07 `ses_f2012f043ffeOm3m2MThb1LbCH`,
  08 independent review `ses_f2009f4c6ffei5JiellG7F1twY`.
- Rechecked final worktree, representative auth/probe/CI/typecheck diffs,
  `git diff --check`, and `git diff --exit-code HEAD -- CHANGELOG.md`; passed.
- Stopped the owned disposable PostgreSQL container
  `aiproxy-review-20260926-145524`. A subsequent `docker ps -a` name-filter check
  returned no container, confirming its automatic removal. Removed the temporary
  empty Docker client configuration used to bypass the host credential-helper
  failure when pulling the public PostgreSQL image. No existing database was used.
- Final verification is recorded under BOUNDARY-08; the documented scope and
  runtime limitations remain explicit. All planned work is complete.
