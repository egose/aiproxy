# Copilot UI Device Authorization And Encrypted Database Credentials

Created: 2026-09-27 10:55:47 local time

## Objective And Confirmed Current Behavior

Add Connect GitHub to the web provider creation/edit form. The application server
obtains the device challenge, performs interval-controlled polling and stores the
result as an encrypted structured credential in PostgreSQL. The user sees only
the authorization URL/code and status. Saving the provider explicitly activates
the authorized credential through the existing catalog reload path.

This flow does not exist currently: `providers-page.tsx` asks for a name from
`aiproxy login`; `DBProvider` holds only `CopilotCredentialPath/Name`; `BuildProvider`
uses that filesystem sidecar. Existing generic API-key encryption is not Copilot
credential storage. App-user OIDC login is unrelated to upstream device authorization.

Preserve the CLI/sidecar alternative, static configuration, prior completed work
and all incoming dirty changes. **Do not edit CHANGELOG.md or commit.** No actual
GitHub login/account is required for verification. Live compatibility remains
deferred in `20260927-102347-copilot-live-compatibility.md` (COPILOT-LIVE-01).

## Analysis And Design Decision

Followed AGENTS.md and `task-as-you-go skill`.
Read-only architecture session `ses_f1c014f75ffessMyXvzZhV0KvT` inspected provider
aggregate revisions, DB/runtime conversion, credential validation, admin/membership
locks, crypto/migrations, device protocol, UI forms and auth generations. Coordinator
independently inspected the primary integration points. No baseline tests run during
analysis. No dependency, deployment or live-provider audit is claimed.

Choose a pre-save, DB-backed, user-and-org-bound temporary device session. It stores
the encrypted pending device challenge, then encrypted ready credential. Existing
provider aggregate POST/PUT consumes that ready session exactly once and saves the
dedicated provider credential in the same DB transaction. This avoids partial draft
providers, browser token handling, process-local sessions and a generic credential
management subsystem. There is no business decision blocking these defaults.

### Data And Ownership Contract

- Add `db_providers.copilot_credential_encrypted` using existing AES-GCM storage of
  versioned structured `copilotlogin.Credential`, never `APIKeyEncrypted`. The new
  field is valid only for Copilot and mutually exclusive with sidecar/API-key sources.
  Runtime materializes only into `Provider.CopilotToken`; do not expose a raw-token
  config/API input. Static HCL/JSON remains sidecar-based.
- Add a separate temporary flow table: random UUID, org/initiating-user IDs, purpose
  create/edit, optional immutable bound provider UUID, public client ID, state,
  encrypted challenge/ready payloads, expiry/poll/lease/revision/retention metadata,
  and consumed-provider identity/revision. Use foreign keys/checks/indexes; no raw
  credentials or arbitrary upstream error bodies. DB writes always encrypt first.
- States: starting → pending → ready → consumed, with denied/expired/failed/cancelled
  terminal alternatives. Terminal/consumed rows erase encrypted payloads. Ready is
  authorization only; provider becomes connected only on explicit successful save.
- Fixed initial bounds: pending <= issuer expiry and 15 minutes; ready 10 minutes;
  start reservation/poll lease 30 seconds; existing OAuth timeout 15 seconds; one
  live session/user/org, 20/org, 1,000 retained rows deployment-wide, one start/user
  per 30 seconds including failed/cancelled starts; terminal tombstones <=1 hour.
  Reserve capacity before issuer I/O. Atomic start admission may use a short PG
  advisory transaction lock, never held across network I/O. Cleanup runs in bounded
  batches each minute and on access, owned/stopped by App, safe across instances.
- Same encryption key must be available to instances. Existing key config/fallback
  remains; no new key-rotation infrastructure. Saved credentials are organization
  provider state; offboarding invalidates unfinished flows, not saved credentials.

### API And Polling Contract

- `POST /_internal/admin/copilot-device-flows`: `{client_id, org_id?, provider_name?}`.
  Creation resolves/pins org using existing write-org policy; editing resolves/pins
  stored provider UUID/org, rejects static providers/conflicting supplied scope.
- `GET /_internal/admin/copilot-device-flows/{id}`: sanitized status only, no OAuth.
- `POST /_internal/admin/copilot-device-flows/{id}/poll`: at most one token request,
  only when DB timing/state and lease permit. Early/competing calls return pending
  state and delay, not a terminal error or extra upstream call.
- `DELETE /_internal/admin/copilot-device-flows/{id}`: cancel/erase pending or ready
  payloads and invalidate in-flight lease. Terminal cancellation is idempotent.
- DTO: id/org_id/status, pending user_code/verification_uri/expires_at/poll_after_ms,
  ready_expires_at, bound/consumed provider_name and safe controlled error_code.
  Never return device_code/access_token/refresh_token/ciphertext. Responses use
  `Cache-Control: no-store`.
- These routes require current app-user JWT authority and org-admin/system-admin
  permission, plus initiating-user ownership on every operation. Another admin may
  not adopt a flow. Team-admin/dashboard-token/inference-key permissions do not suffice.
  Inaccessible IDs return 404, invalid login 401, insufficient known-scope rights 403.
- Explicit public client ID entered in UI; fixed existing read:user scope/GitHub
  issuer endpoints. No caller-selected scope/domain/endpoint or public mock flag.
  Validate returned browser URL against the fixed GitHub device authorization page.
  All web-facing failures use safe codes/messages rather than raw OAuth responses.
- Reuse an exported typed one-step operation extracted from existing `pollOnce`;
  CLI `Poll` remains its sleep/deadline loop over shared semantics. On web poll:
  claim a fenced DB lease, commit, perform one bounded request without DB locks,
  then finalize only if lease/state/expiry/current authority still match. Persist
  slowdown intervals from completion without lowering upstream advice. Use DB time.
  An abandoned/ambiguous lease fails for explicit restart rather than replaying an
  unknown exchange. No exactly-once GitHub/DB distributed-transaction claim.

### Saving, Authority And Failure Contract

- Provider POST/PUT accepts `copilot_device_flow_id` instead of a raw credential.
  Require `expected_updated_at` (opaque revision from provider view) for edits that
  consume a flow; UI uses it for other edits too. Existing non-flow clients may omit
  it. Add `copilot_credential_source: database|sidecar|none` and `has_credential` support.
  Keep provider-type credential kind compatible; add supports_device_authorization.
