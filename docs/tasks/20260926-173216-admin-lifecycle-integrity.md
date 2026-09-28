# Administrative Lifecycle And Mutation Integrity

Created: 2026-09-26 17:32:16 local time

## Objective And Business Context

aiproxy's optional multi-tenancy lets organizations delegate team administration,
issue/share API keys, and control spending. Onboarding must honor invitation
revocation, offboarding must remove management access, role changes must retain
administrative control, and rejected edits must not become live policy later.
This follow-up closes four evidenced gaps at shared authorization/transaction
boundaries, with focused regression coverage and operator-facing documentation.

Scope: invitation acceptance, organization/team role transitions, offboarding,
and key policy/binding updates. Preserve existing dirty work and **do not edit
`CHANGELOG.md`**. No commits. Public behavior changes belong in relevant docs/help
and this execution record. Avoid broad store/route rewrites.

## Analysis And Deduplication

- Followed AGENTS.md and
  `task-as-you-go skill`.
- Read-only review session `ses_f1fbafa4fffeYVUeA89CPTBkp3` inspected product
  requirements, organization/team/invite/key/quota routes, store models/migrations,
  tests and relevant browser forms. Coordinator inspected the principal source
  locations independently and checked the dirty worktree.
- Existing completed work is recorded in
  `20260926-145524-business-boundary-health-review.md` and
  `20260926-163550-upstream-and-cli-health-review.md`; earlier September residual
  and relevant historical tasks were checked for duplicates. These findings are
  new mutation/offboarding defects, not repetitions of dashboard authority, live
  expiry, durable spend, reload resources, billing, browser generations, CI,
  redirects, conversion, inspection or discovery work.
- No analysis-stage tests run; source-confirmed findings require local regressions
  during implementation. This is a bounded review, not an exhaustive quota,
  provider mutation, dependency, deployment or distributed-consistency audit.
- Prior deferred product/architecture decisions remain in their original records.
  This plan does not change the policy for already-distributed bearer credentials:
  removing account management access is distinct from rotating/revoking a shared
  API key. Preserve historical key/spend attribution and document that distinction.

## Sequential Execution And Verification

Execute LIFE-01 through LIFE-05 in order, each in a **fresh sub-agent session**,
never concurrently. LIFE-05 is an independent reviewer. No nested agents. Shared
hotspots are store transactions, admin routes, tests and documentation. Each owner
sets only their task `in_progress`, implements/tests, then adds `Completion evidence`
and marks `completed` only after acceptance and required checks pass. Record exact
blockers otherwise. Add necessary scoped discoveries before implementing them.

P1 = authorization/credential or persisted-policy integrity; P2 = admin workflow
correctness and maintainability. Use existing naming/errors/abstractions and
apply_patch. Store-level transaction enforcement should serve callers consistently;
handler prechecks alone do not establish concurrent invariants.

All commands run at `<repo-root>`. Each task needing PostgreSQL
creates its **own disposable** PostgreSQL 17 fixture using the documented README
Docker pattern (unique name/database, loopback port, readiness check), supplies
`AIPROXY_TEST_DATABASE_URL`, executes DB packages serially (`GOFLAGS=-p=1` or
`go test -p 1`), and removes only its own fixture afterward. Never use a developer
database or prior session's fixture. Record actual DB execution without prerequisite
skips, failure injection and cleanup. Test fixture creation/deletion must not
clobber another task's state. No real AI providers or production credentials.

Focused verification is specified below. Run `make vet test` after nontrivial
changes, plus `make docs-contract` when docs change. Final gates: `make vet test`,
`make test-race`, `make integration`, `make docs-contract`, frontend tests/typecheck
if changed, and `git diff --check`. Final DB suites use a fresh disposable fixture
with no prerequisite skips. Finish UI builds before Go checks read embedded assets.

Definition of done: five tasks completed with actual evidence, no unmet acceptance
criterion or blocker, integrated checks passing, docs agree, fixtures removed,
existing work preserved, CHANGELOG unchanged, independent final acceptance audit.

### Task LIFE-01: Atomically Consume Invitations And Complete Onboarding

Status: completed

Kind: defect

Priority: P1 — revoked/expired invitations can still create accounts and failed
membership creation leaves partially completed onboarding.

Suggested agent: invitation transaction implementer

Dependencies: none

Primary ownership: `internal/store/crud.go` (`AcceptInvite`),
`internal/httpapi/admin_invites.go`, focused store/HTTP tests, onboarding docs.

Finding: the handler checks invitation validity before bcrypt, then AcceptInvite
updates by ID without an unconsumed/unexpired condition or affected-row check.
It can create an account after the invite was deleted or expired. Organization
membership is written only after the account/invite transaction commits, so a
membership failure consumes the invitation and leaves an unusable partial flow.

References: `internal/httpapi/admin_invites.go` (`acceptInvite`, lines 173–220),
`internal/store/crud.go` (`AcceptInvite`, lines 323–342);
`internal/httpapi/admin_users_invites_test.go` (sequential acceptance tests).

Requirements:

1. At the store boundary, atomically consume an existing, unaccepted, unexpired
   invitation and create its account plus optional organization membership. Require
   exactly one consumed row; derive authority/scope from current stored invitation.
2. Roll back all writes on account or membership failure, returning controlled
   errors. Preserve legitimate system/org invites and default member role semantics.
3. Cover revocation/expiry between handler precheck and consume, stale invite
   snapshots, racing accepts and rollback. Do not rely on bcrypt timing sleeps.

Acceptance criteria:

- Real PG regressions show revoke-before-consume and expiry-before-consume reject
  without accounts/membership; concurrent accepts yield one complete onboarding.
- Injected membership failure leaves no account and an unconsumed invite that can
  be successfully retried; legitimate responses and login flow continue to work.
- Handler no longer performs a separate post-commit membership write; docs explain
  complete-or-no-change acceptance and controlled failure behavior.

Verification: database-backed `go test -race -p 1 ./internal/store ./internal/httpapi`,
default sanity/docs checks and final shared gates.

Execution notes:

- Fresh sequential LIFE-01 implementer read the full shared rules, AGENTS.md and
  requested project skill. Dependencies: none. Incoming extensive dirty work inspected.
- Existing Docker containers inspected before fixture creation; all are unrelated
  and will be preserved. A unique PG17 fixture will be created for this task.
- Scoped LIFE-01 details confirmed before production edits: match both invitation
  ID and token hash when re-reading the current row (a replaced token cannot use
  an old snapshot); publish caller output only after commit; use database wall
  time after acquiring the invitation lock so waiting transactions cannot consume
  an expired invitation. These are part of current-record/rollback requirements.
- Local red regressions executed against PG17 before production edits. Store
  accepted revoked/expired/accepted/replaced-token snapshots, trusted stale admin
  authority, and allowed all 8 competing attempts with different caller emails.
  HTTP query barriers after prechecks deterministically produced 201 after
  revocation/expiry/acceptance; injected membership CHECK failure exposed SQL and
  left both a consumed invitation and account. No bcrypt timing sleeps used.

Completion evidence:

- Changed only LIFE-01 implementation/tests/docs:
  `internal/store/crud.go`, `internal/httpapi/admin_invites.go`, new
  `internal/store/invite_accept_test.go`, new
  `internal/httpapi/admin_invite_atomic_test.go`, the Invitation Onboarding section
  of `website/docs/api-reference.md`, and this task's status/execution record.
  Preserved incoming `GetUserByID`, user/invitation test changes, billing docs, and
  the extensive other dirty/untracked work. No CHANGELOG edits, commits or agents.
