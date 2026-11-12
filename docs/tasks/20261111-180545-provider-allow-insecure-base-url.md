# Provider `allow_insecure_base_url` opt-in

Created: 2026-11-11 18:05 UTC

## Objective

Allow operators to point a provider at a trusted plain-HTTP upstream that is not
on a loopback host (primary use case: another Service in the same Kubernetes
namespace, e.g. `http://my-llm.my-ns.svc.cluster.local`). Today
`validateProviderBaseURL` rejects every non-HTTPS base_url unless the host is
literally `localhost` or a loopback IP, and there is no opt-out.

Introduce a provider-only boolean attribute `allow_insecure_base_url`
(default `false`). When `true`, the `http` scheme is accepted for any host.
`https` remains always allowed; non-http(s) schemes, userinfo, and non-absolute
URLs remain rejected. The attribute is loud and explicit: no root-scope default,
no environment variable, no CLI bypass.

## Scope and non-goals

- In scope: HCL/JSON static config, validation, derived-provider rules,
  database-backed providers (store column + migration + dbmerge), admin API
  (upsert + views), web UI provider form, `configure provider` CLI (non-interactive
  and interactive), docs, tests.
- Non-goals: relaxing the dashboard `allow_insecure_remote` rule, changing
  provider health/cooldown semantics, root-scope defaults, TLS client
  configuration, forwarding any additional inbound headers.

## Analysis coverage and limitations

- Inspected: `internal/config/{schema,types,build,validate,dynamic,catalog,convert}.go`,
  `internal/store/{models.go,store.go,schema.sql,forward_authorization.sql}`,
  `internal/dbmerge/dbmerge.go`, `internal/httpapi/admin_providers.go`,
  `internal/configedit/configedit.go`, `cmd/aiproxy/configure.go`,
  `web-ui/src/{types.ts,pages/providers-page.tsx}`, tests pinning the loopback
  error, and docs (`docs/design.md`, `website/docs/*`, `AGENTS.md`).
- `cloneProvider` (config and dbmerge) copies the struct by value, so the new
  field inherits to derived providers automatically; the derived-provider
  attribute rejection list must forbid a local declaration.
- Convert (`internal/config/convert.go`) is generic over the syntax tree and
  revalidates by re-loading; no convert changes required.
- Verification baseline: `make vet test` not run before this file was created;
  run after implementation (see tasks).

## Tasks

### Task INSECURE-01: Static config validation supports `allow_insecure_base_url`

Status: completed

Kind: improvement

Priority: P1 — unblocks plain-HTTP in-cluster upstreams; deliberate opt-in keeps the default secure.

Suggested agent: Go config implementer

Dependencies: none

Primary ownership:

- `internal/config/schema.go`
- `internal/config/types.go`
- `internal/config/build.go`
- `internal/config/validate.go`
- focused tests in `internal/config`

Finding:

`validateProviderBaseURL` (internal/config/validate.go:330-352) rejects `http`
for non-loopback hosts with no override. `internal/config/opencode_test.go:193`,
`internal/config/copilot_test.go:358`, and `internal/config/load_test.go:598`
pin the error.

References:

- `internal/config/validate.go:344-348` (`validateProviderBaseURL`)
- `internal/config/build.go:531-545` (`buildProvider` literal)
- `internal/config/build.go:297-301` (`validateDerivedProviderSurface` forbidden attrs)

Requirements:

1. Add optional bool `allow_insecure_base_url` to `rawProvider` and `Provider`.
2. `buildProvider` maps it; `validateDerivedProviderSurface` rejects a local
   declaration on derived providers (the flag rides with the inherited
   base_url from `cloneProvider`).
3. `validateProviderBaseURL`: when the flag is set, accept `http` for any host;
   all other rules unchanged.
4. Existing loopback error tests keep passing (flag defaults false).

Acceptance criteria:

- `provider "openai-compatible" "x" { base_url = "http://svc.ns.svc.cluster.local/v1" ... allow_insecure_base_url = true }` loads.
- The same config without the flag fails with the existing error.
- `allow_insecure_base_url = true` on a derived provider errors ("derived provider cannot declare allow_insecure_base_url").
- JSON config form and `aiproxy convert` round-trip the attribute (generic path; covered by a load test with a JSON config).

Verification: `go test ./internal/config/...` (working directory: `<repo-root>`)

### Task INSECURE-02: Database-backed providers support the flag

Status: completed

Kind: improvement

Priority: P1 — multi-tenancy providers must be able to opt in like static ones.

Suggested agent: Go store implementer

Dependencies: INSECURE-01

Primary ownership:

- `internal/store/models.go`
- `internal/store/store.go`
- `internal/store/allow_insecure_base_url.sql` (new)
- `internal/dbmerge/dbmerge.go`

Finding:

`store.DBProvider` has no field; `dbmerge.BuildProvider` cannot carry the flag
into `config.Provider`; validation of merged providers would fail for
non-loopback http base_urls.

References:

- `internal/store/forward_authorization.sql` (migration pattern)
- `internal/store/store.go:28` (migration registration)
- `internal/dbmerge/dbmerge.go:186-193` (`BuildProvider` non-extends literal)

Requirements:

1. New migration adding `allow_insecure_base_url BOOLEAN NOT NULL DEFAULT false`
   to `db_providers`, registered after `forward_authorization.sql`.
2. `DBProvider` gains the bun-tagged field; `BuildProvider` maps it for
   non-derived rows; derived rows inherit the base's value via clone (no row
   override).
3. Update migration Applied-list pins in `internal/store/spend_ledger_test.go`
   and `internal/store/copilot_credential_test.go`.

Acceptance criteria:

- A row with `allow_insecure_base_url = true` and an http non-loopback base_url builds and passes `config.ValidateDynamicProvider`.
- A derived row inherits the base's flag.
- A stored derived row cannot carry a true local flag override.
- Fresh and migrated databases both expose the column.

Verification: `go test ./internal/store/... ./internal/dbmerge/...` (working directory: `<repo-root>`)

### Task INSECURE-03: Admin API and web UI expose the flag

Status: completed

Kind: improvement

Priority: P1 — workspace administrators need parity with static config.

Suggested agent: Go/TS full-stack implementer

Dependencies: INSECURE-02

Primary ownership:

- `internal/httpapi/admin_providers.go`
- `web-ui/src/types.ts`
- `web-ui/src/pages/providers-page.tsx`

Finding:

`adminProviderUpsert` and `adminProviderView` lack the field, so the admin API
cannot set or report it; the web UI provider form has no control.

References:

- `internal/httpapi/admin_providers.go:334-354` (`adminProviderUpsert`)
- `internal/httpapi/admin_providers.go:785-851` (`applyProviderUpsert`)
- `web-ui/src/types.ts:274-298` (`adminProviderSchema`)
- `web-ui/src/pages/providers-page.tsx:354` (payload mapping pattern)

Requirements:

1. `allow_insecure_base_url *bool` on the upsert; apply when present.
2. `allow_insecure_base_url bool` on views for both static and database providers.
3. Web UI: zod field, form state, payload mapping, and a toggle near the
   base_url field with a cleartext warning hint.
4. Validation flows through `config.ValidateDynamicProvider` (already covered by
   INSECURE-01/02 semantics).

Acceptance criteria:

- `POST/PUT` provider with `"allow_insecure_base_url": true` and an http base_url succeeds and is reflected in GET views.
- Static-config providers report their flag in the merged view.
- Web UI form round-trips the toggle; non-derived providers only.
- Explicit true/false declarations on derived provider POST/PUT fail before
  persistence; concrete-to-derived transitions clear omitted local flags.

Verification: `go test ./internal/httpapi/...` and web-ui checks (working directory: `<repo-root>`)

### Task INSECURE-04: `configure provider` CLI supports the flag

Status: completed

Kind: improvement

Priority: P2 — scripted/interactive provisioning should reach the attribute.