- Before validation, load an authorized ready snapshot without consuming it and
  attach encrypted candidate data. At aggregate commit recheck matching flow revision,
  ownership/purpose/org/provider UUID/expiry/current authority, plus provider revision.
  Consume flow, save provider and optional models atomically; rollback preserves
  previous provider and unconsumed ready flow. Do not import dbmerge into store.
- Lock order: actor user (FOR SHARE or stronger, so disable/demotion is serialized)
  → organization (existing NO KEY UPDATE membership mutex) → flow → provider.
  KEY SHARE alone does not protect non-key disabled/admin changes. Recheck actual
  user active/admin status and current org membership. User/org/provider deletion
  invalidates corresponding unfinished flows. Membership removal/authority-losing
  demotion and user disable invalidate them transactionally so remove/re-add and
  disable/re-enable cannot revive stale authorizations. Preserve LIFE invariants.
- Omitted flow/ref preserves credential bytes. Flow switches to DB and clears refs;
  nonempty sidecar switches to file and clears DB ciphertext. Explicit empty ref may
  clear only under existing enabled-provider validation. Flow+ref is ambiguous and
  rejected; wrong-type/API-key mixtures rejected. Apply source switching consistently
  in the legacy credential endpoint (it need not consume flows).
- Derived providers always have their own DB/file credential, never inherit a token.
  Resolved sidecar ref+CopilotToken is legitimate, not a conflicting source. Root
  defaults/local enabled/display-name semantics remain as implemented.
- After commit, request activation once before response presentation. Add a safe
  machine-readable saved indicator (e.g. `X-Aiproxy-Catalog-Saved: true`) set only
  after commit. Failed activation/view leaves complete saved provider + consumed
  flow, old runtime if activation failed; UI reads current state instead of blind
  POST retry. GET consumed tombstone recovers ambiguous network-loss outcomes.
  Existing reload activates receiving process only; no cluster broadcast is added.

### UI Contract

- Create: Authorize with GitHub or Existing CLI sidecar. Edit: Keep current credential
  by default, explicit Reauthorize or Switch to sidecar. Client ID, open-link/copy-code,
  pending/expiry/countdown/cancel/retry and ready states live in the form, not persisted
  storage/URLs. Ready text says "Authorized; save provider to apply".
- Disable device save until ready. Leave current saved credential intact on failure.
  Pin flow org and provider target; ambient org changes cannot retarget save. Use
  existing auth generations plus local attempt generation; abort timers/requests and
  best-effort cancel on close/type/source changes, never with a replacement account.
- Start/poll HTTP timeout exceeds OAuth timeout (e.g. 25s vs 15s), while retaining
  AbortSignal/generation guards. Poll by server-advised delay; no overlapping requests.
- Preserve unrelated change-only fields, untouched credentials, local models and
  inheritance behavior. Device bodies contain flow ID only, never an empty ref too.
  Existing Copilot "Rotate credential" must open reauthorization, not API-key form.
- Handle 409 with explicit reread/review, saved-but-activation-failed with saved-state
  message, and unknown network outcomes via flow/provider reads rather than replay.

## Sequential Execution And Verification

Execute WEB-COPILOT-01 through 05 sequentially, each in a fresh sub-agent session.
05 is an independent reviewer, not a previous implementer. No nested agents.
Set only owned task in_progress, append Completion evidence, mark completed only
after criteria/checks pass. Record necessary scope changes before implementation.
P1 = required identity/storage/runtime feature boundary; P2 = UI/verification delivery.

Commands run from `<repo-root>`. Backend tasks and final reviewer
use OWN NEW disposable PG17 per README: inspect Docker, unique names/DB, loopback
readiness, explicit AIPROXY_TEST_DATABASE_URL, GOFLAGS=-p=1; no developer/prior DB.
Prove relevant DB tests execute without prerequisite skips; remove only owned
fixture. Frontend-only task may use HTTP mocks and disclose optional DB skips.
All OAuth networking is test-injected local fixtures; never call GitHub here.

Default after nontrivial changes: `make vet test`; focused checks per task below.
Final: `make vet test`, `make test-race`, `make integration`, `make docs-contract`,
`pnpm --filter @aiproxy/web-ui test`, `pnpm --filter @aiproxy/web-ui typecheck`,
`git diff --check`. Serialize UI builds before Go asset readers and all DB suites.
Definition of done: five completed tasks with evidence, tested UI→API→DB→runtime
mock workflow and independent audit, docs agree, fixtures removed, prior work and
CHANGELOG preserved, live compatibility remains explicitly unverified/deferred.

### Task WEB-COPILOT-01: Add Encrypted DB Credential And Runtime Support

Status: completed

Kind: improvement

Priority: P1, prerequisite for server-side credential persistence.

Suggested agent: credential representation/runtime implementer

Dependencies: none

Primary ownership: `internal/store/models.go`, new forward migration,
`internal/copilotlogin/credential.go`, `internal/dbmerge/dbmerge.go`,
`internal/config/dynamic.go`, focused crypto/merge/config tests and design docs.

Finding: DBProvider stores only sidecar references and BuildProvider cannot resolve
an encrypted structured Copilot credential. Generic APIKeyEncrypted is incompatible.

References: DBProvider; BuildProvider/attachCredential/resolveCredentials;
validateDynamicCredential/ResolveCopilotCredential; store EncryptSecret/DecryptSecret.

Requirements:

1. Implement dedicated versioned encrypted credential representation and provider
   column with source/type constraints. Add forward migration, leave applied history.
2. Materialize validated DB credentials into CopilotToken only; protect malformed,
   wrong-key/domain/expiry/conflict cases with controlled errors and no leaked data.
3. Preserve sidecar/disabled/derived/root-default behavior and offline validation;
   no raw-token public input or OAuth networking in load/reload/inference.

Acceptance criteria:

- PG upgrade/existing-row compatibility and encrypted-at-rest roundtrip tests pass;
  ciphertext/plaintext never enters admin DTOs. Invalid blobs/sources fail safely.
- DB-derived credentials never inherit base tokens; sidecar and DB-backed concrete/
  derived/disabled providers validate as intended and correct runtime token is used.
- Existing mock Copilot, root defaults, billing and catalog tests remain correct.

Verification: PG-backed `go test -race -p 1 ./internal/store ./internal/dbmerge ./internal/config ./internal/app ./internal/copilotlogin`,
default checks; no API/UI feature claim yet. Publish next-agent data/helper contract.

Implementation scope clarification (WEB-COPILOT-01):

