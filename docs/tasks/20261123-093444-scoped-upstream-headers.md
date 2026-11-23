# Scoped Fixed Upstream Headers

Created: 2026-11-23 09:34:44 (local time)

Status: completed (UHDR-01–UHDR-03; integration verified)

## Objective and Scope

Let operators set fixed outbound HTTP headers without requiring the caller to
supply them. Add `upstream_headers` to root, provider, model, and alias scopes
in static HCL/JSON config, including operator inline config. This is an opt-in
improvement; it does not change `forward_headers` or add a credential mode.

```hcl
upstream_headers = {
  "X-App-Name" = "my-app"
}
```

## Header Resolution Contract

| Route                       | Highest to lowest priority, per header name                                                  |
| --------------------------- | -------------------------------------------------------------------------------------------- |
| Direct model                | Model → provider → root → permitted caller value                                             |
| Alias                       | Alias → selected target's model → selected target's provider → root → permitted caller value |
| Static model-ID passthrough | Provider → root → permitted caller value                                                     |

- Match header names case-insensitively. A fixed value is sent once; it replaces
  all caller values for that name, even when `forward_headers` permits it.
  If no fixed value exists, keep current caller-forwarding behavior. For example,
  a caller's `X-App-Name: client` loses to an alias's `x-app-name: public`.
- Merge individual names, not entire maps. Omission and `{}` both mean no
  local overrides; unrelated lower-layer names survive. No deletion marker in v1.
- Proxy-managed authentication, HTTP framing, and adapter-owned headers are not
  configurable via this map. Existing credential, `User-Agent`, and forwarding
  options continue to control their dedicated headers.
- Resolve after routing and authorization. On alias failover, recompute from the
  new target using the admitted config snapshot; retain the alias overrides but
  no previous target's provider/model values. Same-target reasoning recovery uses
  the same fixed values. Do not change which requests retry or where they route.

### First-release boundaries

- Only inference HTTP requests to static targets receive fixed headers. No probes,
  model discovery, or OAuth/device requests. Do not send fixed values in outgoing
  deferred peer control headers **or** submit metadata: a peer can log submitted
  headers without knowing their origin. A receiver's own local inference request
  to a static upstream uses its local scopes and existing execution limits.
- No admin API, dashboard, database storage, or new CLI editing controls. No
  fixed headers at any scope reach a database-managed provider (System included),
  even via a static alias or via inheritance from a static base. A managed alias
  supplies no alias overrides; an otherwise-authorized static destination still
  uses its own root/provider/model scopes. Do not change workspace authorization.
- Treat fixed values as potentially sensitive. Static `extends` inherits the
  endpoint and model definitions, not the base provider's or models' fixed maps.
  Static derivatives can declare their own provider map and local existing-name
  model overlays; root and alias still apply.
  Use root/alias values only when safe to send to every eligible target, including
  fallbacks. Prefer HTTPS for sensitive values and use the dedicated credential
  mechanism when applicable.

## Analysis Coverage and Baseline

- `internal/config/schema.go` (`rawFile`, `rawProvider`, `rawModel`, `rawAlias`)
  has no arbitrary fixed-header map. `internal/config/validate.go`
  (`ValidateForwardHeaders`) reserves proxy-managed names. The existing scoped
  [encrypted-reasoning task](20261116-190846-scoped-encrypted-reasoning-policy.md)
  is useful for locating the four layers, but replaces whole policies, not names.
- `internal/httpapi/dispatch.go` (`dispatchDirect`, `dispatchAlias`) creates
  `provider.Request` after selecting the actual target. `internal/provider/provider.go`
  (`executeUpstream`, `applyForwardedHeaders`) currently copies allowed caller
  fields without overwriting the adapter's headers. The receiver's bounded path
  is `executeUpstreamLimited` in `internal/provider/execution_limits.go`.
- `internal/provider/deferred.go` (`doDeferred`, `deferredInferenceHeaders`)
  packages allowed caller metadata for peers; `prepareDeferredExecution` in
  `internal/httpapi/deferred_execution.go` can log that input. Local outbound
  captures use `payloadUpstreamRequest` in `internal/httpapi/handler.go` and
  `RedactHeaders` in `internal/payloadlog/payloadlog.go`, which redacts known
  names rather than all custom fields.
- `internal/upstreamhttp/redirect.go` already limits redirects to the original
  origin and forbids peer-control redirects. Preserve the guard from the
  [upstream and CLI health review](20260926-163550-upstream-and-cli-health-review.md).
  OpenRouter's existing built-in attribution in `internal/provider/openai.go`
  is adapter-owned, not an existing generic setting.