- `AcceptInvite` retains its narrow shared store API, locks the current invitation
  by ID **and token hash**, then conditionally updates an unaccepted invitation
  whose expiry is strictly after database `clock_timestamp()`. Exactly one affected
  row is required. The locked stored email/system-admin/org/org-role fields drive
  account and membership creation; only the supplied password hash is carried into
  the new account. Optional membership is inserted in the same transaction, with
  the existing fallback-to-member semantics. Caller outputs publish after commit.
- Handler has no post-commit membership write. Missing/revoked/replaced invitation
  is controlled 404, expired/accepted invitation or email collision is controlled
  400, and storage failures are generic 500 (`could not accept invite`) without SQL
  details. Lookup failures are distinguished from missing records. Documentation
  describes current authority, complete-or-no-change behavior and retry conditions.
- Deterministic regressions:
  - `TestAcceptInviteRejectsStaleEligibility`: deleted, expired, already accepted,
    and token-replaced snapshots reject without account/membership writes.
  - `TestAcceptInviteUsesCurrentAuthority`: changed email, removed system-admin
    authority, changed org and admin/member/default/legacy-invalid org roles; also
    a current global-admin invitation without an org. Stored and returned account/
    invitation fields and exact membership scope are checked.
  - `TestAcceptInviteConcurrentSingleConsumption`: eight start-barrier contenders
    with distinct caller emails yield exactly one invited account and membership;
    all losers return the invitation eligibility error, not uniqueness accidents.
  - `TestAcceptInviteRechecksAfterLockWait`: a separate PG transaction owns the row
    lock; `pg_blocking_pids` proves acceptance is blocked before the owner deletes
    or expires the row and commits. Expiry is set using database wall time **after**
    the waiting transaction began, guarding against transaction-start `now()` use.
  - `TestAcceptInviteRollbackAndRetry`: injected account and membership CHECK
    failures roll back consumption/account/membership and publish no uncommitted
    output; removing each fault permits complete acceptance on retry.
  - HTTP `TestInviteAcceptanceRechecksAfterPrecheck` uses a Bun query hook after the
    email precheck to revoke/expire/accept the already-read invite, then asserts
    exact controlled responses and absent accounts. No bcrypt timing dependence.
  - HTTP current-authority, email-collision and concurrent-request regressions
    inspect current stored users, exact memberships, invitation consumption and
    response bodies. The competing request runs while the first is held after its
    precheck, with one complete winner and a controlled eligibility loser.
  - HTTP injected org-specific membership CHECK failure produces generic 500,
    leaves no account/membership and an unconsumed invite, then succeeds with 201
    and normal login after fault removal. Existing invite/accept/login tests pass.
- Dedicated fixture (README Docker pattern, no reused fixture):
  - Initial `docker ps -a` recorded the existing unrelated containers before creation.
  - Created with `docker run --detach --rm`, name
    `aiproxy-life01-20260926-173416-985503`, container `e77db2a1123c`, image
    `postgres:17` (reported PG 17.11), user `life01_test`, disposable database
    `life01_20260926_173416`, and `-p 127.0.0.1::5432`.
  - `pg_isready -U life01_test` healthcheck (1s interval, 5s timeout, 60 retries);
    `docker inspect` reported **healthy** before any DB tests. Dynamic host port
    was `38584`, bound only to `127.0.0.1`.
  - Every Go verification below explicitly supplied
    `AIPROXY_TEST_DATABASE_URL='postgres://life01_test:life01_fixture_only@127.0.0.1:38584/life01_20260926_173416?sslmode=disable'`. // pragma: allowlist secret
    Builds/test commands ran sequentially; packages used `-p 1` / `GOFLAGS=-p=1`.
    Store regressions additionally used unique disposable schemas and dropped them.
- Actual verification at the repository root:
  1. Before production edits: `go test -p 1 -count=1 -v ./internal/store ./internal/httpapi -run 'TestAcceptInvite|TestInviteAcceptance'`
     **failed as expected**, reproducing the historical defects described above.
  2. Initial fixed focused suite plus `TestInviteAcceptLoginFlow`: **passed**
     (`internal/store` 4.126s, `internal/httpapi` 2.235s).
  3. Final `go test -race -p 1 -count=1 -v ./internal/store ./internal/httpapi`:
     **passed** (`internal/store` 8.860s, `internal/httpapi` 196.148s).
     Full verbose output checked: **zero SKIP and zero FAIL records**; all new
     cases, existing DB-backed cases and race checks actually ran. Session log:
     `<tool-output>/tool_0e04e02d50011UKFtbrYTR7y5H`.
  4. `GOFLAGS=-p=1 make vet test` with the same explicit URL: **passed** across
     the repository. Store, HTTP, app and dbmerge DB suites executed (not cached).
  5. `make docs-contract`: **passed**, documentation contract matrices match.
     `git diff --check`: **passed**. `gofmt -l` on the four touched Go files:
     empty output. Both staged and unstaged CHANGELOG diffs: empty.
- Cleanup: ran `docker stop aiproxy-life01-20260926-173416-985503` after all DB
  verification. Docker's asynchronous `--rm` cleanup completed; a subsequent full
  `docker ps -a` confirmed the fixture absent and all previously listed unrelated
  containers preserved. No developer database or prior fixture was used.
- Acceptance: all LIFE-01 criteria satisfied; blockers: none. Other task statuses
  remain pending. Repository-wide race/integration and independent LIFE-05 audit
  remain the plan's later final gates; this session ran the requested LIFE-01 race,
  default sanity and docs/diff gates. Frontend files were not changed by LIFE-01.

### Task LIFE-02: Preserve Administrative Control Across Every Membership Mutation

Status: completed

Kind: defect

Priority: P2 — duplicate Add member can silently demote the last administrator,
while precheck/write races can also remove all administrative control.

Suggested agent: membership transition and admin UX implementer

Dependencies: LIFE-01

Primary ownership: organization/team membership store boundary,
`internal/httpapi/admin_orgs.go`, membership tests, relevant browser forms/docs.

Finding: org/team POST add-member calls role-changing upserts without PUT's
self-demotion/last-admin protections. The normal form defaults to member, so adding
an existing sole admin can lock out management. PUT/DELETE count admins separately
from the write, leaving competing transitions outside a transactional invariant.

References: `internal/httpapi/admin_orgs.go` (`adminOrgMembers`, `adminOrgTeams`),
`internal/store/crud.go` (`UpsertMembership`, `AddTeamMember`, role/removal methods),
`web-ui/src/pages/orgs-page.tsx` (add-member forms),
`internal/httpapi/admin_team_roles_test.go`, `admin_orgs_test.go`.

Requirements:

1. Make Add member insert-only: duplicate membership returns an actionable conflict
   and never changes a role. Explicit role edits remain on PUT. Cover ID/email forms
   and document/update UI messaging if required by its existing error handling.
2. Enforce existing self-demotion/last-org-admin and last-team-admin contracts at a
   shared transactional transition boundary. Serialize competing role changes and
   removals with a consistent lock order; preserve valid promotions/additions.
3. Inspect all production callers (including invite/bootstrap/OIDC) before adapting
   APIs so legitimate initialization is preserved and no alternate write bypasses
   the established last-admin invariant. Do not make raw SQL test setup a product API.

Acceptance criteria:

- Duplicate POST by email/ID, with omitted or explicit role, cannot change stored
  admin authority; normal additions/promotions and subsequent management succeed.
- Concurrent demotion/removal combinations cannot reduce an established org/team
  admin set to zero. Regressions inspect actual counts and retained access.
- Existing organization self-demotion and team role semantics remain enforced;
  errors guide operators to explicit role editing or assigning another admin.

Verification: database-backed `go test -race -p 1 ./internal/store ./internal/httpapi`,
frontend tests/typecheck if changed, default sanity/docs and final shared gates.

Execution notes:

- Fresh isolated sequential LIFE-02 session; LIFE-01 completion confirmed. Read
  shared rules, AGENTS.md and both loaded/requested task skills. Incoming dirty
  LIFE-01, BOUNDARY/SAFE and other work inspected and preserved.
- Necessary scope corrections before implementation: `DeleteUser` is a production
  cascade writer (`admin_users.go`), so its deletion must also check established
  org/team admin sets under the same locks. This is invariant protection, not
  LIFE-03 offboarding/grant cleanup. Test cleanup may use raw SQL where it must
  intentionally destroy fixtures rather than expose an unsafe product method.
- Replace `UpsertMembership` with insert-only `AddMembership` and explicit
  actor-aware `SetMembershipRole`; adapt org creation/OIDC personal-org callers
  and test setup. Invite acceptance and system bootstrap also participate in
  membership-add serialization; new-user/new-org registration remains legitimate.
- Lock design: transaction-scoped organization row `FOR NO KEY UPDATE` serializes
  all org/team membership additions, role transitions and removals in that org.
  Team additions recheck org membership while holding this lock. User deletion
  first locks its user row, then affected organizations in UUID order; additions
  first take a user `FOR KEY SHARE` lock to prevent a concurrent cascade bypass.
  Future LIFE-03 should reuse the org lock and perform all handoff checks/cleanup
  in that transaction, with no team-first or org-then-user lock inversion.
- UI inspection: both add forms render `errorMessage(err)` in an Alert and have
  explicit row role controls. Actionable server conflicts suffice; no UI edits.

Completion evidence:

- Changed: new `internal/store/membership.go`, new
  `internal/store/membership_test.go`, `internal/store/crud.go`,
  `internal/store/seed.go`, `internal/httpapi/admin_orgs.go`,
  `internal/httpapi/admin_users.go`, `internal/httpapi/admin_orgs_test.go`, new
  `internal/httpapi/admin_membership_integrity_test.go`, the Administrative
  Membership Transactions section in `docs/design.md`, the Organization And Team
  Membership Changes section in `website/docs/api-reference.md`, and this LIFE-02
  record. Preserved incoming LIFE-01/BOUNDARY/SAFE work, including the current
  invitation transaction and `GetUserByID`; no CHANGELOG, frontend, other task
  status, commit or subagent changes.
- POST uses insert-only `AddMembership`/`AddTeamMember`; duplicate rows return
  `ErrMembershipExists` mapped to **409** with guidance to the member's role
  control / explicit PUT. Omitted roles still default to member. Neither a
  duplicate admin-to-member add nor member-to-admin add changes authority.
  Existing explicit PUT and successful additions/promotions remain functional.
- Removed route-level count/write guards. `SetMembershipRole` receives the
  initiating actor ID and checks current self-demotion/last-admin state inside the
  transaction. Team role/removal methods and `DeleteMembership` enforce current
  last-admin counts under the same organization lock. Controlled guard errors are
  **400** with handoff instructions, missing memberships **404**, unexpected store
  failures generic **500**. Team self-demotion with a successor remains allowed;
  org self-demotion remains forbidden even with other org admins.
- Caller audit: org creation and OIDC `ensurePersonalOrg` use `AddMembership`;
  registration and LIFE-01 invitation acceptance use transaction-local
  `insertMembership`; bootstrap `addSystemMembership` ignores only the explicit
  duplicate error and cannot overwrite roles. No role-changing upsert remains.
  `DeleteUser` now guards cascade removals transactionally, including team rows
  surviving a historical org-membership removal. Whole-org/team destruction is
  scope deletion, not a role transition. New/adminless scopes can still initialize.
- Next-agent method/lock contract (also in `docs/design.md`):
  - Membership transitions use READ COMMITTED and `lockMembershipOrg(ctx, tx,
orgID)` (`FOR NO KEY UPDATE`) before reading roles/counts. That one row is the
    mutex for the org and **all its teams**, held until commit/rollback.
  - `protectOrgAdmin` / `protectTeamAdmin` are transaction-local guards; callers
    must already own the containing org lock. Missing membership returns
    `sql.ErrNoRows`. `lockTeamMembershipOrg` discovers the immutable team org,
    acquires its lock and rechecks team existence in that org.
  - Additions first take `lockMembershipUser` (`FOR KEY SHARE`), then the org lock;
    team additions recheck current org membership under that lock. `DeleteUser`
    takes the user row `FOR UPDATE` before discovering affected orgs, locks orgs
    in ascending UUID order, checks every affected admin set, and deletes only
    after all checks succeed. Additions cannot slip into a cascade after discovery.
  - Extend **the existing `DeleteMembership` transaction** for LIFE-03: take its
    org lock, check affected teams using `protectTeamAdmin`, then do all scoped
    cleanup before commit. Do not call transaction-opening public methods inside
    that transaction, take team locks first, or acquire user locks after org locks.
    Multi-org operations must use ascending UUID order. LIFE-02 does not add
    offboarding/team/grant cleanup or change key/quota management authorization.
- Actual PG regressions:
  - `TestMembershipConcurrentTransitions`: org/team × demote+demote,
    demote+remove, remove+remove, delete-user+demote, delete-user+remove and
    delete-user+delete-user (**12 cases**). A separate transaction holds the org
    row; `pg_blocking_pids` confirms **both** contenders are waiting before release.
    Exactly one succeeds and one gets the appropriate last-admin error; fresh
    reads show exactly one admin, its account and its containing-org membership.
  - `TestMembershipHTTPConcurrentTransitionsRetainAccess`: org/team × PUT+PUT,
    PUT+DELETE and DELETE+DELETE (**6 cases**) uses the same real PG lock barrier.
    Exactly one 200 and one actionable 400; persisted count is one. The surviving
    non-system admin can still perform an authorized role edit; the former admin
    cannot regain authority using their existing token (403 or 404).
  - `TestMembershipDuplicatePOSTAndExplicitPUT`: both ID/email forms with omitted,
    explicit member and explicit admin role reject duplicates with 409 while the
    sole admin's stored role/count stay intact. Sole-admin PUT/DELETE reject;
    retained admins add new members by both forms, duplicate promotion rejects,
    explicit PUT promotion succeeds, and org/team self-demotion semantics differ
    as intended. Store role/default/missing-row/bootstrap contracts also pass.
  - `TestTeamAdditionRechecksMembershipAfterLockWait`: a blocked addition sees a
    committed org-membership removal and rejects without adding a team admin.
  - `TestMembershipTransitionRollback`: injected membership CHECK constraint
    rejects a role write, leaves both admins intact, and permits retry after fault
    removal. `TestDeleteUserCannotCascadeLastMembershipAdmin` verifies actionable
    HTTP rejection and preserved account/membership. Existing invitation rollback,
    login, OIDC, registration, org/team management and other DB tests also pass.
- Dedicated disposable fixture, created only after inspecting existing Docker
  containers and README instructions:
  - `docker run --detach --rm --name aiproxy-life02-20260926-174407-316775378`
    with image `postgres:17`, container `394dff1b45aa`, user `life02_test`, database
    `life02_20260926_174407`, and dynamic loopback mapping `-p 127.0.0.1::5432`.
  - Healthcheck `pg_isready -U life02_test -d life02_20260926_174407` (1s interval,
    5s timeout, 60 retries). Docker reported **healthy** before testing; host port
    **38662**, bound only to **127.0.0.1**. Server reported PostgreSQL **17.11**.
  - Every Go check below explicitly used `GOFLAGS=-p=1` and
    `AIPROXY_TEST_DATABASE_URL='postgres://life02_test:life02_fixture_only@127.0.0.1:38662/life02_20260926_174407?sslmode=disable'`. // pragma: allowlist secret
    Test commands and DB packages ran serially. Store tests used unique disposable
    schemas and dropped them. No developer database or previous fixture was used.