- Add `internal/store/copilot_credential.go` for typed encryption/decryption using
  existing crypto and a versioned payload; store may import copilotlogin, never dbmerge.
- Add migration `000009`; preserve preexisting `000008` byte-for-byte. Update the
  prior spend-ledger upgrade test's last-migration assertion to include the new forward
  migration, retaining its original ledger checks.
- Validate source/type combinations before decryption, including orphan reference
  paths. Add focused PG upgrade/roundtrip and merge/isolation tests; static config
  inputs remain sidecar-only. Existing admin DTOs remain explicit secret-free views.
- Add an App-level PG/local-upstream regression to verify saved DB credentials on
  reload/restart and JSON/SSE dispatch, and check existing admin views for leakage.
  This exercises storage directly and adds no public provisioning handler.
- Docker inspected before fixture creation; this session owns a new uniquely named
  loopback PG17 fixture and will run DB suites serially, then remove only that fixture.

Completion evidence (WEB-COPILOT-01):

- Added `internal/store/migrations/000009_copilot_credentials.sql`, the dedicated
  `DBProvider.CopilotCredentialEncrypted` field, typed crypto helpers in
  `internal/store/copilot_credential.go`, DB credential validation in
  `internal/copilotlogin/credential.go`, and runtime support in
  `internal/dbmerge/dbmerge.go` / `internal/config/dynamic.go`.
- Added `internal/store/copilot_credential_test.go`,
  `internal/dbmerge/copilot_credential_test.go`,
  `internal/config/dynamic_copilot_test.go`, and
  `internal/app/copilot_database_test.go`. Real PG coverage includes upgrade from
  000008 with existing rows, encrypted roundtrip/unrelated-edit preservation,
  SQL source/type constraints and rollback, persisted bad blobs/key/domain/expiry/
  version rejection, DB/file concrete/derived/disabled combinations and root defaults.
  Crypto checks also cover tampering, truncation, missing keys, key fallback, token
  injection, unknown fields/kinds, trailing JSON and missing fields. App checks prove
  derived token isolation on mock JSON/SSE, secret-free existing admin list/detail
  views, failed-reload rollback, and restart from saved DB state.
- Required full race command passed with `-count=1 -v`, `GOFLAGS=-p=1` and explicit
  owned-fixture URL: store 33.415s, dbmerge 1.626s, config 1.797s, app 28.749s,
  copilotlogin 1.382s. Inspected verbose output: **no SKIP or prerequisite skips**.
  Final strengthened PG merge/App tests also passed under `-race -p 1 -count=1 -v`
  (dbmerge 1.504s, app 1.737s), including persisted invalid cases and derived dispatch.
- `GOFLAGS=-p=1 AIPROXY_TEST_DATABASE_URL=<owned fixture> make vet test docs-contract`
  passed, including existing mock Copilot, billing, catalog and root-default suites;
  repeated after final test edits and passed. Initial full-check invocation exceeded
  the tool's 120s timeout; no Go test process remained, and the 600s retry passed.
  `git diff --check` passed. Documentation contract matrices match.
- Fixture: new Docker `postgres:17` container
  `aiproxy-webcopilot01-20260927-a91c` (`9f0d933462be`), database
  `webcopilot01_a91c`, user `webcopilot01`, loopback `127.0.0.1:46117`.
  Inspected Docker before creation, waited for healthy, serialized DB suites.
  Stopped only this owned `--rm` container; final filtered `docker ps -a` is empty.
- Updated design rationale in `docs/design.md`. Preserved incoming dirty work,
  migration 000008, other task statuses and CHANGELOG; only the documented legacy
  spend-ledger test assertion was relaxed to permit later forward migrations.
  No commits, nested agents, public provisioning input/handlers/UI, or live GitHub calls.

Next-agent representation/migration contract:

- Next migration is **000010**. 000009 adds nullable BYTEA
  `copilot_credential_encrypted`; NULL/nil means absent (empty nonnil blobs invalid).
  Its CHECK applies when this column is non-NULL: Copilot type only, no nonempty
  API-key ciphertext, no API-key ref path/key, and no sidecar path/name.
  Existing NULL-source rows remain untouched. The new upgrade test currently expects
  000009 alone after an 000008 baseline; extend its expectation when adding 000010.
- Call `store.EncryptCopilotCredential(cred copilotlogin.Credential, now time.Time)`
  before storing. The encrypted JSON envelope is version 1, kind `github-copilot`,
  nested `credential` with all six existing Credential fields. Call
  `store.DecryptCopilotCredential(blob, now)` to decode/revalidate. Both return
  `store.ErrCopilotCredential` on failure, with nil bytes/zero Credential and no
  secret-bearing underlying errors. Supply current DB time for transactional work;
  these helpers perform neither database writes nor OAuth calls/session transitions.
  `copilotlogin.ValidateDatabaseCredential` is the shared pure structured validator.
- Encryption uses existing AES-GCM/key selection unchanged. Store imports
  copilotlogin; it does not import dbmerge/config. BuildProvider checks local sources
  before decrypting, validates even disabled DB blobs, and materializes only the
  access token into `Provider.CopilotToken`. It clears base credentials before local
  attachment; sidecar resolution and resolved ref+token remain valid.
- Aggregate Bun inserts/updates already carry the new column; omitted credential
  edits preserve bytes by retaining the loaded row. Future source switches must
  explicitly clear DB ciphertext to nil for sidecars, or both sidecar fields and
  API-key fields for DB credentials. Session consumption, public source/has-credential
  DTO changes and legacy endpoint switching remain owned by 02/03 as planned.
- Blockers: none for WEB-COPILOT-01. Public device authorization and live GitHub
  compatibility are not claimed by this storage/runtime completion.

### Task WEB-COPILOT-02: Implement Bounded Sessions And Atomic Credential Consumption

Status: completed

Kind: improvement

Priority: P1, identity and transaction boundary for the authorization workflow.

Suggested agent: PostgreSQL session/aggregate lifecycle implementer

Dependencies: WEB-COPILOT-01

Primary ownership: new flow migration/models/store module, provider aggregate
transaction helpers, membership/user authority invalidation and focused PG tests.

Finding: No device-session storage exists. Existing aggregate writes protect revision
but cannot atomically consume an owner-bound ready authorization or expire its state.

References: provider_aggregate.go Create/UpdateProviderAggregate; membership.go
lock/role/removal methods; crud.go UpdateUser/DeleteOrganization; store crypto.

Requirements:

