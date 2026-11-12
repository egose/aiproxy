# Working-tree review closure

Created: 2026-11-11 21:32 local

## Objective and scope

Fix the six confirmed review findings in guardrail redaction, capture completeness,
Helm isolation/connectivity, derived-provider validation, and logging config edits.
Preserve unrelated working-tree changes, audit-mode forwarding, static provider
inheritance, and the default omission of guardrail-covered payload bodies.

Related plan: [provider opt-in](20261111-180545-provider-allow-insecure-base-url.md),
especially INSECURE-02, INSECURE-03, and INSECURE-06. Unrelated CI edits and the
previously recorded JSON `forward_user_agent` follow-up are outside this scope.

## Analysis and baseline

- Inspected the working-tree diff, relevant plans, shared request boundary,
  scanner/capture and exception paths, config editing, provider upserts/dbmerge,
  and rendered parent/subchart manifests.
- Existing `make vet test`, affected-package tests, web typecheck/tests (211),
  docs contracts, and Helm lint/render passed before remediation.
- Temporary regression tests reproduced recoverable JSON-escaped secrets in
  payload logs, admission of 18 findings after clearing only 16 captures,
  silently ignored derived-row flags, and loss of the logging redaction setting.
- Helm rendering confirmed empty ingress sources allow all callers and the
  documented relay-to-API HTTP connection has no permitted egress route.
- Database-backed baseline tests skipped because no test database URL was set.
  No live Kubernetes networking test has been performed; manifest assertions
  cover the NetworkPolicy contract, not CNI behavior.

## Tasks

### Task LOG-01: Redact JSON without leaking escaped secrets

Status: completed

Kind: defect

Priority: P1 — blocked secrets must not remain recoverable from payload logs.

Dependencies: none

Primary ownership: `internal/httpapi/guardrail.go`, `internal/httpapi/handler.go`,
and focused redaction tests.

Finding and references: `handler.go:326` calls `rewriteGuardrailBody`, which
replaces raw bytes although scanning uses decoded strings (`guardrail.go:418`).

Requirements:

1. Rewrite decoded JSON strings and encoded tool arguments with valid escaping.
2. Preserve numbers/non-secret content; bound nesting and omit logs or block
   exception redaction when safe rewriting cannot complete.

Acceptance criteria:

- Literal, Unicode-escaped, repeated, and embedded-JSON secrets are masked.
- Custom quote/backslash placeholders keep JSON and tool arguments valid.
- Unrewritable/deep bodies fail closed; audit upstream bodies remain literal.

Verification: focused HTTP/redaction tests, then shared Go checks below.

### Task LOG-02: Do not clear truncated capture sets through exceptions

Status: completed

Kind: defect

Priority: P1 — uncaptured secrets can currently reach upstream in block mode.

Dependencies: none

Primary ownership: `internal/guardrails/quarantine.go`,
`internal/httpapi/guardrail.go`, and capture/exception tests.

Finding and references: completeness affects only log state
(`guardrail.go:289`); exception clearance considers only captured findings
(`guardrail.go:317-328`), including the truncated 16-finding set.

Requirements:

1. Require complete capture before exception clearance in block mode.
2. Keep admission independent of the logging flag and preserve audit forwarding.
3. Exactly-at-cap captures must remain complete when no findings were dropped.

Acceptance criteria:

- Eighteen findings with exceptions for sixteen never reach upstream in block
  mode, with/without quarantine and with either logging setting.
- Complete allow/redact decisions work identically with either logging setting.
- Cap/depth gaps omit request-side logs; exact-cap captures can be reviewed.

Verification: guardrails/HTTP tests and race checks below.

### Task HELM-01: Fail closed and explicitly permit relay-to-API traffic

Status: completed

Kind: defect

Priority: P1 — default ingress permits every caller; the HTTP example is unusable.

Dependencies: none

Primary ownership: `helm/main/templates/networkpolicy.yaml`, chart values/examples,
`helm/main/networkpolicy_test.go`, and the Helm verification target.

Finding and references: empty `from` lists at template lines 23-40 and 88-105
match all sources; relay egress at lines 118-138 lacks the API HTTP destination.

Requirements:

1. Emit no ingress allow rule when no sources are configured, for API and relay.
2. Add an explicit, default-off, same-release relay-to-API opt-in permitting both
   directions with narrow selectors and the configured API port.
3. Document the opt-in and add network-free parent-template regression tests
   using temporary charts, plus a `make helm-test` gate requiring Helm.

Acceptance criteria:

- Empty sources deny ingress; explicit namespace/pod peers retain their scope.
- Only the opted-in relay can contact the same-release API on its configured
  port; disabled components and overridden names are handled.
- DNS, explicit CIDR/extra egress, and NetworkPolicy disablement remain intact.

Verification: `make helm-test` and actual chart lint/render with relay enabled.

### Task DB-01: Enforce inherited-only transport opt-in

Status: completed

Kind: defect

Priority: P2 — database/admin behavior contradicts static config and public docs.

Dependencies: none

Primary ownership: `internal/dbmerge/dbmerge.go`,
`internal/httpapi/admin_providers.go`, provider/store tests, and original plan closure.

Finding and references: upsert stores a derived provider's local flag
(`admin_providers.go:816`); the derived builder silently ignores it
(`dbmerge.go:159-184`).

Requirements:

1. Reject explicit true or false declarations on derived admin POST/PUT.
2. Reject stored derived rows with a true local override during catalog build.
3. Preserve inherited values and safely clear local flags when changing a
   concrete provider into a derived one without declaring the flag.