- Verification at repository root:
  1. Focused `go test -p 1 -count=1 -v ./internal/store ./internal/httpapi -run
'TestMembership|TestTeamAddition|TestDeleteUserCannot|TestTeamRolesAndManagement|TestAcceptInvite|TestInviteAcceptance|TestOrg'`:
     **passed** (store 7.752s, HTTP 7.745s).
  2. Required `go test -race -p 1 -count=1 -v ./internal/store ./internal/httpapi`:
     **passed** (store **14.981s**, HTTP **217.298s**). Full verbose log checked:
     **zero SKIP, zero FAIL, zero DATA RACE** records. All DB cases executed.
     Log: `<tool-output>/tool_0e058a186001NbOwwOZX3I9ixM`.
  3. `GOFLAGS=-p=1 make vet test` with the explicit fixture URL: **passed** across
     the repository; DB packages executed (app 16.003s, dbmerge 0.169s,
     HTTP 21.380s, store 8.423s; none cached).
  4. `make docs-contract` and `git diff --check`: **passed**. `gofmt -l` on all
     eight touched Go files: empty. Staged/unstaged CHANGELOG diffs: empty.
     Frontend tests/typecheck not needed: no frontend changes; inspected existing
     `errorMessage` string-body handling and both form Alert/role controls.
- Cleanup: `docker stop aiproxy-life02-20260926-174407-316775378` completed;
  subsequent `docker ps -a --format '{{.ID}} {{.Names}}'` confirmed its automatic
  removal and all original unrelated containers still present.
- Acceptance: LIFE-02 criteria satisfied; blockers: none. The later LIFE-03
  offboarding work and shared final LIFE-05 race/integration audit remain in their
  own tasks; their statuses were not edited.

### Task LIFE-03: Remove Scoped Management Authority When Offboarding Members

Status: completed

Kind: defect

Priority: P1 — former organization members retain direct key/quota management.

Suggested agent: offboarding authorization and cleanup implementer

Dependencies: LIFE-02

Primary ownership: `internal/httpapi/admin_keys.go`, `admin_quotas.go`, shared team
authority helpers, `internal/store` membership/grant cleanup, focused lifecycle tests/docs.

Finding: DeleteMembership removes only the organization row; team roles survive.
canManageKey accepts personal ownership or team-admin role without current org
membership, and keyDetailView checks it before org visibility. Team quota handlers
similarly trust surviving team roles. Removed but active users with known IDs can
still view/edit/rotate/revoke/delete keys and read/change team quotas.

References: `internal/store/crud.go` (`DeleteMembership`),
`internal/store/migrations/000004_orgs.sql` (team membership relationship),
`internal/httpapi/admin_keys.go` (`canManageKey`, `keyDetailView`),
`internal/httpapi/admin_quotas.go` (`adminTeamQuota`), shared `isTeamAdmin` helpers.

Requirements:

1. Require current org membership before non-system-admin ownership/team authority
   can authorize scoped management. Cover direct endpoints and shared predicates,
   not only list filtering. Preserve system-admin and current-member workflows.
2. Atomically remove team memberships and direct user key-sharing grants belonging
   to that org on offboarding; keep other org memberships/grants and spend/ownership
   history. Re-add as ordinary member must not restore old team admin/shared grants.
3. Integrate LIFE-02 locks/invariants: refuse offboarding a sole team admin with
   actionable handoff guidance and no partial deletion. Assign another admin before
   retrying; do not silently orphan administrative teams or elevate another user.
4. Document management authority vs previously distributed bearer credentials;
   existing credential revocation remains explicit. Already-admitted operations
   are not claimed to be retroactively canceled.

Acceptance criteria:

- PG-backed personal-owner/team-admin/shared-member matrix after removal covers
  direct key detail/update/rotate/revoke/delete and team quota GET/PUT, with denied
  responses, no credential exposure and no changed records/activation callbacks.
- Cleanup is org-scoped/atomic and survives fresh store reads; re-add has no prior
  team authority/sharing. Unrelated organizations and historical spend are preserved.
- Sole-admin offboarding refusal and handoff-success paths are covered, including
  races with team admin transitions/additions at the shared store boundary.

Verification: database-backed `go test -race -p 1 ./internal/store ./internal/httpapi`,
default sanity/docs and final shared gates.

Execution notes:

- Fresh isolated sequential LIFE-03 session; LIFE-01/02 completed. Read complete
  shared rules, LIFE-02 method/lock contract, relevant design, AGENTS.md and both
  task skills. Incoming dirty work inspected and preserved.
- Scoped additions recorded before implementation: test legacy surviving team/share
  rows independently from transactional cleanup, including the team quota GET
  fallback (which reads team membership directly). Gate `isTeamAdmin`, key owner
  authority and quota visibility on current org membership, failing closed.
- Extend only the existing DeleteMembership transaction under its org lock: check
  every affected team's handoff before any deletion, delete scoped team rows and
  direct user shares, then org membership. Use transaction-local helpers, no added
  user locks or nested public transactions. Inject a late deletion failure to prove
  complete rollback, and use real PG lock barriers for additions/admin transitions.
- Preserve ownership and bearer credentials explicitly. Concurrent key-binding
  replacement validation/serialization belongs to LIFE-04's already specified
  boundary; this task tests cleanup of existing grants and membership races.
- Docker containers inspected; create an owned unique PG17 fixture per README,
  never using the existing unrelated containers or previous fixtures.
- Pre-fix PG reproduction: `go test -p 1 -count=1 -v ./internal/store
./internal/httpapi -run TestOffboarding` failed as expected. Current owner/team
  authority let removed users read/update/rotate/revoke/delete keys (four activation
  callbacks and returned rotation token); surviving team members read quota and
  former team admins changed it. Cleanup/re-add retained rows; sole-team-admin
  deletion returned success; competing transitions left orphaned team authority.
  Log: `<tool-output>/tool_0e05e766f001g696HTF0dK3xhu`.
- Initial fixed focused suite passed (store 4.203s, HTTP 2.533s). Added a quota
  cache-preservation assertion before final gates to verify denied quota PUT does
  not invalidate serving state, in addition to unchanged DB rows/reload counters.

Completion evidence:

- Changed: `internal/store/membership.go`, new
  `internal/store/offboarding_test.go`, `internal/httpapi/admin_keys.go`,
  `internal/httpapi/admin_orgs.go`, `internal/httpapi/admin_quotas.go`, new
  `internal/httpapi/admin_offboarding_test.go`, `docs/design.md`,
  `website/docs/api-reference.md`, and only LIFE-03's status/execution record here.
  Incoming LIFE-01/02 and all other dirty/untracked work preserved. No CHANGELOG,
  commits, subagents, frontend changes or other task status edits.
- `canAccessOrg` fails closed for non-system callers without current membership;
  `canManageKey`, `isTeamAdmin`, `keyDetailView` and team quota admission use it.
  Owner identity and historical team roles cannot bypass current org membership.
  Detail visibility checks precede usage/quota/owner view building. Team quota's
  ordinary-member GET fallback cannot bypass the gate. System-admin and current
  member workflows retain their existing authority.
- Extended the existing READ COMMITTED `DeleteMembership` transaction, retaining
  LIFE-02's containing-org `FOR NO KEY UPDATE` lock. It checks org admin protection
  and every affected team's `protectTeamAdmin` guard before deleting any row, then
  removes scoped team memberships, scoped direct user key grants, and org membership
  in that transaction. No public transaction-opening helper is called within it;
  no user/team locks were added, so user-before-org/UUID-order contracts stand.
  Sole-team-admin errors retain the existing actionable HTTP 400 handoff message.