1. Implement states/bounds/admission/status/claim/finalize/cancel/cleanup contracts
   above, using DB time/fencing and encrypted payloads. Recheck authority at boundaries.
2. Integrate aggregate consume transaction with validated snapshot/revision checks
   without circular imports or duplicated SQL. Support create/edit intent and immutable
   provider binding. Preserve source switching and existing revision/caller ownership.
3. Invalidate unfinished sessions on authority loss/deletion; preserve offboarding
   last-admin/lock contracts and saved-provider lifetime. Expose bounded cleanup API
   for App lifecycle integration in 03.

Acceptance criteria:

- Two store instances race start/poll/consume: limits hold, one lease/one consume
  wins; early polls zero-claim, expired/cancelled/abandoned/stale completions rejected.
- Model-insert/conflict/validation failures leave ready flow and old provider intact;
  successful commit saves ciphertext and erases session payload exactly once.
- Current-user/org checks, cross-org/other-user denial, lost authority during waits,
  remove/re-add, disabled/re-enabled, provider delete/recreate and cleanup are tested
  against real PG with lock-order/deadlock assertions and no partial writes.

Verification: PG-backed `go test -race -p 1 ./internal/store ./internal/dbmerge ./internal/httpapi`,
default checks, next-agent transaction/session contract.

Implementation clarification (WEB-COPILOT-02):

- Extract transaction-local aggregate helpers so both legacy writes and flow consumption
  share SQL and caller-copy/revision behavior. Flow snapshots are opaque server-side
  candidates; API validation happens before the consuming transaction.
- Use migration 000010; extend the 000008 upgrade test's forward-migration expectation.
  Deletion may remove flows via cascading foreign keys (including consumed tombstones);
  immutable UUID binding prevents provider delete/recreate from reviving authorization.
- Authority invalidation takes the affected user lock before organization locks and
  flow locks. User updates lock organizations in UUID order. Cleanup locks only flow
  rows, uses bounded SKIP LOCKED batches, and never subsequently locks parent rows.
- Retain terminal rows for one hour to preserve failed/cancelled start throttling;
  admission serializes only DB capacity accounting, with no issuer work in the store.
- Necessary admission detail: persist the last admitted start timestamp on the user
  row, independent of flow FK deletion, so deleting an organization cannot bypass the
  deployment-wide per-user 30-second throttle. Start uses the allowed stronger actor
  FOR UPDATE lock; other flow boundaries use FOR SHARE. The timestamp is not an
  additional retained-session row and ordinary user updates preserve it.

Completion evidence (WEB-COPILOT-02):

- Scope adjustment: none beyond the documented user-row throttle timestamp. All
  design bounds/limits retained as binding; no new key infrastructure, no network
  I/O inside store transactions, no cluster broadcast.
- Changed (dirty-baseline owned, no prior migration edits): `internal/store/migrations/000010_copilot_device_flows.sql`,
  `internal/store/copilot_device_flow.go`, `internal/store/provider_aggregate.go`,
  `internal/store/crud.go` (aggregate delegation + provider-flow delete),
  `internal/store/membership.go` + `UpdateUser/DeleteUser` invalidation,
  `internal/store/copilot_device_flow_test.go`,
  `internal/store/copilot_device_flow_lock_test.go`,
  `internal/store/copilot_credential_test.go` (000009+000010 upgrade expectation),
  `internal/store/provider_aggregate_test.go`. No CHANGELOG, no commits, no live calls.
- Verified with OWN NEW disposable PG17 per README (Docker inspected first):
  container `aiproxy-webcopilot02-20260927-c04d`, database `webcopilot02_c04d`,
  user `webcopilot02`, loopback `127.0.0.1:46128`, healthy before tests, serial
  `GOFLAGS=-p=1`, explicit `AIPROXY_TEST_DATABASE_URL`. Pre-existing
  `aiproxy-webcopilot02-20260927-b72e` left untouched; only owned fixture stopped/removed.
- Real PG results, `-race -p 1 -count=1`: focused
  `store -run TestCopilotFlow|TestCopilotCredential|TestProviderAggregate|TestSpendLedger`
  pass 24.5s, no SKIP; full `store` pass 52.324s; `dbmerge` pass 2.273s; `httpapi`
  pass 244.573s. `make vet` pass, `go vet ./internal/store ./internal/dbmerge ./internal/httpapi`
  pass, `make docs-contract` pass, `git diff --check` pass.
- Acceptance: two-store start race 1 win/1 limit; one poll lease/one consume wins;
  early poll zero-claim; slowdown never lowers interval; expired/cancelled/abandoned/
  stale-lease/late finalize rejected; terminal/cancel idempotent with erased payloads;
  model-fault/conflict/name-conflict leave ready flow + old provider + caller intact;
  success saves DB ciphertext once and erases session; cross-org/other-user denied;
  remove/re-add, demote/promote, disable/enable, system demote, provider delete/recreate,
  user/org delete invalidate without revival; saved provider survives actor lifecycle;
  per-user 30s (incl failed/cancelled, org-deletion-proof via user timestamp), 1 live/user/org,
  20/org, 1000 retained, 1h tombstones, bounded SKIP LOCKED cleanup (incl concurrent overlap).
  Lock-order/deadlock tests pass; store never imports dbmerge; no secret in DTO/lease/snapshot JSON.
- Next-agent store/session contract: `Store.StartCopilotFlow(CopilotFlowStart{ActorID,OrgID,ProviderID?,ClientID}) (CopilotFlowStatus,*CopilotFlowLease,error)`;
  `FinalizeCopilotFlowStart(lease, *copilotlogin.DeviceCode)`; `GetCopilotFlow`;
  `ClaimCopilotFlowPoll` (nil lease = wait, no extra I/O); `FinalizeCopilotFlowPoll(lease,CopilotFlowPollResult{State:pending|ready|denied|expired|other,Interval,Credential})`;
  `ReadyCopilotFlow` -> opaque `CopilotFlowSnapshot`; `Snapshot.AttachCredential(*DBProvider)`;
  `ConsumeCopilotFlow(snap, *DBProvider, *[]DBProviderModel)` atomic; `CancelCopilotFlow`;
  `CleanupCopilotFlows(limit 1..100)`. DTO `CopilotFlowStatus{id,org_id,status,user_code?,verification_uri?,expires_at?,poll_after_ms?,ready_expires_at?,provider_name?,consumed_provider_id?,consumed_updated_at?,error_code?}` with `no-store` left to API layer. Lock order everywhere: actor (`FOR UPDATE` on Start, `FOR SHARE` otherwise) -> org `NO KEY UPDATE` -> flow `FOR UPDATE` -> provider (`SHARE` on start-bind, `UPDATE` on consume/edit); cleanup locks only flow rows `FOR UPDATE SKIP LOCKED`; user invalidation locks affected user first then orgs in UUID order; `DeleteUser` takes user `FOR UPDATE` before org discovery/locks. Errors: `ErrCopilotFlowNotFound/Unauthorized/Forbidden/Limit/Unavailable/Input`, `ErrCatalogConflict` on stale provider/expired-during-wait.