4. Verify the original INSECURE-02 fresh/legacy database migration criterion;
   exercise a legacy provider in an isolated test schema and migration idempotence.

Acceptance criteria:

- Invalid declarations fail before persistence, including update/transition
  paths; inherited opt-ins and concrete true/false updates still work.
- PostgreSQL-backed provider checks pass; original plan statuses/evidence
  reflect the corrected behavior.

Verification: pure upsert/dbmerge tests and shared database checks below.

### Task CLI-01: Preserve payload-log redaction on config edits

Status: completed

Kind: defect

Priority: P2 — changing log level silently disables the selected logging behavior.

Dependencies: none

Primary ownership: `cmd/aiproxy/configure.go`, `internal/configedit/configedit.go`,
and logging edit/render tests.

Finding and references: `parsePayloadLogInput` (`configure.go:4477`) and
`PayloadLogInput`/`RenderLoggingBlock` omit the new attribute.

Requirements: carry explicit true/false values through parsing and rendering.

Acceptance criteria: level-only scripted and interactive edits preserve either
declared value, while absent attributes remain absent.

Verification: CLI/configedit tests, then shared Go checks.

### Task VERIFY-01: Verify and reconcile completion

Status: completed

Kind: improvement

Priority: P1 — completion requires regression and integration evidence.

Dependencies: LOG-01, LOG-02, HELM-01, DB-01, CLI-01

Primary ownership: verification and these task documents only.

Requirements and acceptance criteria: all checks below pass; record actual
results, preserve historical evidence, and keep unverified requirements blocked.

## Shared verification and definition of done

All commands run from `<repo-root>` with the declared Go/pnpm toolchain. Replace
the database URL placeholder with an isolated disposable PostgreSQL database.

- Focused: `go test -count=1 ./internal/guardrails ./internal/httpapi ./internal/dbmerge ./internal/configedit ./cmd/aiproxy`
- Full: `make vet test`, `make test-race`, `make docs-contract`, `make helm-test`
- Database: `AIPROXY_TEST_DATABASE_URL='<disposable-postgres-url>' go test -race -p 1 -count=1 ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app`
- Frontend: `pnpm --filter @aiproxy/web-ui typecheck` and
  `pnpm --filter @aiproxy/web-ui test`
- Artifact: `make build` and actual Helm lint/render (relay off/on).
- Done: all six tasks completed with regression evidence, docs agree with
  behavior, unrelated changes retained, and no local paths/session identifiers
  recorded here. Live Kubernetes/CNI verification remains outside this task.

## Completion evidence

- Permanent regression tests were added before the fixes and failed on the
  original working tree for all six findings (eight top-level failing tests).
- Changed: JSON-aware redaction preserves numbers, escapes custom placeholders,
  masks original requests, and fails closed on unmatched/deep/colliding rewrites.
  Full request-local secrets are used rather than truncated snippets. Capture
  completeness gates exception clearance, independently of logging/quarantine;
  exact-cap captures remain complete and audit fallback remains non-blocking.
- Changed: API/relay ingress has no allow rule without peers. The explicit,
  default-off `networkPolicy.relay.allowApiUpstream` restricts relay egress and
  API ingress to same-release pods on the API port, without exposing relay ingress.
  Parent-template tests need no downloaded subcharts; YAML's existing module
  version was promoted to a direct test dependency without version changes.
- Changed: derived admin writes reject true/false declarations before persistence,
  stored true overrides reject catalog build, concrete-to-derived transitions
  clear local flags, and CLI logging edits preserve declared values.
- Regression coverage: `internal/httpapi/guardrail_redaction_test.go`,
  `internal/guardrails/quarantine_test.go`, `helm/main/networkpolicy_test.go`,
  `internal/httpapi/admin_providers_insecure_test.go`,
  `internal/dbmerge/allow_insecure_base_url_test.go`,
  `internal/store/allow_insecure_base_url_test.go`,
  `cmd/aiproxy/payloadlog_redaction_test.go`, and configedit rendering tests.
- Verified from `<repo-root>` against the final working-tree code:
  - `make vet test` and `make test-race` — pass; rerun after final test/module edits.
  - `make helm-test` — pass with Helm 4.3.1; actual chart lint/render also passes.
  - `make docs-contract` — documentation matrices match.
  - Web UI typecheck — pass; web UI tests — 11 files, 211 tests passed.
  - `make build` — full UI build and host binary build pass.
  - `go mod tidy -diff`, touched-file gofmt, Prettier checks, and
    `git diff --check` — clean.
- PostgreSQL 17 verification used an isolated disposable test database:
  `go test -race -p 1 -count=1 ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app`
  passed all four packages. Additional legacy-migration and durable-transition
  regressions were then added and passed with the race detector in focused
  store/dbmerge/HTTP runs, alongside concrete toggles and derived rejection/inheritance.
  The legacy migration preserves provider identity/fields, defaults the opt-in to
  false, persists a later true value, and is idempotent; its destructive fixture
  operations are restricted to the helper's fresh isolated schema.
- Docs explain the NetworkPolicy upgrade behavior, explicit connection opt-in,
  redaction limits, audit fallback, and derived-provider transitions. Original
  INSECURE-02/03/06 are reclosed with fresh evidence, preserving their history.
- Limitations: no live Kubernetes/CNI or real-provider compatibility test was
  performed; these remain outside the remediation scope. Unrelated working-tree
  changes were retained, and no commit or deployment was performed.