- `TestOffboardingDirectAuthorityMatrix`: **42 direct negative endpoint cases**
  (normal transactional removal and legacy surviving rows × personal owner, team
  admin, shared member × detail/update/rotate/revoke/delete/team quota GET/PUT).
  Exact 404/403 responses, absent metadata/credential/quota disclosure, identical
  persisted key fields and user/team bindings, identical quota rows, zero reload
  callbacks and unchanged serving quota cache are asserted after each request.
  The existing active user's pre-removal JWT is reused. Shared predicates are
  tested directly so route filtering/cleanup cannot mask a broken authority gate.
  Positive current-member reads, owner/admin edits, successor quota edits, system
  detail and explicit system revoke are exercised. Ordinary re-add has no team or
  direct share, no team quota access and no team-owned key visibility. Preserved
  personal ownership is explicitly usable again upon legitimate org re-add.
- `TestOffboardingScopedCleanupAndRollback`: normal removal, sole-admin refusal,
  and an injected **late organization-membership DELETE trigger failure**. Fresh
  queries verify both team memberships and both direct shares survive rejection;
  removing the fault or promoting a successor allows a clean retry. Successful
  cleanup and ordinary re-add retain no prior team/share grants. Another org's
  membership/admin role, team admin and direct share remain; all key fields,
  ownership, credentials/enabled state, team-wide shares, spend ledger entry and
  user quota budget/spend offset remain unchanged.
- `TestOffboardingConcurrentTeamTransitions`: **12 real PG barrier cases**, with
  each operation queued first in turn: offboarding versus another admin's demotion,
  team removal, org offboarding, the departing member's promotion, ordinary team
  addition and admin addition. A separate transaction holds the org row and
  `pg_blocking_pids` proves both contenders wait before release. Competing admin
  losses yield exactly one success and one last-team-admin error with one admin
  left; additions/promotions cannot leave a team grant without org membership.
  `TestOffboardingHTTPRequiresTeamHandoff` verifies controlled refusal, preserved
  org membership and successful cleanup after explicitly adding a successor.
- Documentation describes atomic cleanup, handoff/retry, direct endpoint denials,
  preservation/re-add, and the separate bearer-token policy. No automatic key
  revocation/rotation or retroactive cancellation of already-admitted operations
  is introduced. LIFE-04 retains its planned concurrent key-binding validation and
  atomic update work; that boundary must contend on this same org lock and recheck
  user membership before creating/replacing grants.
- Owned disposable fixture (README Docker pattern, after inspecting all containers):
  `aiproxy-life03-20260926-1820-b7d2`, container `e4b48dab6696`, image `postgres:17`,
  reported PostgreSQL **17.11**, user `life03_test`, DB `life03_20260926_b7d2`.
  Used `--detach --rm -p 127.0.0.1::5432`, dynamic host port **38450**;
  `pg_isready -U life03_test -d life03_20260926_b7d2` healthcheck (1s interval,
  5s timeout, 60 retries), Docker **healthy** and SQL version probe before tests.
  Every Go command explicitly supplied `GOFLAGS=-p=1` and
  `AIPROXY_TEST_DATABASE_URL='postgres://life03_test:life03_fixture_only@127.0.0.1:38450/life03_20260926_b7d2?sslmode=disable'`. // pragma: allowlist secret
  Commands/DB packages ran serially; store test schemas were disposable and dropped.
- Verification at repository root:
  1. Pre-fix `go test -p 1 -count=1 -v ./internal/store ./internal/httpapi -run
TestOffboarding`: **failed as expected**, with meaningful red evidence above.
  2. Fixed focused `go test -p 1 -count=1 ./internal/store ./internal/httpapi -run
TestOffboarding`: **passed**, store 4.203s, HTTP 2.533s.
  3. Required final `go test -race -p 1 -count=1 -v ./internal/store ./internal/httpapi`:
     **passed**, store **19.093s**, HTTP **202.892s**. Full verbose log searched:
     **zero SKIP, zero FAIL, zero DATA RACE**. All DB prerequisites satisfied and
     new/existing DB regressions actually executed. Log:
     `<tool-output>/tool_0e061217e001a5wi6ZCWxgqdS8`.
  4. `GOFLAGS=-p=1 make vet test` with the same explicit URL: **passed** across
     the repository, including uncached app 15.695s, dbmerge 0.155s, HTTP 30.310s
     and store 17.095s. Existing membership/invitation/key/quota regressions pass.
  5. `make docs-contract`, `git diff --check`: **passed**. `gofmt -l` on all six
     touched Go files: empty. Staged/unstaged CHANGELOG diffs: empty.
- Cleanup: `docker stop aiproxy-life03-20260926-1820-b7d2` succeeded after all DB
  checks. Subsequent full `docker ps -a --format '{{.ID}} {{.Names}}'` confirmed
  automatic removal and all original unrelated containers preserved. No developer
  database or prior fixture used.
- Acceptance: LIFE-03 complete, all specified criteria pass; blockers: none.
  Later LIFE-04 and independent LIFE-05 shared final integration/race audit remain
  assigned to their existing tasks. This session ran all requested LIFE-03 gates.

### Task LIFE-04: Commit Key Policy And Sharing Changes As One Validated Update

Status: completed

Kind: defect

Priority: P1 — rejected key edits persist and activate on a later unrelated reload.

Suggested agent: atomic credential-policy mutation implementer

Dependencies: LIFE-03

Primary ownership: `internal/httpapi/admin_keys.go` PUT handling, store key/binding
transaction methods, DB/HTTP/runtime regression tests and admin API docs.

Finding: PUT saves metadata/expiry/tenant/allowed_models before validating user/team
bindings, and bindings use a separate transaction. Invalid binding input returns
an error with policy edits already persisted; reload is skipped then but a later
SIGHUP or valid mutation activates the supposedly rejected edits.

References: `internal/httpapi/admin_keys.go` (PUT lines 235–295,
`resolveKeyBindings`), `internal/store/crud.go` (`UpdateInboundKey`), binding store
methods (`SetKeyBindings`), `internal/app/app.go` (`Reload`, database catalog merge).

Requirements:

1. Validate the complete request before writes, then persist fields and optional
   sharing replacement atomically at the store boundary. Preserve existing omitted
   vs empty list semantics and ownership/credential identity.
2. Validation/storage failures preserve fields and both binding sets and invoke no
   activation. Protect against the concurrent offboarding boundary from LIFE-03
   where necessary for membership/grant validity; use consistent lock order.
3. Preserve the documented saved-but-activation-failed contract for valid commits
   whose subsequent runtime reload fails; distinguish it from rejected edits.

Acceptance criteria:

- Mixed valid policy edits plus invalid UUID/cross-org/nonmember bindings leave DB
  fields and bindings unchanged, including after an explicit later runtime reload.
- Injected binding-write failure rolls back fields/bindings; valid combined edits
  commit together and invoke activation once. Tests verify live authentication/
  allowed-model/expiry effects and unchanged state on rejection, not just status.
- Existing key creation, sharing, expiry, rotation/revocation and preserved-spend
  tests pass; documentation accurately distinguishes mutation and activation failure.

Verification: database-backed `go test -race -p 1 ./internal/store ./internal/httpapi ./internal/dbmerge ./internal/app`,
default sanity/docs and final shared gates.

Execution notes:

- Fresh sequential LIFE-04 session; LIFE-01..03 completion and shared lock/offboarding
  contracts read, along with AGENTS.md and both task skills. Incoming dirty work
  inspected and preserved. Only LIFE-04 status is being changed.