- Blockers: none. API networking/cleanup worker remains WEB-COPILOT-03 as planned.

### Task WEB-COPILOT-03: Expose Device Authorization API And Wire Activation

Status: completed

Kind: improvement

Priority: P1, server orchestration and protected browser-facing contract.

Suggested agent: one-step OAuth/admin API implementer

Dependencies: WEB-COPILOT-02

Primary ownership: copilotlogin device step, new admin device-flow handlers/service,
admin routing/provider DTOs/save/switching, App dependency/cleanup lifecycle, tests/docs.

Finding: Only CLI invokes device authorization; admin provider writes accept references
and have no way to consume ready DB authorization or communicate credential source.

References: device.go RequestCode/Poll/pollOnce; admin.go routing/adminClaims;
admin_providers.go upsert/credential/view; admin_catalog_write.go activation; app.go.

Requirements:

1. Implement exact route/DTO contract and safe error mapping; only current initiating
   org/system admins act. Fixed issuer/verification URL/scope, no-store, no secret DTOs.
2. Reuse typed single-step poll in CLI loop; web claim/network/finalize follows lease
   contract and no DB lock during HTTP. Inject local issuer in tests only.
3. Wire provider creation/edit/source changes, client revision, type capability,
   has_credential, saved header and exactly-once postcommit activation. Preserve CLI.
4. Start bounded session cleanup after successful Build, preserve across reload,
   stop on Close; handle failed Build/Reload ownership using existing resource rules.

Acceptance criteria:

- HTTP PG/mocked issuer tests cover success/pending/slowdown/denial/expiry/cancel,
  early/concurrent polls, held success racing authority loss, arbitrary URL/error
  rejection and secret absence from all responses/logs.
- Actual App flow start→ready→provider save→JSON/SSE works with encrypted DB token,
  no sidecar. Conflict/failed model write permits retry, failed activation reports
  saved state with old runtime intact, subsequent reload recovers.
- Read-only status/denied requests make zero issuer calls; cleanup lifecycle and
  two service instances share coherent DB state, while no cluster reload is claimed.

Verification: PG-backed `go test -race -p 1 ./internal/copilotlogin ./cmd/aiproxy ./internal/store ./internal/httpapi ./internal/app ./internal/dbmerge`,
default/docs checks; publish precise frontend DTO/service/state contract.

Implementation scope clarification (WEB-COPILOT-03):

- No UI edits; this task publishes the exact frontend contract below for
  WEB-COPILOT-04 and implements only the server side. `SnapshotDependencies`
  on `httpapi.Handler` and `App.SetCopilotDeviceClient` are the only new
  injection seams; production defaults always use the fixed GitHub issuer,
  fixed `read:user` scope, and fixed verification page. Local loopback HTTP
  issuers are accepted only when explicitly injected by tests.
- Poll on a terminal flow returns `200` with the terminal status and zero
  issuer calls (consistent with idempotent cancel and consumed-tombstone
  recovery), not `409`. A `409` with current status is returned only when a
  claimed lease cannot be finalized (e.g. authority lost mid-poll).
- Provider views now expose opaque `updated_at` (RFC3339Nano) revision for
  `expected_updated_at`, `copilot_credential_source` (`database`/`sidecar`/
  `none`, Copilot only), and corrected `has_credential` (now includes DB
  ciphertext). `provider-types` adds `supports_device_authorization` (Copilot
  only) while keeping the existing `credential` kind. `X-Aiproxy-Catalog-Saved:
true` is set after every durable provider/credential commit, including
  saved-but-activation-failed responses.
- App owns one bounded cleanup worker (1-minute tick, batch of 100, 30s
  per-run timeout): started after successful `Build`, preserved across
  `Reload`, stopped on `Close`. `Reload` re-attaches the device-issuer factory
  so injected issuers survive activation reloads (production factory is nil).

Frontend DTO/service/state contract (for WEB-COPILOT-04):

- `POST /_internal/admin/copilot-device-flows` `{client_id, org_id?,
provider_name?}` -> `201` status DTO. `GET /_internal/admin/
copilot-device-flows/{id}` -> `200` status DTO, zero issuer calls.
  `POST .../{id}/poll` -> `200` status DTO, at most one upstream token call;
  early/competing polls return pending status plus `poll_after_ms` with no
  extra call. `DELETE .../{id}` -> `200` status DTO, idempotent cancel.
- Status DTO: `{id, org_id, status, user_code?, verification_uri?,
expires_at?, poll_after_ms?, ready_expires_at?, provider_name?,
consumed_provider_id?, consumed_updated_at?, error_code?}`; statuses
  `starting|pending|ready|consumed|denied|expired|failed|cancelled`;
  `error_code` in `access_denied|expired|authorization_failed|
lease_abandoned|cancelled|authority_lost`. Never contains device codes,
  tokens, or ciphertext. All flow responses carry `Cache-Control: no-store`.
- Provider create/update accept `copilot_device_flow_id` (flow ID only) and
  `expected_updated_at` (required for flow-consuming edits; `409` on stale,
  ready flow preserved). Ready text contract: "Authorized; save provider to
  apply". Auth: current app-user JWT + org/system admin + initiating-user
  ownership; `404` foreign IDs, `401` bad login, `403` insufficient rights.

Completion evidence (WEB-COPILOT-03):