- `internal/dbmerge/dbmerge.go` (`cloneProvider`, `PrepareProvider`) copies
  static base settings for managed derivatives. `internal/config/convert.go`
  (`writeHCLBody`) currently interprets JSON objects as blocks by default; the
  new string map must round-trip as an attribute instead.

Baseline: source and related task files inspected; no runtime tests or live
upstream calls run during planning. Test commands below are implementation checks.
Before implementation, `make check-toolchain` and `make docs-contract` passed
from the repo root. The default environment has no `AIPROXY_TEST_DATABASE_URL`.

## Implementation Tasks

### Task UHDR-01: Parse, validate, and isolate static header declarations

Status: completed (UHDR-01 implementation session)

Kind: improvement

Priority: P2 — opt-in feature; validation is required before wiring outbound I/O.

Dependencies: none

Primary ownership: `internal/config/{schema,load,build,validate,types,catalog}.go`,
`internal/dbmerge/dbmerge.go`, and focused tests. See the schema, validation,
and managed materialization references above.

Requirements:

1. Parse a strict map of strings in HCL and JSON. Reject non-string, null, or
   blank values; invalid HTTP field names; controls (including CR/LF); and
   duplicate names within one map, including case variants. Check duplicate
   declarations before decoding can collapse them. Validate disabled declarations
   too, without printing values in diagnostics.
2. Reject the proxy-managed names already reserved by `forward_headers`,
   `Accept-Encoding`, private deferred headers, and known adapter-owned names
   on applicable targets. Keep dedicated settings as the only way to change
   their headers. Bound declarations and resolved sets to 32 names and 16 KiB
   of name/value bytes, with at most 128 bytes per name and 4096 per value.
   Reject an over-budget candidate rather than truncating it.
3. Preserve local intent and immutable catalog snapshots. Add only the
   existing-name static model-overlay allowance needed for this map; keep all
   unrelated `extends` restrictions. Ensure database materialization does not
   copy the static maps, including through inherited model indexes.

Acceptance criteria: Four-scope declarations load in both formats. Tests cover
mixed-case merging, malformed and reserved fields, duplicates, size boundaries,
static derivative isolation, managed inheritance isolation, and snapshot copies
that cannot mutate serving configuration.

Verification: `go test ./internal/config ./internal/dbmerge` from the repo root.

Completion evidence:

- `internal/config/upstream_headers{,_syntax}.go` now normalize and validate
  declarations; the catalog resolves per-target limits and isolates static and
  managed derivation. Tests cover HCL/JSON, malformed and duplicate inputs,
  reserved names, bounds, inheritance, and copy isolation.
- `go test ./internal/config ./internal/dbmerge` and `make vet test` passed in
  the implementation session. The coordinator reviewed the changes and reran
  `go test -count=1 ./internal/config ./internal/dbmerge`: passed.
- `go test -race -p 1 -count=1 ./internal/config ./internal/dbmerge` passed
  with an isolated loopback PostgreSQL 17 fixture.

### Task UHDR-02: Apply fixed headers at inference dispatch and redact captures

Status: completed (UHDR-02 implementation session)

Kind: improvement

Priority: P2 — delivers the feature and prevents disclosure across routes.

Dependencies: UHDR-01

Primary ownership: `internal/httpapi/{handler,dispatch}.go`, `internal/provider/`,
and `internal/payloadlog/` plus HTTP/peer regression tests. See the dispatcher,
execution, redaction, and redirect references above.

Requirements:

1. Feed the effective fixed set for each attempt into the shared inference
   preparation path. Keep adapter headers authoritative as a runtime safeguard;
   apply the remaining fixed values before forwarding or snapshotting. Preserve
   outgoing deferred peer controls and submit metadata as they are today.
2. Redact all applied fixed values in upstream request payload logs, including
   receiver-side bounded inference captures, regardless of header name. Keep
   the wire value unchanged; do not add raw values to errors, metrics, or
   administrative views. Do not assume a trusted upstream will redact on its end.
3. Reject attempts to configure `upstream_headers` via admin provider/model,
   alias, or credential writes. Keep strict allowlists and reject this field in
   the legacy credential decoder without changing handling of other unknown
   fields (`adminCredentialUpsert.UnmarshalJSON` in `internal/httpapi/admin_providers.go`).