- Necessary scoped details before implementation: use a policy patch rather than a
  stale whole-key save, preserving omitted fields and concurrent rotation/revocation.
  Recheck actor's current org/owner/team authority as well as proposed grants after
  the org lock. Binding inserts acquire sorted user KEY SHARE locks before the org
  lock (including actor), preventing inversion with DeleteUser's cascade contract.
  Reuse transaction-local binding replacement for standalone SetKeyBindings too,
  so its creation caller cannot recreate a grant after offboarding; creation's
  broader mutation/activation flow is outside this task. Either binding list supplied
  replaces both sets; neither supplied preserves both, matching current behavior.
  Activation follows a successful commit before response-view reads, so view building
  cannot silently skip activation of a committed edit.
- Resumed after the prior tool session failure: inspected current task status,
  all dirty/untracked paths and the owned healthy PG17 container
  `aiproxy-life04-20260926-1903-c91e` (`f077eb6e43ac`, loopback port `38474`).
  The notes and fixture survived; no LIFE-04 implementation/test files had been
  saved. Continued this task's owned fixture as explicitly requested instead of
  resetting it or creating a duplicate; other task work and CHANGELOG preserved.
- Additional scoped concurrency detail: rotation/revocation now update only their
  credential/enabled columns, so an older route snapshot cannot overwrite a policy
  commit in the reverse ordering. Proposed team KEY SHARE locks precede the key
  row lock to match team deletion's cascade order. These close the existing
  preservation and consistent-lock-order requirements, not a new task.
- Before production changes, `TestKeyPolicyAtomicHTTP` failed against the owned
  fixture: a malformed user UUID returned 400 but persisted description, tenant,
  allowed_models and expiry. Focused store/HTTP/runtime tests now cover rollback,
  current authority/grants, credential concurrency, and explicit later reload.

Completion evidence:

- Changed: `internal/httpapi/admin_keys.go`, the key methods in
  `internal/store/crud.go`, new `internal/store/key_policy.go`, new
  `internal/store/key_policy_test.go`, new
  `internal/httpapi/admin_key_atomic_test.go`, new
  `internal/app/key_policy_test.go`, the atomic-key section of `docs/design.md`,
  the key-policy/sharing section of `website/docs/api-reference.md`, and this
  task's execution record/status. Preserved incoming changes in shared files,
  all other task records/statuses, frontend changes and CHANGELOG. No commits,
  resets, duplicate fixture or nested agents.
- PUT validates the complete request before persistence. Its shared store patch
  transaction rechecks current membership, team scope and manager authority,
  writes only supplied policy columns, and replaces both sharing sets atomically
  when requested. Omitted/null versus empty semantics, expiry clearing, ownership,
  name/org/credential identity and enabled state are preserved. Standalone binding
  replacement uses the same validation/locking boundary. Rotation/revocation use
  narrow writes so neither ordering overwrites unrelated policy or credentials.
- Sorted user KEY SHARE locks precede the organization NO KEY UPDATE lock;
  proposed team KEY SHARE locks precede the current key lock. This cooperates with
  LIFE-03 offboarding and user-deletion order and avoids team cascade inversion.
  Missing rows and lost authority/grants return controlled errors; unexpected
  update storage failure is generic `500 could not update key`, with no activation.
  Valid commits activate once before response-view queries. Failed activation
  retains the complete saved edit and existing saved-but-activation-failed response.
- Regression evidence (six new top-level tests):
  - `TestKeyPolicyRollbackAndReplacement`: real PostgreSQL CHECK failures on
    **both** user and team binding inserts roll back all policy columns and both
    old sharing sets, including timestamps. Removing each fault permits the full
    combined commit. Nonmember/cross-org store calls reject; omitted fields and
    sharing remain intact and explicit expiry clearing succeeds.
  - `TestKeyPolicyRechecksAfterOrgLock`: a held org lock and `pg_blocking_pids`
    barrier prove the update waits before recipient offboarding, actor offboarding,
    team-admin demotion, rotation, revocation, key deletion or another policy edit.
    The resumed update rejects lost authority/grants without writes, preserves
    current credentials/enabled state and omitted policy, and never resurrects a
    deleted key. Later credential writes also preserve the committed policy.
  - `TestKeyBindingsSerializeWithOffboarding`: actual competing DeleteMembership
    and policy/standalone-binding transactions leave no grant to the removed member.
    `TestKeyPolicyUserDeletionLockOrder` deterministically exercises both
    user-delete-first and policy-first lock chains; both complete without deadlock
    and retain no deleted-user grant.
  - `TestKeyPolicyAtomicHTTP`: mixed policy plus invalid user/team UUIDs,
    nonmember and cross-org bindings preserve exact DB fields/shares and invoke
    zero activations. Injected team-binding failure returns generic 500 with the
    same rollback/no-activation guarantee. A valid combined edit activates exactly
    once and clears both sets when one supplied list is empty. A failed activation
    is counted once and leaves the full valid edit saved.
  - `TestKeyPolicyRuntimeAtomicity`: builds a real App against an isolated PG
    schema and local mock upstream. Invalid UUID/nonmember/cross-org updates and
    an injected binding insert failure preserve fields/shares and live model
    policy before **and after an explicit App.Reload**. A valid combined edit
    changes the allowed-model 200/403 results immediately; setting a past expiry
    produces 401 and clearing expiry restores authentication. Empty models clears
    restrictions. Invalid config forces post-commit activation failure, retaining
    old live policy and saved new policy; repairing config and reloading activates
    that saved edit.