- Changed: `internal/copilotlogin/device.go` (exported `PollOneStep`/
  `PollStepResult`; `Poll` loop delegates with identical sleep/deadline
  semantics), new `internal/httpapi/admin_copilot_flows.go` (start/status/
  one-token-poll/cancel, fixed issuer/scope/verification URL, 25s issuer
  timeout, no DB lock during HTTP, safe error codes, `no-store`),
  `internal/httpapi/admin.go` + `handler.go` (routes, `CopilotDeviceClient`
  factory, `SnapshotDependencies`), `internal/httpapi/admin_providers.go`
  (flow-ID consume on POST/PUT, `expected_updated_at` revision, type
  capability, `copilot_credential_source`/`updated_at`/`has_credential` views,
  `supports_device_authorization`, saved header, legacy source switching),
  `internal/httpapi/admin_catalog_write.go` (not-ready `409`),
  `internal/app/copilot_cleanup.go` + `internal/app/app.go` (App-owned
  bounded cleanup worker, reload-safe issuer factory,
  `SetCopilotDeviceClient`), `website/docs/api-reference.md` (device-flow +
  provider-save contract). No CHANGELOG, no commits, no nested agents.
- Added: `internal/copilotlogin/pollstep_test.go`,
  `internal/httpapi/admin_copilot_flows_test.go`,
  `internal/app/copilot_provision_test.go`. Real PG coverage: start to
  pending/ready, slowdown interval growth without lowering, denial/expiry/
  cancel (+idempotent cancel), early/concurrent polls with exact one/zero
  upstream-call counts, held lease racing membership removal (finalize
  rejected, no credential, `404`/`403` on HTTP), arbitrary verification-URL
  and unknown/malformed OAuth error rejection to safe `authorization_failed`,
  secret scans on every response, read-only zero-call status, dual-handler
  coherence, legacy sidecar switch clearing DB bytes, wrong-type/flow+ref/
  missing/stale-revision rejection with ready-flow preservation, App
  start→ready→save→JSON/SSE dispatch on the encrypted DB token with no
  sidecar, conflict retry, failed-model retry, failed activation leaving
  saved provider + consumed flow + saved header with old runtime intact and
  reload recovery, second-instance coherence, cleanup worker lifecycle.
- Verified with OWN NEW disposable PG17 per README (Docker inspected first):
  container `aiproxy-webcopilot03-20260927-c03a`, database
  `webcopilot03_c03a`, user `webcopilot03`, loopback `127.0.0.1:46133`,
  serial `GOFLAGS=-p=1`, explicit `AIPROXY_TEST_DATABASE_URL`. Results
  `-race -p 1 -count=1`: copilotlogin ok 1.479s, dbmerge ok 2.130s, store ok
  67.481s, httpapi ok 321.458s, app ok 32.753s, cmd/aiproxy ok 80.574s;
  `make vet` ok, `make test` (full `go test ./...`) ok, `make docs-contract`
  ok, `git diff --check` ok. Focused verbose run: 18 PASS, zero SKIP/FAIL.
  Fixture stopped/removed afterwards; prior containers untouched.
- Acceptance: all WEB-COPILOT-03 criteria pass. Live GitHub never contacted
  (httptest issuers only). Blockers: none. Frontend contract published above
  for WEB-COPILOT-04.

### Task WEB-COPILOT-04: Add Connect GitHub To Provider Create And Edit

Status: completed

Kind: improvement

Priority: P2, requested interactive provisioning and recovery workflow.

Suggested agent: provider form/device authorization UI implementer

Dependencies: WEB-COPILOT-03

Primary ownership: provider page, focused authorization panel, services/types and
mounted form/auth-lifecycle tests, relevant operator docs.

Finding: UI currently requires credentialName from CLI and opens an API-key rotation
form for generic credential actions. It cannot show device authorization or DB source.

References: providers-page.tsx ProviderForm/buildProviderBody/providerToFormValues;
services/admin.ts; auth-lifecycle.ts; existing providers-page.test.tsx.

Requirements:

1. Implement creation/edit source choice, Connect/open/copy/wait/cancel/retry/ready
   states, 25s start/poll timeout, server interval polling and ready-only save.
2. Preserve identity/org pinning, change-only forms/inheritance, existing sidecars,
   current credential on failure, and safe auth/local attempt generations/cleanup.
3. Handle revision conflict, consumed/lost-response and saved-but-activation-failed
   outcomes truthfully; no credential in browser responses/storage/URLs. Copilot
   rotate action uses reauthorization. Accessible labels/status and type=button.

Acceptance criteria:

- Mounted actual page/service tests assert create/re-auth/sidecar/unchanged edit
  HTTP bodies (flow ID, no token), source/reopen state and ready-only save.
- Pending/slowdown/expiry/denial/retry/cancel/close/type/org/account changes and late
  replies cause no stale state installation, overlapping polls or wrong-scope save.
- Save conflict preserves ready flow for explicit reread; saved error and response
  loss recover current state without duplicate creation. No API-key form for Copilot.

Verification: `pnpm --filter @aiproxy/web-ui test`, `pnpm --filter @aiproxy/web-ui typecheck`,
default/docs checks with optional DB omissions explicit; final real PG integration next.

Scope adjustment (WEB-COPILOT-04):

- Panel lives inside `providers-page.tsx` (no new page file) to preserve baseline file ownership; services/types/tests/docs updated as owned.
- Edit saves always include `expected_updated_at` when a revision is known (backend ignores it off-flow), satisfying "UI uses it for other edits too" without breaking empty-revision fixtures.
- Create-with-device includes pinned `org_id`; edit-with-device pins `provider_name` only; ambient org/account drift blocks device save with restart guidance instead of retargeting.
- Poll 409 with current status is adopted as flow state; terminal poll/cancel are idempotent; cleanup cancels only live flows with unchanged auth generation.
- Existing Copilot reference tests updated for keep/sidecar source choice; new mounted fixtures use Axios adapters locally, no GitHub calls.

Completion evidence (WEB-COPILOT-04):