Suggested agent: Go CLI implementer

Dependencies: INSECURE-01

Primary ownership:

- `internal/configedit/configedit.go`
- `cmd/aiproxy/configure.go`

Finding:

`providerOptions` and `ProviderInput` lack the field; `RenderProviderBlock`
cannot emit it; `existingProviderInput` would drop the attribute when
re-rendering an edited block.

References:

- `internal/configedit/configedit.go:283-326` (`RenderProviderBlock`)
- `cmd/aiproxy/configure.go:72-116` (`providerOptions`)
- `cmd/aiproxy/configure.go:4158-4191` (`existingProviderInput`)

Requirements:

1. `--allow-insecure-base-url` flag (non-interactive), stored with a Changed sentinel.
2. `ProviderInput.AllowInsecureBaseURL` + emission after `forward_authorization`
   (and preserved on edits via `existingProviderInput`).
3. Interactive flow: boolean prompt near base_url for non-derived providers.

Acceptance criteria:

- `aiproxy configure provider --non-interactive --type openai-compatible --name x --base-url http://svc/v1 --allow-insecure-base-url ...` renders the attribute and the config validates.
- Editing an existing provider that has the flag preserves it.
- Derived providers reject the flag combination with a clear error.

Verification: `go test ./cmd/aiproxy/...` (working directory: `<repo-root>`)

### Task INSECURE-05: Docs and conventions

Status: completed

Kind: improvement

Priority: P1 — public attribute without docs is a contract drift.

Suggested agent: docs implementer

Dependencies: INSECURE-01

Primary ownership:

- `docs/design.md`
- `website/docs/configuration.md`
- `website/docs/api-reference.md`
- `AGENTS.md`

Finding:

`docs/design.md:1564-1565` states the absolute rule with no opt-in; the
attribute must be documented wherever the loopback rule and per-provider
opt-ins are listed.

References:

- `docs/design.md:1561-1566`
- `website/docs/configuration.md:233-286` (provider attributes + admin API provider sections)
- `website/docs/api-reference.md:271-282` (upsert partials)

Requirements:

1. Design doc: update the base_url rule and add the attribute to the provider
   attribute lists (base, derived forbidden list, dynamic provider rules).
2. Configuration docs: attribute description with security warning and k8s
   example.
3. API reference: document the upsert/view field.
4. AGENTS.md: convention bullet mirroring the other provider opt-ins.

Acceptance criteria:

- `make docs-contract` passes.
- All four docs mention the attribute consistently with its semantics.

Verification: `make docs-contract` (working directory: `<repo-root>`)

### Task INSECURE-06: Full verification

Status: completed

Kind: improvement

Priority: P1 — repo-mandated sanity checks.

Suggested agent: integrator

Dependencies: INSECURE-01, INSECURE-02, INSECURE-03, INSECURE-04, INSECURE-05

Primary ownership:

- none (verification only)

Requirements:

1. `make vet` and `make test` clean.
2. Web UI test/typecheck command per repo config.
3. `make docs-contract`.

Acceptance criteria:

- All commands pass with evidence recorded below.

Verification: see acceptance criteria.

## Definition of done

- All tasks completed with evidence in this file.
- No absolute local paths or session identifiers in this document.

## Implementation notes and deviations

- The interactive `configure provider` flows prompt for the flag only when the
  entered base_url parses as an absolute plain-HTTP URL (`isPlainHTTPBaseURL` in
  `cmd/aiproxy/configure.go`). For `https` URLs the flag is a no-op, so
  prompting would be noise; this also keeps existing scripted line-oriented
  test inputs valid without modification.
- The CLI renders the attribute immediately after `base_url` (inside the
  `base_url` emission block in `RenderProviderBlock`) rather than after
  `forward_authorization`, keeping the attribute adjacent to the URL it
  qualifies.