Acceptance criteria: Mock upstreams see exactly one configured value despite
repeated, mixed-case caller input, and see allowed caller values for names with
no fixed override. Direct, alias, failover, passthrough, and receiver-side tests
show the correct scopes; include streaming and a non-JSON request path. A
two-upstream failover test proves the first target's private field is not reused.
Admin writes cannot add maps, managed destinations and peer submissions receive
none, and a cross-origin redirect receives no request. Captured logs mask the
test secret while the eligible upstream receives it.

Verification: `go test ./internal/provider ./internal/httpapi ./internal/payloadlog ./internal/e2e`
from the repo root, using mock upstreams and peers.

Completion evidence:

- `internal/modelresolver/resolve.go` pins the root map to the resolver snapshot;
  `internal/httpapi/dispatch.go` resolves the map per target. The shared send
  path in `internal/provider/provider.go` applies fixed values before caller
  forwarding and masks them in cloned snapshots. The bounded receiver and
  admin credential decoder are covered; the outgoing peer path is unchanged.
- `go test ./internal/modelresolver ./internal/provider ./internal/httpapi ./internal/payloadlog ./internal/e2e`
  and `make vet test` passed in the implementation session. The coordinator
  reviewed the changes and reran the focused feature and redirect race tests
  with isolated PostgreSQL 17; all passed, including the admin persistence/view check.

### Task UHDR-03: Preserve declarations through config tooling and verify reload

Status: completed (UHDR-03 implementation session)

Kind: improvement

Priority: P2 — prevents config loss and documents routing and trust boundaries.

Dependencies: UHDR-02

Primary ownership: `internal/config/convert.go`, `internal/configedit/`,
`cmd/aiproxy/configure.go`, reload tests in `internal/app/`,
`website/docs/configuration.md`, and `docs/design.md`. See `writeHCLBody`,
`RenderProviderBlock`, `RenderAliasBlock`, and `App.Reload`.

Requirements:

1. Round-trip the map through HCL↔JSON as an attribute, not a block. Validate
   before converting so duplicate keys cannot disappear. Preserve map declarations
   and `env()` spelling through supported in-place edits to unrelated settings,
   without displaying fixed values in CLI previews. Explicit conversion follows
   existing `env()` resolution; document that outputs can contain secrets.
2. Document the precedence table, caller conflict, derivative and peer limits,
   managed-provider exclusion, and trusted-upstream requirement with load-tested
   HCL/JSON examples. Reload all scopes atomically via the existing `SIGHUP` path.

Acceptance criteria: Conversion and unrelated provider/model/alias edits preserve
declared intent. A successful reload affects new admissions; an invalid candidate
leaves the old values intact. An admitted request that fails over after reload
still resolves its next target from the original snapshot. Docs and tests agree
that health probes and upstream discovery do not send fixed values.

Verification: `go test ./internal/config ./internal/configedit ./internal/app ./cmd/aiproxy`
from the repo root, including the documented configuration fixtures.

Completion evidence:

- Config conversion preserves the map attribute at all four scopes and rejects
  duplicate declarations or conversion loss. HCL edits preserve raw maps on
  retained scopes, including derived header-only overlays; previews mask them.
- `internal/app/upstream_headers_reload_test.go` proves activation, rollback,
  old-snapshot alias failover, and probe exclusion. CLI discovery tests assert
  the same exclusion. `TestDocumentedFixedUpstreamHeaders` loads and converts
  the exact public HCL/JSON examples.
- `go test ./internal/config ./internal/configedit ./internal/app ./cmd/aiproxy`,
  the focused `-race` tests for `UpstreamHeader|FixedHeader` in those packages,
  and `make docs-contract` passed in the implementation session.

## Definition of Done

All three tasks meet their acceptance criteria and record verification evidence.
From the repo root, run `make vet test docs-contract` and `make integration` with
the Go toolchain specified by `go.mod` and loopback access for test servers. No
live credentials or provider calls are needed. Behavior with `upstream_headers`
omitted must remain unchanged.

### Integration review

The coordinator reviewed each task before starting the next, then checked the
combined config, inference, redaction, tooling, and reload paths. From the repo
root, `make vet test docs-contract`, `make integration`, and `pnpm docs:build`
all passed. A separate smoke check of `dist/aiproxy` against a loopback stub
confirmed that fixed values win over caller values on direct and alias requests,
that a valid `SIGHUP` activates new values, and that a rejected reload keeps
the previous values. Changed Go files passed formatting checks. PostgreSQL was
used for the focused checks above; no live upstream or provider credentials
were used.