- Changed: `web-ui/src/types.ts` (flow status schema, `updated_at`/`copilot_credential_source`/`supports_device_authorization`), `web-ui/src/services/admin.ts` (+25s flow start/poll/status/cancel), `web-ui/src/pages/providers-page.tsx` (source choice, Connect/open/copy/pending/countdown/cancel/retry/ready, pinned org/provider, attempt+auth generations, no-overlap server-advised polling, ready-only save, 409/saved-but-failed/loss recovery, Copilot Rotate→reauthorization, type=button, labels/status), `web-ui/src/pages/providers-page.test.tsx` + `web-ui/src/services/admin.test.ts` + `web-ui/src/services/auth-lifecycle.test.ts` (mounted bodies/reopen/switching/gating/conflict/recovery/no-API-key-form, 25s/timeout/sanitized DTO, generation isolation), `website/docs/operations.md` (Connect GitHub alternative, no browser secrets, Rotate→reauthorization).
- Verified: `pnpm --filter @aiproxy/web-ui test` 11 files/209 tests pass; `pnpm --filter @aiproxy/web-ui typecheck` pass; `make vet` pass; `make docs-contract` pass ("documentation contract matrices match"); `git diff --check` pass. Backend `go test ./internal/httpapi` fails without a PG fixture (8 Copilot tests need `AIPROXY_TEST_DATABASE_URL`); UI-only per task allowance, no fresh PG created; `go test ./internal/store ./internal/config ./internal/copilotlogin` pass where hermetic. No GitHub calls; all OAuth networking is Axios-injected local fixtures.
- Outcomes: create sends `{name, type, models, copilot_device_flow_id, org_id?}` only when ready, never `credential_ref`/codes/tokens; edit re-auth sends `{copilot_device_flow_id, expected_updated_at}`; sidecar switch sends `{credential_ref}`; keep/unchanged sends `{expected_updated_at?}` only; inherited models/healthcheck untouched; late/duplicate polls, type/source/close cancels, org/account drift blocks wrong-scope save; 409 preserves ready flow for reread/retry; saved-but-failed shows saved-state + reload; network loss recovers via provider/flow reads, no duplicate POST.
- Preserved: baseline dirty work, other task statuses, CHANGELOG (untouched), no commits, no nested agents, no credentials in browser storage/URLs/responses.
- Blockers: none for WEB-COPILOT-04. Real PG UI→API→DB→runtime composition deferred to WEB-COPILOT-05; live GitHub remains deferred (COPILOT-LIVE-01).

Reviewer contract (frontend DTO/service/state):

- `POST /_internal/admin/copilot-device-flows {client_id, org_id?, provider_name?}` → 201 flow DTO (create pins org via `org_id`; edit pins provider via `provider_name`, no `org_id`).
- `GET .../{id}` → 200 DTO, zero issuer calls (reread/recovery path).
- `POST .../{id}/poll` → 200 DTO, ≤1 upstream call; early/competing → pending + `poll_after_ms`; terminal → terminal DTO; lease-conflict 409 body is adopted as current status.
- `DELETE .../{id}` → 200 DTO, idempotent cancel (live flows only, same-generation best-effort).
- DTO `{id, org_id, status, user_code?, verification_uri?, expires_at?, poll_after_ms?, ready_expires_at?, provider_name?, consumed_provider_id?, consumed_updated_at?, error_code?}`; statuses `starting|pending|ready|consumed|denied|expired|failed|cancelled`; never device codes/tokens/ciphertext; `Cache-Control: no-store` server-side.
- Provider save: create `{..., copilot_device_flow_id, org_id?}`; edit `{..., copilot_device_flow_id, expected_updated_at}`; 409 stale → reread `GET providers` for `updated_at` then retry with preserved flow; 500 `saved but activation failed` + `X-Aiproxy-Catalog-Saved: true` → saved-state message + reload, no blind retry; network loss → `GET providers` + `GET flow` (consumed→saved, ready→retry) without duplicate POST. Ready text: "Authorized; save provider to apply".

### Task WEB-COPILOT-05: Independently Verify Composed Workflow And Contracts

Status: completed

Kind: improvement

Priority: P1, independently validate authorization, storage and UI integration.

Suggested agent: independent reviewer, not WEB-COPILOT-01–04 implementer

Dependencies: WEB-COPILOT-01, WEB-COPILOT-02, WEB-COPILOT-03, WEB-COPILOT-04

Primary ownership: independent review, composed mock PG/API/runtime and UI evidence,
necessary scoped corrections, public docs/examples/AGENTS and this record.

Finding: Unit flow success alone cannot establish actual UI intent, transaction
isolation, encrypted persistence, cross-instance leases or reload semantics.

References: all preceding contracts/evidence and actual changed producer/consumer code.

Requirements:

1. Trace form→API→issuer→DB→save→runtime, both credential sources/derived models,
   authority changes and alternate save routes. Add missing composed regressions;
   record scoped corrections before edits. Verify encryption/no browser secrets,
   cancellation/expiry/bounds, lock order, duplicate submits and lost responses.
2. Run shared final gates with own fresh PG/no prerequisite skips; verify two service
   instances, App restart/reload and mocked JSON/SSE using stored DB credential.
3. Reconcile docs/help/examples/AGENTS with new DB-vs-CLI modes and client-ID need,
   encryption configuration, abandoned-flow cleanup, saved states and instance-local
   activation. Keep mock-only qualifier and live task deferred; no external calls.
4. Record per-task acceptance/status audit, actual verification/cleanup/limits and
   confirm existing work and CHANGELOG preservation.

Acceptance criteria:

- Five tasks completed with actual evidence and no unresolved criterion/blocker;
  final checks pass and public/UI/API/runtime contracts agree.
- No raw token/challenge leakage, cross-user flow adoption or late result revival
  found in tested paths. Owned fixture removed; deferred live work stays deferred.

Verification: shared final gates, independent source/acceptance audit and composed mocks.

Completion evidence (WEB-COPILOT-05, independent reviewer; implementer of 01-04: no):

- Trace verified against actual code (not plan prose): `providers-page.tsx`
  device panel (source choice, Connect/open/copy/pending/countdown/cancel/retry,
  ready text "Authorized; save provider to apply" line 990, Rotate->openEdit
  reauthorization lines 1435-1443) -> `services/admin.ts` (25s flow start/poll,
  sanitized zod DTO) -> `admin_copilot_flows.go` (fixed issuer/scope/verification
  URL, 25s issuer timeout, `no-store`, <=1 token call per claimed lease) ->
  `copilotlogin/device.go PollOneStep` -> `store/copilot_device_flow.go` +
  `copilot_credential.go` (AES-GCM, versioned envelope) -> provider POST/PUT
  consume in `admin_providers.go` (flow-ID-only, `expected_updated_at`, source
  switching, saved header) -> `dbmerge`/`dynamic.go` runtime (`CopilotToken`
  only) -> `app/copilot_cleanup.go` worker (1-min tick, 100/batch, reload-safe).
  Static HCL/JSON remains sidecar-only; no raw-token input exists anywhere.