- Fixture continuity and actual execution:
  - On resume, inspected Docker's full container list, identity, environment,
    auto-remove flag, health and binding. Continued only this interrupted task's
    owned `aiproxy-life04-20260926-1903-c91e` (`f077eb6e43ac`, `postgres:17`,
    PG 17.11), disposable database `life04_20260926_c91e`, user `life04_test`,
    loopback port `38474`, already healthy. No developer database was used.
  - Every database test command explicitly supplied
    `AIPROXY_TEST_DATABASE_URL='postgres://life04_test:life04_fixture_only@127.0.0.1:38474/life04_20260926_c91e?sslmode=disable'`. // pragma: allowlist secret
    Packages ran serially with `-p 1` / `GOFLAGS=-p=1`; test-owned isolated schemas
    were dropped. No UI build or other shared-output build ran concurrently.
  - Before production edits:
    `go test -p 1 -count=1 ./internal/httpapi -run '^TestKeyPolicyAtomicHTTP$'`
    **failed as expected**, demonstrating persisted rejected fields.
  - Focused store and HTTP regressions passed. Initial runtime fixture setup
    correctly failed Build's pending-migration gate; adding fixture MigrateUp
    before Build resolved setup, and the real runtime regression passed.
  - Final `go test -race -p 1 -count=1 -v ./internal/store ./internal/httpapi ./internal/dbmerge ./internal/app`
    **passed**: store 23.080s, HTTP 196.610s, dbmerge 1.300s, app 21.902s.
    Full verbose log checked: **zero SKIP, zero FAIL, zero data-race reports**.
    Existing creation/sharing, expiry, rotation/revocation, preserved-spend and
    LIFE-01..03 suites actually executed. Log:
    `<tool-output>/tool_0e0899054001vDIfjpyhfYVi7P`.
  - `GOFLAGS=-p=1 make vet test` with the same explicit fixture URL: **passed**
    repository-wide; all four DB packages executed uncached (other unchanged
    packages could use Go's cache).
  - `make docs-contract`: **passed**. `git diff --check`: **passed**.
    `gofmt -l` on all six touched/new Go files: empty. Staged and unstaged
    `CHANGELOG.md` diffs: empty.
- Cleanup: after verification, ran
  `docker stop aiproxy-life04-20260926-1903-c91e`; its `--rm` cleanup completed.
  A subsequent full `docker ps -a` confirmed the owned fixture absent and every
  previously observed unrelated container still present.
- Acceptance: all LIFE-04 criteria satisfied; blockers: none. LIFE-05 remains
  pending for its independent audit and shared final gates (repository-wide race,
  integration and applicable frontend checks); this session completed the full
  LIFE-04 four-package race, repository sanity and docs/diff gates.

### Task LIFE-05: Independently Audit Lifecycle Integration And Completion

Status: completed

Kind: improvement

Priority: P1 — coupled membership, invitation and key transactions need a combined audit.

Suggested agent: independent integration reviewer, not a preceding implementer

Dependencies: LIFE-01, LIFE-02, LIFE-03, LIFE-04

Primary ownership: review all plan changes and this task file; narrowly scoped
corrections only where required for established acceptance.

Finding: independently passing changes must jointly preserve offboarding, authority,
transaction rollback and activation ordering under alternate routes/concurrency.

References: prior tasks, actual changed source/tests/docs and Completion evidence.

Requirements:

1. Audit every acceptance criterion against source/regression evidence, including
   alternate mutation callers, transaction/lock ordering, current authority,
   invitation row lifetime, offboarding re-add, sharing cleanup and reload behavior.
2. Add explicit scoped requirements and regressions for any correction needed.
   Run all shared final gates with a new disposable PG fixture, with DB tests actually
   executed and cleanup confirmed. Preserve completed baseline work and CHANGELOG.
3. Record a per-task acceptance/status audit and honest verification/analysis
   limitations. Do not mark unresolved requirements completed.
4. Scoped final-review correction: invitation acceptance must not invert the parent
   organization deletion cascade (org row then invite row). Current AcceptInvite
   locks the invite, inserts the user, then waits for the org mutex; production
   DeleteOrganization can hold the org while waiting for that same invite. Protect
   the invitation's current parent lifetime before locking the invite, recheck any
   intervening scope change, and retain expiry/current-authority/rollback semantics.
   Add a deterministic query/PG-lock-barrier regression using the actual deletion
   caller, proving both operations finish without deadlock or partial onboarding.

Acceptance criteria:

- All tasks completed with actual Completion evidence, passing final gates and no
  unmet criterion; docs and implementation agree. Each fixture has been removed.
- Existing work and CHANGELOG are preserved; final review documents residual
  limitations without presenting skipped checks as successful verification.

Verification: shared final gates and independent source/acceptance review.

Execution notes:

- Fresh independent LIFE-05 reviewer; implemented none of LIFE-01..04. Read the
  entire task/evidence, AGENTS.md and requested project task-as-you-go skill.
  Dependencies report completed. Auditing the resulting LIFE-04 source and callers
  after failed session `ses_f1f9a2b20ffe9HsXom7mrT37jS` and fresh recovery session
  `ses_f1f7e4b6effeaGKzgBXtEuQv6W`, rather than relying on their reports alone.
- Inspected incoming extensive dirty/untracked BOUNDARY/SAFE/LIFE baseline and all
  Docker containers. No earlier LIFE fixture remains. A new reviewer-owned PG17
  fixture will be used; frontend build/checks will finish before Go asset readers.

Completion evidence:

- Independent source audit completed against the resulting implementation, not
  just the LIFE-01..04 reports. Read the complete scoped store modules, invitation,
  membership/offboarding and key-policy regressions; inspected production handlers,
  OIDC callback/personal-org provisioning, registration, bootstrap, user deletion,
  organization/team deletion, quota gates, JWT stored-user resolution, activation,
  App.Reload and dbmerge key loading, relevant migrations, API/design documentation
  and browser membership error/role controls. The LIFE-04 recovery's patch and
  credential-only writes are present and exercised in the combined final run.
- One scoped acceptance correction, recorded in requirement 4 **before** editing:
  `AcceptInvite` versus `DeleteOrganization` had opposite invite/parent lock order.
  New `TestAcceptInviteOrganizationDeletionLockOrder` holds acceptance after its
  invitation lock with a Bun hook, starts the real store deletion, and uses
  `pg_blocking_pids` to prove it is waiting before releasing acceptance. Pre-fix
  execution failed with PostgreSQL **40P01 deadlock detected** on organization
  deletion (1.390s). The corrected implementation protects the current parent with
  KEY SHARE before locking the invite; this is compatible with the later org
  membership NO KEY UPDATE mutex and existing-user-before-mutex ordering.
  Both real operations now commit without deadlock: onboarding commits its user,
  and subsequent whole-scope deletion removes the org/invite/membership as intended.
- Added `TestAcceptInviteScopeChangeDuringLockAcquisition`: deterministic changes
  between scope discovery and invitation lock cover org-to-org, org-to-global and
  global-to-org. Each returns `ErrInviteChanged` without account, membership,
  consumption or caller-output writes; retry succeeds in the current scope.
  Handler maps this to actionable **400**. Existing current stored email/admin/role,
  token identity, expiry-after-lock, single-consumption and rollback behavior passes.
  Updated design/API docs describe the parent lifetime lock and scope-change retry.
- Reviewer edits are limited to `internal/store/crud.go`,
  `internal/store/invite_accept_test.go`, `internal/httpapi/admin_invites.go`,
  `docs/design.md`, `website/docs/api-reference.md`, and this task record. All
  incoming BOUNDARY/SAFE/LIFE work remains. UI assets were rebuilt by official
  commands; removed only the newly generated untracked `internal/webui/dist/.gitkeep`
  from `make integration`. Final asset status matches the incoming ignored-output
  state. No subagents, commits or CHANGELOG edits.

Per-task acceptance/status audit:

| Task    | Final status | Independent acceptance result                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| LIFE-01 | completed    | PASS: ID+token/current-authority re-read, strict database wall-clock expiry after row lock, exactly one consumption, account+membership atomicity, generic storage failures, retry/login and concurrent accept regressions. Handler has no post-commit membership write. Parent deletion inversion corrected above.                                                                                                                                                                                                                        |
| LIFE-02 | completed    | PASS: email/ID duplicate adds with omitted/member/admin roles return actionable 409 and never change authority; explicit promotions/additions work. Org/team demote/remove/user-delete combinations retain one admin with management access. Org self-demotion remains forbidden; team self-demotion with successor works. Store guards run under the common org mutex.                                                                                                                                                                    |
| LIFE-03 | completed    | PASS: 42 direct negative cases cover normal removal and legacy surviving rows for owners/team admins/shared members, with no disclosure, row mutation, reload callback or quota-cache invalidation. Cleanup/handoff/retry is transactional and org-scoped; ordinary re-add restores no team/direct grant, other orgs and spend/ownership remain. Twelve barrier cases cover competing team transitions/additions/offboarding.                                                                                                              |
| LIFE-04 | completed    | PASS: invalid UUID/nonmember/cross-org mixed edits and injected user/team binding writes preserve exact policy/shares. Omission/null/empty semantics and credential/owner identity remain. Current manager/grants are rechecked under the shared boundary; concurrent offboarding/deletion cannot recreate a grant. Narrow rotate/revoke writes preserve policy in both orderings. Real App tests verify live model/expiry behavior and unchanged rejected policy after explicit reload, plus saved-valid-edit/failed-activation recovery. |
| LIFE-05 | completed    | PASS: independent production-caller/lock/docs audit, necessary correction with red-to-green deterministic regressions, full final gates and actual four-package PG execution, owned fixture cleanup and dirty baseline/CHANGELOG preservation confirmed.                                                                                                                                                                                                                                                                                   |

Caller/lock and scope findings:

- Production membership writes route through `AddMembership`, `insertMembership`,
  explicit role transitions or guarded removals. No role-changing upsert remains.
  OIDC `ensurePersonalOrg` only creates a new personal org when membership is absent;
  it never modifies an existing org role. Registration creates new user/org rows in
  one transaction. Bootstrap ignores only duplicate membership and cannot demote or
  promote an existing member. User-deletion cascades lock the user, discover both org
  and legacy team memberships, then lock orgs in UUID order before all admin guards.
- Membership additions take user KEY SHARE before the org mutex; offboarding has no
  org-to-user lock inversion. Key policy/binding updates take sorted recipient/actor
  user locks, org mutex, proposed team KEY SHARE locks, then current key lock.
  Inspected team-delete parent/cascade path and its production owned-key refusal;
  proposed team locks precede key writes. The new invitation parent-lifetime lock is
  compatible with membership mutexes; its created user is not externally visible.
- Production key PUT uses the patch transaction; the old broad `UpdateInboundKey`
  has no production caller. Rotation/revoke call credential/enabled-only methods.
  Standalone `SetKeyBindings` (creation caller) shares validation and offboarding
  serialization. Creation's broader multi-step persistence flow remains explicitly
  outside the PUT contract. Direct key/quota checks consult current org membership;
  `adminClaims` refreshes active account/system authority from storage. Previously
  admitted non-PUT operations are not promised retroactive cancellation, and bearer
  credential revocation remains explicit, matching the documented scope.
- `activateChange` is reached once after a successful PUT commit and before view
  queries. Rejections never invoke it. Reload serializes via `reloadMu`, loads saved
  database key policy, and retains the previous runtime on candidate failure; a later
  successful reload can activate a valid saved edit, never a rolled-back rejection.

New disposable fixture and exact verification:

- After inspecting Docker and README, created only
  `aiproxy-life05-20260926-185240-a8f3`, container `c86d83236e16`, using:
  `docker run --detach --rm --name aiproxy-life05-20260926-185240-a8f3 -e POSTGRES_USER=life05_test -e POSTGRES_PASSWORD=life05_fixture_only -e POSTGRES_DB=life05_20260926_a8f3 -p 127.0.0.1::5432 --health-cmd 'pg_isready -U life05_test -d life05_20260926_a8f3' --health-interval 1s --health-timeout 5s --health-retries 60 postgres:17`.
  Docker reported **healthy**, binding **127.0.0.1:39450**, and SQL version probe
  reported **PostgreSQL 17.11** before tests. No developer/prior fixture used.
- All Go commands below used the explicit environment
  `AIPROXY_TEST_DATABASE_URL='postgres://life05_test:life05_fixture_only@127.0.0.1:39450/life05_20260926_a8f3?sslmode=disable'` // pragma: allowlist secret
  and `GOFLAGS=-p=1` (race gate additionally added `-v -count=1`). Commands and DB
  packages were serialized. Working directory: `<repo-root>`.
  1. `pnpm --filter @aiproxy/web-ui test && pnpm --filter @aiproxy/web-ui build`:
     **PASS**, 9 files / **136 tests**, `tsc -b --force` typecheck and Vite build.
     Completed before any Go gate read embedded assets.
  2. `go test -count=1 -v ./internal/store -run '^TestAcceptInviteOrganizationDeletionLockOrder$'`:
     **expected pre-fix FAIL**, actual PG deadlock above. After correction,
     `go test -count=1 -v ./internal/store -run '^TestAcceptInvite'`: **PASS**
     (7.216s). After scope-change regression/error mapping was added,
     `go test -count=1 ./internal/store ./internal/httpapi -run 'TestAcceptInvite|TestInviteAcceptance'`:
     **PASS**, store **6.898s**, HTTP **3.624s**.
  3. `make vet test`: **PASS** on completed rerun. First invocation exceeded the
     tool's 120s limit after passing app **18.803s**, dbmerge **0.248s**, HTTP
     **37.910s**; it was not counted as a passing gate. Confirmed no Go test process
     remained, then reran with a 600s tool timeout. All packages passed, store
     **25.517s**; completed earlier packages used cache. The next gate forced every
     package to run uncached, including all database suites.
  4. `GOFLAGS='-p=1 -v -count=1' make test-race` with the explicit fixture URL:
     **PASS**, repository-wide uncached race run. DB-owning packages actually ran:
     **app 26.463s, dbmerge 1.263s, HTTP 226.406s, store 40.125s**. Full verbose
     output checked: **zero FAIL, zero DATA RACE, zero DB prerequisite skips**.
     Existing OIDC full code flow, registration, invite/login, key sharing,
     rotation/revocation/expiry, quota/preserved-spend and all LIFE tests passed.
     Log: `<tool-output>/tool_0e097aaed001xdly3Hld3APNAw`.
     The sole repository-wide skip is `internal/webui.TestNotBuiltWithoutIndex`,
     intentionally applicable only before UI assets are built; built-asset tests ran.
  5. `make integration`: **PASS**, rebuilt/typechecked UI first, built the real
     binary, then hermetic integration suite **1.721s**. No overlapping asset readers.
  6. `make docs-contract`: **PASS**. `git diff --check`: **PASS**.
     `gofmt -l` on the three changed Go files: empty. Both staged and unstaged
     `CHANGELOG.md` diffs: empty. Final task-record/docs edits receive the same
     docs-contract/diff checks after this evidence is recorded.
- Cleanup: `docker stop aiproxy-life05-20260926-185240-a8f3` succeeded. The first
  immediate listing briefly showed Docker's asynchronous `--rm` state as Dead;
  the subsequent full `docker ps -a` confirmed the fixture **absent** and all
  originally observed unrelated containers present. LIFE-01..04 fixture absence
  was independently confirmed in the initial listing; their own cleanup commands
  remain documented in their historical evidence.

Limitations and final disposition:

- This is the bounded LIFE acceptance review plus combined baseline test gates,
  not an exhaustive audit of every provider/quota mutation, arbitrary raw SQL,
  all possible PostgreSQL schedules, distributed activation or production load.
  Browser checks use the repository's Vitest/typecheck/build workflow, not a new
  manual browser session. OIDC and inference use hermetic/local fixtures; real
  providers, external IdP deployments and production credentials were not used.
- No required gate or acceptance criterion remains unmet. Expected pre-build-only
  UI skip and the superseded tool timeout are explicitly distinguished from passes.
  Final counts: **5 completed, 0 pending, 0 in_progress, 0 blocked**. CHANGELOG and
  incoming dirty baseline preserved; no commits or subagents.

## Coordinator Final Closure

- Read the entire completed task record after the independent reviewer returned,
  including all scoped additions, acceptance evidence, final gate results and
  fixture cleanup. Confirmed five completed statuses with Completion evidence and
  no unresolved execution blockers. Rechecked worktree, `git diff --check` and
  `git diff --exit-code HEAD -- CHANGELOG.md`; both checks passed.
- Task sessions ran sequentially, with no overlap:
  LIFE-01 `ses_f1fb7b8feffecWpN31VsCe0ddF`;
  LIFE-02 `ses_f1faebd66ffeLWejqVU4SEcCuI`;
  LIFE-03 `ses_f1fa3ab5fffeDnDtT9j0rV6HQb`;
  LIFE-04 interrupted `ses_f1f9a2b20ffe9HsXom7mrT37jS`, then recovered by fresh
  session `ses_f1f7e4b6effeaGKzgBXtEuQv6W` using the task's already-owned fixture;
  independent LIFE-05 `ses_f1f700a9bffeVnKHpkgn6MAn2I`.
- All task outcomes and verification are recorded above; real-provider and
  exhaustive-product audits are not claimed.

Follow-up review: `20260926-191434-admission-and-catalog-workflows.md` tracks
quota-read failure handling, catalog publication and browser catalog-edit fidelity.