- Discovered follow-up (out of scope, pre-existing): the JSON-config derived
  provider forbidden-attribute scan in `internal/config/load.go`
  (`annotateProviderSyntax` attribute schema) does not list
  `forward_user_agent`, so a JSON derived provider declaring it is not
  rejected while the HCL path rejects it. `allow_insecure_base_url` was added
  to that list; `forward_user_agent` was left untouched to avoid a behavior
  change outside this task.

## Verification evidence

Review reopening (2026-11-11): the earlier evidence below remains historical.
The subsequent review reproduced a prohibited derived-row flag being silently
accepted, so INSECURE-02/03/06 were reopened. Follow-up implementation and fresh
verification are tracked in
[review closure](20261111-213252-review-closure-remediation.md).

- `go build ./...` — clean (no host Go toolchain; installed go1.27.1 under a
  session-local tool directory and prefixed `PATH`).
- `go vet ./...` — clean.
- `go test ./internal/config/...` — ok; includes new
  `TestLoadAllowInsecureBaseURL` (flag allows non-loopback http, flag false
  keeps rejection, derived inherits flag, JSON round-trip, JSON rejects flag
  on derived) and the new `forbidden allow_insecure_base_url` case in
  `TestLoadRejectsInvalidDerivedProviders`.
- `go test ./internal/store/...` — ok against a live PostgreSQL test database;
  migration Applied-list pins updated to include `allow_insecure_base_url.sql`
  in `internal/store/spend_ledger_test.go` and
  `internal/store/copilot_credential_test.go`.
- `go test ./internal/dbmerge/...` — ok; new
  `TestBuildProviderAllowInsecureBaseURL` (row mapping + loopback rejection
  without the flag) and `TestBuildProviderDerivedInheritsAllowInsecureBaseURL`.
- `go test ./internal/httpapi/...` — ok against a live PostgreSQL test
  database; new `TestAdminProviderAllowInsecureBaseURL` (create with flag +
  view round-trip + PUT toggle), `TestAdminCreateProviderRejectsInsecureWithoutFlag`,
  `TestAdminProviderViewReportsStaticAllowInsecureBaseURL`.
- `go test ./cmd/aiproxy/...` — ok; new
  `TestConfigureProviderNonInteractiveAllowInsecureBaseURL`,
  `TestConfigureProviderEditPreservesAllowInsecureBaseURL`,
  `TestConfigureProviderNonInteractiveRejectsFlagOnDerived`.
- `make vet test` — exit 0, 26 packages ok (with `AIPROXY_TEST_DATABASE_URL`
  set to a local disposable test database).
- `make docs-contract` — pass ("documentation contract matrices match").
- `gofmt -l` over all touched packages — clean.
- web-ui `pnpm typecheck` — clean; `pnpm test` — 11 files, 211 tests passed,
  including the new plain-HTTP opt-in round-trip test in
  `web-ui/src/pages/providers-page.test.tsx`.
- `make build` (WEB_SKIP=1) — built `dist/aiproxy` successfully.

### Review correction and re-verification (2026-11-11)

- INSECURE-02: stored derived rows with a true local override are rejected;
  inherited opt-ins still pass. The fresh/legacy PostgreSQL 17 migration test
  preserves an existing provider, defaults the new flag to false, persists true,
  and verifies idempotence in an isolated test schema.
- INSECURE-03: derived POST/PUT rejects explicit true and false before persistence;
  a concrete-to-derived PUT clears an omitted local opt-in durably. Pure and
  PostgreSQL-backed regressions pass; concrete true/false toggles and static views
  remain covered.
- From `<repo-root>`, `make vet test`, `make test-race`, `make docs-contract`,
  `make helm-test`, web UI typecheck/tests (211), and full `make build` pass.
  The PostgreSQL-backed store/dbmerge/HTTP/app race suite passed, followed by
  focused race checks for the added migration/transition cases. `go mod tidy -diff`,
  gofmt, Prettier, and whitespace checks are clean.
- INSECURE-02/03/06 are completed again. The full remediation evidence and
  manifest-only Kubernetes verification limitation are recorded in
  [review closure](20261111-213252-review-closure-remediation.md).