- Added composed regression `TestAdminCopilotReviewerDuplicateAndBinding`
  (`internal/httpapi/admin_copilot_flows_test.go`, appended to the owned 03 file,
  no new files): UI-intended create body saves via ready flow (201 + saved
  header); ciphertext at rest holds no plaintext token and decrypts with the
  pinned client ID; provider view `database`/`has_credential=true`; second
  handler instance observes consumed tombstone + saved provider; edit-bound flow
  reused for a create is rejected 400 with the ready flow preserved; two
  concurrent creates racing one ready flow yield exactly one 201 (saved header)
  and one 409, the tombstone references the winner, and the loser row never
  exists (lost-response recovery via GET flow/provider, no duplicate POST).
  Three org-admin actors used (one start/user/30s throttle is by design).
  Derived-model token isolation stays covered at store/dbmerge level by 01
  (HTTP derived-via-flow not added: harness catalog is static so
  `validateExtends` would reject the just-saved base for harness reasons).
- Docs corrections (behavior unchanged, mock-only + COPILOT-LIVE-01 kept):
  `AGENTS.md` Copilot bullet (was CLI-sidecar-only) now documents Connect
  GitHub DB alternative, UI client-ID need, shared encryption key, bounded
  cleanup, saved-state header, instance-local activation, mock-only pointer;
  `website/docs/operations.md` Connect GitHub paragraph extended with
  encryption-key sharing, expiry/cleanup bounds, saved states, instance-local
  activation; `website/docs/providers-and-routing.md` + `examples/github-copilot.hcl`
  gained DB-alternative pointers. No CHANGELOG edits, no commits, no live calls.
- Gates, all with OWN NEW disposable PG17 (Docker inspected first; container
  `aiproxy-webcopilot05-20260927-f8045`, db `webcopilot05_f8045`, user
  `webcopilot05`, loopback `127.0.0.1:46151`, healthy before tests, serial
  `GOFLAGS=-p=1`, explicit `AIPROXY_TEST_DATABASE_URL`): new test pass;
  `store`/`dbmerge`/`config`/`copilotlogin` race-verbose pass with zero
  SKIP/--- SKIP lines; `httpapi`/`app`/`dbmerge` race-verbose pass with zero
  skips (incl. re-run dual-service coherence, provision end-to-end with second
  instance + failed-activation saved state + reload recovery + JSON/SSE on the
  stored DB token, restart-from-DB runtime/views); `cmd/aiproxy` race-verbose
  pass with zero skips; `make vet`, full `make test`, full `make test-race`,
  `make integration`, `make docs-contract` ("matrices match"),
  `pnpm --filter @aiproxy/web-ui test` (11 files/209 tests), `pnpm --filter
@aiproxy/web-ui typecheck`, `git diff --check` -- all pass. UI build
  finished before Go asset readers per serialization rule.
- Per-task audit: 01 completed (storage/runtime; re-verified above, no skips);
  02 completed (sessions/atomics; store race suites re-run pass, one-consume +
  invalidation + cleanup semantics intact); 03 completed (API/activation;
  httpapi/app/cmd suites re-run pass, two-instance coherence + restart/reload +
  JSON/SSE + saved-but-failed + consumed recovery intact); 04 completed (UI;
  209 tests + typecheck re-run pass, bodies/gating/conflict/recovery/no-API-key
  behavior matches the published frontend contract, ready text + Rotate paths
  confirmed in source); 05 completed here. No unresolved criterion/blocker.
  No token/challenge leakage (secret scans in every flow response + new at-rest
  check), no cross-user adoption (ownership re-verified at every boundary),
  no late revival (lease/expiry/consumed guards re-tested).
- Cleanup/limits: owned PG container stopped and auto-removed; filtered
  `docker ps -a` shows only pre-existing
  `aiproxy-webcopilot02-20260927-b72e` (healthy, untouched). Baseline dirty
  work otherwise preserved; CHANGELOG verified byte-identical
  (`git status -- CHANGELOG.md` empty). Residual risk unchanged: live GitHub
  compatibility stays explicitly deferred in COPILOT-LIVE-01; activation is
  instance-local (no cluster broadcast); device sessions are process-uncoordinated
  beyond the shared DB rows.

Scope adjustment (WEB-COPILOT-05, recorded before edits):

- Trace (actual code, not plan prose): `providers-page.tsx` device panel ->
  `services/admin.ts` (`startCopilotDeviceFlow`/`pollCopilotDeviceFlow`, 25s
  timeout) -> `POST /_internal/admin/copilot-device-flows`,
  `POST .../{id}/poll` in `internal/httpapi/admin_copilot_flows.go` (fixed
  issuer/scope/verification URL, 25s issuer timeout, `no-store`, one bounded
  token call per claimed lease) -> `internal/copilotlogin/device.go`
  `PollOneStep` -> `internal/store/copilot_device_flow.go` (admission,
  claim/finalize, consume) with AES-GCM via
  `internal/store/copilot_credential.go` -> provider POST/PUT consume in
  `internal/httpapi/admin_providers.go` (`copilot_device_flow_id` only,
  `expected_updated_at` revision, source switching, saved header) ->
  `internal/dbmerge/dbmerge.go` + `internal/config/dynamic.go` runtime
  (`Provider.CopilotToken`) -> `internal/app/copilot_cleanup.go` worker.
  Static HCL/JSON stays sidecar-only; raw-token config/API input does not exist.
- Missing composed regression to add (existing suites cover sequential
  replay, legacy sidecar switch, dual-service cancel, App reload + JSON/SSE,
  store-level races; none covers these at HTTP composition): concurrent
  duplicate provider creates racing one ready flow (one 201 / one 409, consumed
  tombstone recovery, no partial loser), edit-bound flow reuse for a create
  (400 binding), ciphertext-at-rest plaintext absence + decryptability, and
  second-handler-instance visibility of the consumed save. Derived-model
  token isolation stays covered at store/dbmerge level (WEB-COPILOT-01); an
  HTTP derived-via-flow case is not added because the httpapi harness catalog
  is static and `validateExtends` would reject the just-saved base for harness
  reasons, not product reasons.
- Docs corrections to make (no behavior change): AGENTS.md Copilot bullet
  describes CLI sidecar only -- add Connect GitHub DB alternative, UI client-ID
  need, shared encryption-key requirement, bounded cleanup, saved-state header,
  instance-local activation, mock-only pointer, live-deferred pointer.
  `website/docs/operations.md` Connect GitHub paragraph lacks encryption-key
  sharing, cleanup bounds, and instance-local activation -- extend in place.
  `website/docs/providers-and-routing.md` + `examples/github-copilot.hcl`
  describe static sidecar only -- add DB-alternative pointers without changing
  static behavior. Keep every mock-only qualifier and COPILOT-LIVE-01
  deferral; no external calls; no CHANGELOG edits; no commits.
