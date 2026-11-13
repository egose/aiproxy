# Upstream TLS ClientHello profiles and Bun capture tooling

Created: 2026-11-12 17:39:48 PST

Status: completed; TLS-01 through TLS-09 delivered and independently accepted with required verification.

## Objective

Add opt-in upstream TLS ClientHello profiles using
`github.com/refraction-networking/utls`, including built-in browser profiles and
an explicitly supplied custom specification. Provide a local capture/export
workflow that can derive a custom profile from an installed Bun runtime rather
than guessing that a Chrome preset resembles Bun.

The feature changes the observable ClientHello structure. It does not replace
Go's networking stack with BoringSSL, reproduce an entire browser, or guarantee
that a WAF or bot detector will accept a request.

This document originally covered the requested task-file-first deliverable.
The user subsequently authorized implementation by subagents. Execution follows
the dependencies and acceptance criteria below.

## Proposed configuration contract

The following attributes and values are proposals, not existing aiproxy options.
Finalize the implementation decisions in TLS-01 before changing public contracts.

```hcl
provider "openai-compatible" "example" {
  base_url = "https://api.example.test/v1"
  api_key  = env("UPSTREAM_API_KEY")

  tls_client_hello           = "custom"
  tls_client_hello_spec_file = "tls/bun-clienthello.json"

  model "example-model" {}
}
```

The profile path above is illustrative, not a bundled Bun profile. Its resolution
rules must be settled in TLS-01.

| Candidate value          | Intended behavior                                                                  |
| ------------------------ | ---------------------------------------------------------------------------------- |
| `native`                 | Default; preserve the standard Go TLS transport.                                   |
| `chrome`                 | Resolve to the recommended Chrome preset bundled with the pinned uTLS dependency.  |
| `firefox`                | Resolve to the corresponding bundled Firefox preset.                               |
| `safari`                 | Resolve to the corresponding bundled Safari preset.                                |
| Explicit versioned names | Map only whitelisted names to tested presets supported by the pinned dependency.   |
| `android-11-okhttp`      | Optional legacy-client preset, subject to compatibility review.                    |
| `custom`                 | Require a supplied, validated specification; do not detect a client automatically. |

Names discussed previously, such as `chrome-155`, `firefox-148`, and
`safari-26.3`, are candidates observed in upstream source, not promises that a
selected released dependency supports them. Browser aliases do not download
current fingerprints at runtime.

Do not add a generic `boringssl` value or make `bun` an alias of `chrome`.
BoringSSL is a TLS implementation, not a universal fingerprint. A Bun-derived
profile must identify the runtime version, networking API, and relevant TLS
options that produced the capture.

Defer `randomized-alpn` and `randomized-no-alpn` by default: neither provides Bun
fidelity, and their negotiation/stability behavior needs separate validation.
Legacy iOS/Edge profiles are not implicitly included either. TLS-01 may add
explicitly tested candidates or leave them out with a recorded rationale.

### Custom specification contents

Prefer the pinned uTLS dependency's existing `ClientHelloSpec` JSON format rather
than introducing an unrelated recipe format. Validate inputs before invoking its
decoder and record any deliberately unsupported subset.

| Field                  | Meaning                                                                                |
| ---------------------- | -------------------------------------------------------------------------------------- |
| `cipher_suites`        | Ordered cipher-suite names; GREASE placeholders only when appropriate for the profile. |
| `compression_methods`  | TLS compression methods, normally `["NULL"]`.                                          |
| `extensions`           | Ordered extension objects and their extension-specific settings.                       |
| `min_vers`, `max_vers` | Optional version bounds; `771` is TLS 1.2 and `772` is TLS 1.3.                        |

Relevant extension settings include `named_group_list`,
`ec_point_format_list`, `supported_signature_algorithms`, `versions`,
`protocol_name_list`, and `client_shares`. Padding, GREASE, certificate
compression, and session-related extensions must reflect supported behavior, not
be added simply because a browser uses them.

Array order is significant. The upstream hostname supplies SNI, and each
connection gets fresh randomness and cryptographic key material. Neither a
filename nor a JA3/JA4 hash contains enough information to define a profile.

## Scope, non-goals, and working rules

- In scope: provider configuration, strict custom-file loading, immutable runtime
  profiles, profile-aware transport pooling, direct and alias inference,
  HTTP/1.1 and HTTP/2 negotiation, configured environment proxies, reload,
  provider health probes, upstream model discovery, configuration editing,
  managed-provider contract propagation, documentation, and hermetic tests.
- In scope: an optional operator/developer capture utility using installed Bun;
  Bun is not a dependency of the serving binary or mandatory fixture-based CI.
- Recommended initial scope: provider-only settings with `native` as the default;
  derived providers inherit transport settings with their base endpoint. Confirm
  explicit-declaration rules in TLS-01 instead of inventing local overrides.
- Non-goals: full browser/Bun impersonation, arbitrary subsequent handshake or
  TCP packet rewriting, HTTP header or HTTP/2 fingerprint impersonation,
  JavaScript challenges, guaranteed WAF bypass, and runtime browser catalog sync.
- Do not change authentication, credential forwarding, redirect boundaries,
  retry/alias cooldown policy, provider capability matrices, or plaintext URL
  validation as a side effect. OAuth/device login and dashboard clients remain
  outside provider transport configuration.
- Do not disable certificate/hostname verification, enable weak ciphers, silently
  fall back to a different profile, or cycle fingerprints in response to blocks.
- Capture tooling must be local-only by default, require no privileged packet
  capture, and send no provider credentials or application traffic externally.
- Follow `AGENTS.md`, including the source-comment convention and CGO-disabled
  builds. Preserve unrelated worktree changes.
- Shared hotspots are `go.mod`, `internal/config`, the new profile package,
  `internal/upstreamhttp`, and `internal/app`. Sequence dependent changes rather
  than editing these concurrently. The coordinator alone updates this task file;
  use task-local role aliases rather than session identifiers in shared evidence.

## Analysis coverage and limitations

Analyzed revision: `eafc7dd`, with unrelated existing changes in `.tool-versions`,
`helm/main/Chart.lock`, and `helm/main/Chart.yaml`. Do not revert or overwrite them.

Inspected configuration schemas/builders/validators and catalog cloning,
application transport pooling and reload, direct/alias client selection,
health-probe and model-discovery client creation, configuration rendering,
provider storage/dbmerge/admin views, web provider types, and verification scripts.

Confirmed current behavior:

- `go.mod` declares Go 1.27 and has no uTLS dependency.
- `internal/app/app.go:728-767` creates upstream clients keyed only by header
  timeout and clones the standard `http.DefaultTransport`.
- `internal/app/app_test.go:1086-1114` pins zero whole-request timeout for
  inference and reuse by timeout.
- `internal/httpapi/dispatch.go` selects clients through `clientForProvider` for
  both direct requests and each alias attempt.
- `internal/healthcheck/healthcheck.go:51-64,271-296` uses a separate client.
- `cmd/aiproxy/models_upstream.go:42-53,128-134` creates another client with its
  own operation timeout behavior.
- `internal/config/load.go:51-73` receives the source filename but calls
  `buildRuntime` without source-location context; custom-file resolution needs
  an explicit design, including inline `AIPROXY_CONFIG` and conversion.
- Existing config/provider/store/API/UI types have no ClientHello setting.
- Upstream uTLS documentation and JSON decoder/tests were inspected; browser
  parroting caveats and the capture-to-spec API are documented there. Upstream
  master references below are evidence, not a dependency pin.

No existing TLS-profile task was found by searching `docs` for uTLS,
ClientHello, `tls_client_hello`, BoringSSL, or Bun-runtime references. Related
completed work provides conventions, not coverage for this new feature:

- [Upstream header timeout](20260809-132640-configurable-upstream-header-timeout.md)
  for pooling, streaming, proxy, and reload constraints.
- [Provider inheritance](20260822-160713-provider-inheritance.md) for derived-provider rules.
- [Plain-HTTP provider opt-in](20261111-180545-provider-allow-insecure-base-url.md)
  for configuration/store/API/UI propagation patterns.

Limitations: no Bun runtime probe/capture, uTLS transport prototype, live upstream
request, or baseline Go/UI test suite was run while creating this plan. The full
provider adapter implementation and every management route were not audited.
Questions whose answers affect implementation are assigned to TLS-01, not
presented as confirmed defects.

Upstream references:

- [uTLS README: ClientHello scope, compatibility caveats, and fingerprinting](https://github.com/refraction-networking/utls/blob/master/README.md)
- [ClientHello IDs and specifications](https://github.com/refraction-networking/utls/blob/master/u_common.go)
- [JSON decoder](https://github.com/refraction-networking/utls/blob/master/u_clienthello_json.go)
- [JSON fixture illustrating the schema, not a Bun profile](https://github.com/refraction-networking/utls/blob/master/testdata/ClientHello-JSON-Chrome102.json)

## Priorities and execution order

- P1: required for a correct, secure, documented feature or a blocking design decision.
- P2: operator tooling/convenience required by this objective, after the serving contract is stable.

Start with TLS-01. Keep every task pending until its dependencies are completed.
Update this file with decisions and evidence as implementation proceeds; do not
claim independent review unless an independent reviewer actually participates.

Delegation plan (allocation is not completion evidence):

| Tasks          | Role alias               | Sequencing                                                          |
| -------------- | ------------------------ | ------------------------------------------------------------------- |
| TLS-01         | TLS design agent         | First; establishes the shared contract.                             |
| TLS-02         | TLS profile/config agent | After TLS-01; owns shared profile/config APIs.                      |
| TLS-03, TLS-04 | TLS transport agent      | After TLS-02; complete TLS-03 before alternate outbound paths.      |
| TLS-05         | TLS management agent     | After TLS-02; owns CLI/store/API/UI changes.                        |
| TLS-06         | Bun capture agent        | After TLS-02; separate helper/export files from management changes. |
| TLS-07         | TLS documentation agent  | After implementation and local capture evidence.                    |
| TLS-08         | TLS independent reviewer | Separate from implementers; coordinate checks with the integrator.  |

The coordinator reconciles delivered changes, resolves integration issues, and
records actual verification. Do not run builds or database suites concurrently
against shared outputs or the same disposable database.

Execution update: the separate design and profile/config agents delivered TLS-01
and TLS-02. Transport, management, and Bun capture agents then started in parallel
with non-overlapping ownership. An additional read-only early core audit reviews
completed TLS-02; it is supporting review, not premature completion of TLS-08.
Final cross-path review remains gated on all implementation tasks.

Dependency refinement after TLS-01: TLS-05 and TLS-06 can start once TLS-02's
profile/config API is delivered. Management uses that API rather than transport
internals; capture/export can materialize profiles directly against local
listeners. Their files are separate from TLS-03/04. Integrated on-wire activation
and serving verification remain mandatory in TLS-08; this refinement does not
remove any transport acceptance criterion or permit shared-file races.

Final-wave refinement: after TLS-01 through TLS-06 close, a separate binary
integration agent can add TLS-specific hermetic tests while TLS-07 edits public
Markdown. TLS-08 starts with those independent code-verification activities;
its final review/closure still requires completed TLS-07 and docs consistency.
The independent reviewer did not implement production code or the new binary
tests and reviews both. Shared rebuilds/database checks remain serialized.

## Tasks

### Task TLS-01: Resolve the dependency, transport, and configuration contract

Status: completed

Assigned role: TLS design agent

Kind: investigation

Priority: P1 — profile support and HTTP transport integration determine whether the proposed contract can be delivered honestly.

Dependencies: none

Primary ownership:

- This document's decision record.
- Bounded experiments covering `internal/upstreamhttp` and `internal/app`.
- Dependency/release and uTLS JSON/capture compatibility assessment.

Finding:

uTLS profiles are version-dependent and `UConn` is not a drop-in promise of
standard Go HTTP/2 integration. A custom TLS dial hook also needs investigation
for the upstream TLS connection after proxy CONNECT. The current code provides
no profile-loading or custom-file trust policy.

References:

- `go.mod`, `Makefile:27-35` (Go version and CGO-disabled serving build).
- `internal/app/app.go:728-767` (`upstreamClientPool`, `newHTTPClient`).
- `internal/config/load.go:26-73` (file/inline loading and source filename).
- `internal/config/build.go:240-329` (derived-provider restrictions).
- Upstream references listed in the analysis section.

Requirements:

1. Select a maintained, pinned uTLS version compatible with the declared toolchain,
   release targets, license policy, and CGO-disabled builds. Record the actual
   available/tested presets and reject unsupported version names.
2. Use a bounded local prototype to choose an HTTP transport architecture that
   handles ALPN, HTTP/1.1, HTTP/2, HTTP and HTTPS proxy CONNECT, cancellation,
   streaming, and idle cleanup. Distinguish upstream TLS from TLS to a proxy;
   do not claim standard `DialTLSContext` alone covers all paths.
3. Finalize attribute scope, inheritance, profile-file resolution, behavior for
   inline config and relocated conversion output, disabled/plain-HTTP providers,
   supported ALPN/extension subsets, and fixed input bounds.
4. Decide the managed-provider custom-profile representation and authorization
   policy. Recommended initial boundary: built-in profiles in database/API/UI;
   custom filesystem references remain operator-owned static configuration
   unless a safe inline/registered-profile design is explicitly approved.
5. Choose the capture tool's invocation/location and initial Bun API coverage.
   Keep native uTLS-spec JSON and capture metadata separate; do not assume a
   captured Go spec automatically marshals into the accepted JSON schema.

Acceptance criteria:

- The decision record names the exact dependency version, allowlist, transport
  architecture, source-resolution rules, inheritance rules, and input bounds.
- Local evidence establishes the supported HTTP/ALPN/proxy combinations; any
  unsupported combination has an explicit rejection/defer recommendation, not
  an unadvertised native or HTTP/1.1 fallback.
- The custom-profile API/file-access boundary and capture tool contract are
  settled, and downstream requirements are updated to match those decisions.

Verification: evidence review of the pinned upstream APIs and bounded hermetic
prototype results. Record actual commands and outcomes; production implementation
and live-provider networking are not prerequisites for this investigation.

Resolution and completion evidence:

- TLS design agent delivered a separate isolated prototype and evidence-backed
  handoff. Coordinator inspected the handoff/artifacts and confirmed that no
  repository source or dependency files changed during the investigation.
- Pin uTLS v1.8.2, release commit `8fe0b08e9a0e7e2d08b268f451f2c79962e6acd0`,
  with BSD-3-Clause attribution. Final contract and safety restrictions are in the
  decision record below; earlier candidate values remain historical proposals.
- Isolated `go test -mod=readonly -count=1 -timeout=60s ./...` passed (5.974s),
  `go test -mod=readonly -race -count=1 -timeout=60s ./...` passed (8.601s),
  `go vet -mod=readonly ./...` and `go mod verify` passed. Working directory was
  the isolated prototype, not `<repo-root>`; installed Bun was supplied to tests.
- 54 ordinary browser route cases passed; 36 apply to the shipping Chrome/Safari
  subset. Additional custom ALPN, cancellation/SSE, certificate, malformed input,
  weak-negotiation and fragmentation cases passed. Bare-UConn HTTP/2, stock-proxy
  hook bypass, bare JSON decoder, and unguarded SHA-1 counterexamples were reproduced.
- Installed Bun 1.4.4 `fetch` generated a 1500-byte initial ClientHello. Three
  fresh Bun/uTLS captures matched after normalizing only random, session-ID, and
  key-exchange bytes; verified TLS 1.2/1.3 replay passed over all three TLS routes.
- Dependency/prototype test binaries compiled with CGO disabled for all five
  targets in `go-release.json`. This is not production release verification.
- Firefox 120 is deferred because selecting its second offered P-256 share
  reproduced upstream issue #402. No live provider or WAF acceptance was tested.

### Task TLS-02: Add validated profile configuration and custom-spec loading

Status: completed

Assigned role: TLS profile/config agent

Kind: improvement

Priority: P1 — turn profile selection into an explicit, bounded runtime contract before changing any transport.

Dependencies: TLS-01

Primary ownership:

- `go.mod`, `go.sum`.
- `THIRD-PARTY-NOTICES` (new dependency attribution).
- `internal/config/{schema,types,load,build,validate,dynamic,catalog,convert}.go` and focused tests.
- `internal/tlsprofile/` (new registry, decoder, validation, immutable descriptor, and fixtures).

Finding:

Static and dynamic providers have no TLS profile fields. Custom specifications
contain mutable extension objects and cannot be shared indiscriminately between
catalog snapshots or concurrent uTLS connections.

References:

- `internal/config/schema.go:129-147`, `internal/config/types.go:236-256`.
- `internal/config/catalog.go:122-149` (`cloneProvider`).
- `internal/config/dynamic.go:63-73,109-137` (defaults and dynamic validation).
- `internal/config/convert.go:49-94` (conversion revalidation).

Requirements:

1. Add the finalized provider settings with omission resolving to `native`.
   Implement a canonical allowlist mapping aliases to concrete bundled profiles;
   enforce the same selection rules through HCL, JSON, and dynamic validation.
2. Load custom files during candidate configuration construction/validation, not
   on requests. Require a file for `custom`, reject an irrelevant file for a
   built-in/native profile, and honor TLS-01's source-resolution contract.
3. Strictly validate required arrays, names, versions, extension payloads, protocol
   compatibility, trailing/unknown data, and documented size/count limits before
   invoking uTLS. Produce provider/field-specific errors without raw file data,
   panics, weak-cipher enabling, or unsupported-feature masquerading.
4. Store an immutable profile identity/content digest. Materialize fresh mutable
   specifications per connection; retain extension/list ordering and let uTLS
   generate cryptographic values. Make catalog and derived-provider cloning safe.
5. Test the finalized explicit-declaration rules on both HCL and JSON derived
   providers, including explicit default values; do not silently ignore them.
6. Implement the decision record's strict v1 subset, including the private
   X25519MLKEM768 symbol adapter, disabled-provider validation, source-less
   absolute-reference rule, conversion-to-resolved-reference semantics, and
   nonblocking regular-file opening. Bare uTLS decoding is not sufficient.

Acceptance criteria:

- Omitted settings preserve native behavior; every supported name resolves to the
  documented preset and unknown/version-mismatched names fail validation.
- Missing, unreadable, malformed, oversized, unsupported, and incompatible
  custom files fail in a controlled manner before runtime activation.
- A valid custom recipe loads through the supported file/inline/conversion paths;
  no captured hostname, private key, fixed real key share, or session secret is
  replayed as connection configuration.
- Repeated/concurrent profile materialization and catalog copies cannot mutate
  each other's extensions or effective profile identities.

Verification: `go test ./internal/config/...` plus tests for the new profile
package after it exists; include table-driven boundary tests and fixture-based
parser/materialization checks. All commands run from `<repo-root>`.

Completion evidence:

- TLS profile/config agent implemented the dependency pin/notice, strict immutable
  profile registry/decoder and bounded source loading, HCL/JSON/derived/disabled
  validation, conversion reference preservation, and focused tests/fixtures.
- Profile APIs: `Native`, `ResolveBuiltin`, `ParseCustom`, `DecodeSpec`,
  `BuiltinSelectors`; immutable `Profile` exposes native/custom tests, identity,
  canonical name/JSON copy, and fresh `Materialize`. Native materialization fails
  deliberately because its caller must use the standard Go transport.
- Config APIs: `ValidateProviderTLSProfiles(src, filename)` validates only TLS
  settings without staged credential access; `ResolveDynamicProviderTLSProfile`
  resolves built-in-only managed settings without file I/O. Provider metadata
  tracks explicit declarations, inheritance, and the effective immutable profile.
- Agent verification passed: focused normal/race tests and vet, `make vet test`
  without database environment, `go mod verify`, 3,782 parser fuzz executions,
  and CGO-disabled production compilation for all five release targets. Reported
  focused coverage: profile 95.4%, config 89.3%.
- Coordinator inspected the delivered profile/source APIs and relevant diffs,
  reran `go test -race ./internal/tlsprofile ./internal/config` successfully,
  and verified unchanged hashes for the three unrelated user-modified files.
- Non-Unix/non-Windows custom file access fails closed. Cross-platform runtime
  behavior, transport negotiation, and production integration remain unverified
  by this task; compilation alone is not a runtime claim.

Review reopening:

- A separate read-only core audit reproduced a panic in
  `ValidateProviderTLSProfiles` when duplicate JSON `extends` diagnostics caused
  type validation to be skipped before unchecked `AsString` calls. Duplicate
  root TLS declarations also escaped this TLS-only helper.
- The same audit reproduced inconsistent derived checks for numeric/boolean
  `extends`: gohcl coerces these to strings, but the preflight's string-only check
  allowed local TLS attributes that full loading rejects.
- The original TLS profile/config agent owns correction and negative HCL/JSON
  regressions in `internal/config/load.go` and focused tests. Existing completion
  evidence remains historical; TLS-02 cannot close until these tests and an
  independent recheck pass. Windows runtime and all unfinished transport/management
  paths were excluded from this early audit.
- TLS-03/05/06 started after the original gate passed and can continue against
  the unchanged APIs in non-overlapping files. None may be completed or integrated
  until this reopened dependency is reverified; no new dependent task starts now.

Correction and independent re-verification:

- Original config agent changed only `internal/config/load.go` and new
  `internal/config/tlsprofile_inspection_test.go`. Inspection diagnostics now fail
  closed, TLS-only parse errors are redacted, all string extraction is defensive,
  and `extends` conversion matches gohcl for numeric/boolean values. TLS fields
  remain strict strings and staged-credential behavior/APIs remain unchanged.
- Before-fix regressions reproduced both findings. Updated normal/race config
  tests and config vet passed without database environment.
- The separate read-only auditor independently reran the original reproductions,
  coercion/declaration cases, duplicate-root cases, and staged-credential tests
  against an isolated snapshot. Filtered normal tests passed (0.083s) and race
  tests passed (1.493s), with `-mod=readonly -count=1 -timeout=45s`.
- Auditor reported both findings resolved with no remaining concrete finding in
  this correction. Coordinator inspected the changed source/regressions. TLS-02
  is completed again; the historical reopening/evidence above is retained.

### Task TLS-03: Integrate profile-aware upstream transports, pooling, and reload

Status: completed

Assigned role: TLS transport agent

Kind: improvement

Priority: P1 — selected profiles must apply on the wire without breaking serving, proxy behavior, or isolation.

Dependencies: TLS-02

Primary ownership:

- `internal/upstreamhttp/` transport factory and tests.
- `internal/app/app.go`, `internal/app/app_test.go`, focused reload tests.
- `internal/httpapi/dispatch.go` and direct/alias transport-selection tests where needed.

Finding:

The current pool keys only by timeout and idle cleanup recognizes only
`*http.Transport`. That is insufficient for different profiles or a custom
recipe whose contents change at the same path.

References:

- `internal/app/app.go:305-427` (`Reload`), `430-452` (`Close`).
- `internal/app/app.go:728-767` (pool and transport construction).
- `internal/app/app_test.go:1086-1114` (timeout/pooling invariants).
- `internal/httpapi/dispatch.go` (`dispatchDirect`, alias attempts, `clientForProvider`).
- `internal/upstreamhttp/redirect.go` (existing redirect boundary).

Requirements:

1. Keep the native transport path unchanged by default. Use TLS-01's architecture
   for configured profiles, normal certificate/hostname validation, current
   transport deadlines, and protocol selection matching negotiated ALPN.
2. Key pooled transports by effective timeout and effective profile identity,
   including custom content rather than only the file path. Never mutate shared
   transport/spec state per request and never add automatic profile retries.
3. Preserve environment proxy/NO_PROXY behavior, CONNECT isolation, redirects,
   request cancellation, streamed/multipart bodies, and zero inference
   `http.Client.Timeout`. Do not TLS-wrap plain-HTTP requests.
4. Prepare candidate profiles/transports before activation. Successful reloads
   select new settings for new work without stale incompatible connection reuse;
   failed reloads retain the old runtime and profile bytes even if the file changed.
5. Retire unused transport generations and close idle connections through the
   appropriate interface, while allowing admitted in-flight requests to finish.
   Repeated reloads must not create unbounded retained pools.
6. Use Go 1.27's standard-state connection adapter and manually tunneled route
   transports from TLS-01. Add the bounded read-only server-negotiation guard,
   post-preset TLS version clamp, safe cipher checks, explicit selected-ALPS
   rejection, and disabled resumption/renegotiation described in the decision
   record. Add fragmented/malformed/fuzz and cancellation coverage for the guard.

Acceptance criteria:

- Local TLS captures show distinct selected profiles for providers with the same
  timeout, including each target of an alias failover and custom profiles.
- Valid TLS 1.2/1.3 and HTTP/1.1/HTTP/2 requests succeed across the supported proxy
  matrix; invalid certificates/hostnames fail without bypasses or credential leaks.
- Slow-header, long-SSE, cancellation, connection reuse, redirect, and race tests
  preserve the current serving contracts.
- Changing custom content at an unchanged path takes effect on successful reload;
  malformed replacement content rejects reload and leaves old behavior intact.
- Idle cleanup works for native and new transports; repeated profile changes are
  bounded and do not terminate active streams.

Verification: `go test ./internal/upstreamhttp/... ./internal/app/... ./internal/httpapi/...`
and focused race checks for pooling/reload. Use hermetic TLS servers and proxy
fixtures, not real providers; working directory is `<repo-root>`.

Completion evidence:

- TLS transport agent implemented `NewTransport(Options)`, the Go-1.27 state
  adapter, route-isolated CONNECT handling, strict read-only negotiation guard,
  and `NewRetiringTransport` lifecycle tracking. App generations prebuild clients,
  swap only on successful activation, and retire without retaining historical maps.
- Factory options are immutable profile plus exact `ResponseHeaderTimeout`
  (zero disables); returned transport supports idle cleanup. A retiring wrapper
  must be installed around a fresh transport before use and adds `Retire()`.
  Existing `CheckRedirect`/`Do` and dispatch source remained unchanged.
- Focused upstream/app/HTTP API normal tests, race tests and vet passed. An interim
  default `make vet test` passed. 108 TLS/ALPN/route cases plus certificate/weak
  negotiation/cancellation/multipart/redirect/reload/alias-retirement cases passed.
  Guard fuzzing passed 91,682 executions; reported upstream HTTP coverage 94.1%.
- A reproduced late HTTP/2 dial leak before `GotConn` was corrected and covered
  by retirement tests. Parser bounds remain 18,432-byte records, 64 KiB ServerHello,
  and 4 KiB ServerKeyExchange; certificate payloads are skipped. CONNECT headers
  are capped at 10 MiB and per-profile proxy routes at eight.
- Coordinator inspected factory/guard/retirement/app-generation code and reran
  `go test -race -count=1 ./internal/upstreamhttp ./internal/app` successfully
  (18.223s and 20.833s). Database-backed and binary/build verification remain TLS-08.
- SOCKS, HTTP/2 CONNECT, selected ALPS and resumed handshakes fail explicitly.
  Health/model wiring was not started in this handoff and is assigned separately
  as TLS-04 to the same transport agent. No shared builds/provider calls/commits.

### Task TLS-04: Apply provider profiles to health probes and model discovery

Status: completed

Assigned role: TLS transport agent

Kind: improvement

Priority: P1 — provider-related requests must not accidentally retain the native fingerprint on alternate paths.

Dependencies: TLS-03

Primary ownership:

- `internal/healthcheck/healthcheck.go` and focused tests.
- `cmd/aiproxy/models_upstream.go` and upstream discovery tests.
- `internal/app/app.go` dependency wiring and shared factory interfaces as needed.

Finding:

Health checks and CLI upstream model discovery create clients independently of
the serving pool. An inference-only implementation would not configure those
provider requests consistently.

References:

- `internal/healthcheck/healthcheck.go:51-64,140-155,271-296`.
- `cmd/aiproxy/models_upstream.go:42-53,128-134`.
- `cmd/aiproxy/models_upstream_redirect_test.go` and `internal/upstreamhttp/redirect_test.go`.

Requirements:

1. Reuse the profile-aware transport factory/provider descriptor for both paths;
   avoid duplicate TLS implementations or file reads inside operations.
2. Preserve health-check operation deadlines, discovery aggregate bounds and
   deadlines, credential/header allowlists, and redirect isolation. The zero
   whole-request timeout requirement for inference does not erase these separate
   operation timeout contracts.
3. Apply reload/profile changes to subsequent probes and ensure old worker/client
   resources are cleaned up appropriately. Leave OAuth/device and local dashboard
   clients on their existing transport policies.

Acceptance criteria:

- Capture-based tests confirm configured profiles on inference, health probes,
  and upstream model discovery, including custom and native cases.
- Existing discovery pagination/body/timeout and probe cancellation tests pass.
- Profile reload changes subsequent probes without stale profile reuse or
  unintended changes to provider-health/cooldown semantics.

Verification: `go test ./internal/healthcheck/... ./cmd/aiproxy/... ./internal/app/...`
from `<repo-root>`, plus focused race tests for probe reload/lifecycle changes.

Completion evidence:

- TLS transport agent wired fixed, operation/probe-owned clients through the
  shared factory and added `ProfileForProvider`, which validates cached metadata
  without reopening files and accepts validated static custom profiles.
- Health manager exposes `Prepare(catalog, factory)`, `Prepared.Activate(tracker)`,
  and `Prepared.Close`; candidates reserve the update lock until activation or
  discard. Failed preparation preserves old probes/runtime. Profile identity
  restarts changed probes; unchanged probes retain clients without retaining
  retired inference generations, and status/tracker contracts remain intact.
- Discovery pages share one operation-owned retiring transport and release it
  on every exit. Existing whole-client/aggregate/page/body deadlines remain;
  factory header timeout is zero for these separately bounded operations.
- Focused normal/race health/models/app tests and relevant vet passed without
  database environment. Forty health and 100 discovery TLS matrix cases plus
  cancellation/certificate/pagination/cached-custom/reload/rollback cases passed.
  Reported health coverage is 91.6%; scoped whitespace checks passed.
- Coordinator inspected profile resolution, preparation/lifecycle, discovery and
  app wiring, then reran `go test -race -count=1 ./internal/healthcheck ./cmd/aiproxy ./internal/app`
  successfully (5.031s, 51.039s, 25.461s). Shared binary/build/database verification
  and cross-path final review remain TLS-08; no live providers or commits.

### Task TLS-05: Preserve profile settings across CLI and managed-provider surfaces

Status: completed

Assigned role: TLS management agent

Kind: improvement

Priority: P1 — settings must round-trip safely instead of disappearing during editing, persistence, or activation.

Dependencies: TLS-02

Primary ownership:

- `internal/configedit/configedit.go`, `cmd/aiproxy/configure.go`, and focused tests.
- `internal/store/{models,store}.go`, a new migration, and migration tests.
- `internal/dbmerge/dbmerge.go` and dynamic-catalog tests.
- `internal/httpapi/admin_providers.go` and provider mutation/view tests.
- `web-ui/src/types.ts`, `web-ui/src/pages/providers-page.tsx`, and form tests.

Finding:

Provider input/rendering, database fields, admin upserts/views, and UI types lack
profile settings. Custom file references additionally require a different trust
boundary from ordinary selectable profile names.

References:

- `internal/configedit/configedit.go:56-72,285-376`.
- `cmd/aiproxy/configure.go:4203-4235` (`existingProviderInput`).
- `internal/store/models.go` (`DBProvider`), `internal/store/store.go:15-33` (migrations).
- `internal/dbmerge/dbmerge.go:134-203` (cloning and row construction).
- `internal/httpapi/admin_providers.go:42-67,335-356,383-455,789-835`.
- `web-ui/src/types.ts:274-299` and provider form payload mapping.

Requirements:

1. Add CLI controls corresponding to the finalized fields, with omission/change
   tracking, interactive validation, edit preservation, and clear custom-source
   errors. Preserve unrelated values and the established derived-provider rules.
   Preserve `env()` spelling for the new fields, validate references against the
   actual config source, and avoid full credential resolution before a staged
   secrets update is committed; use the TLS-only resolver from TLS-02.
2. Add safe managed-provider persistence and views for the supported profile
   contract. Migrate existing providers to native behavior without data loss and
   apply settings through dbmerge and normal receiving-instance activation.
3. Enforce TLS-01's custom-source policy before persistence. Do not let arbitrary
   workspace API inputs trigger server-local file reads or expose profile file
   contents/paths. If custom is static-only initially, reject unsupported API
   requests clearly and show that limitation in the UI.
4. Preserve omitted values on partial updates; explicit native/reset and
   concrete-to-derived transitions must handle stale custom references according
   to the finalized contract. Reject prohibited derived declarations before commit
   and reject invalid stored rows instead of silently ignoring their settings.
5. Populate UI choices from the approved allowlist/contract, preserve existing
   admin authorization and saved-versus-activated behavior, and avoid claiming
   cluster-wide activation or automatic Bun detection.

Acceptance criteria:

- Scripted/interactive creation and editing preserve valid profile settings and
  custom-file references across supported HCL/JSON conversion workflows.
- Built-in managed profiles round-trip through create, update, GET, reload, and
  the UI; omission retains the existing selection and explicit native resets it.
- Fresh and upgraded disposable databases preserve old providers with native
  behavior; invalid names/derived overrides/custom-source requests do not commit.
- Tests cover the chosen filesystem-access boundary and prevent profile content
  leakage through responses, validation errors, and views.

Verification: `go test ./internal/configedit/... ./cmd/aiproxy/... ./internal/store/... ./internal/dbmerge/... ./internal/httpapi/...`,
`pnpm --filter @aiproxy/web-ui typecheck`, and `pnpm --filter @aiproxy/web-ui test`
from `<repo-root>`. Use `AIPROXY_TEST_DATABASE_URL` pointing to a disposable
database for migration/persistence tests; skipped database tests are not evidence.

Completion evidence:

- TLS management agent implemented CLI flags/presence handling, source-relative
  custom validation and `env()` preservation, TLS-only staged-secret preflight,
  the empty-default selector migration, built-in-only dbmerge/API behavior,
  forbidden derived/custom declarations, and UI controls/metadata.
- API views expose `tls_client_hello`, `tls_client_hello_effective`,
  `tls_client_hello_inherited`, and `tls_client_hello_static_only` without file
  paths or recipe contents. Provider-type metadata supplies approved built-in
  selectors. Credential-update requests reject TLS declarations too.
- Normal/race configedit+CLI tests passed (32 and 234 top-level tests), as did
  normal/race serial database packages: store 38, dbmerge 17, HTTP API 291.
  Database-backed runs used the isolated PostgreSQL 17 fixture with zero skips.
  Fresh/legacy migrations, partial/reset transitions, no-commit negatives,
  inheritance, relative/converted references, and staged secrets were covered.
- Relevant vet and UI typecheck passed. UI tests passed: 11 files, 218 tests.
  Coordinator independently reran the UI tests (same count), inspected migration,
  dbmerge, API presence/privacy, and rendering changes, and confirmed unchanged
  hashes for unrelated tool/Helm edits.
- An interim broad `make vet test` attempted by this agent while capture code was
  still in progress failed at `scripts/tls-clienthello/TestBunCaptureCancellation`.
  It is not final verification or a baseline claim. The capture owner was notified
  to resolve/verify it before handoff; broad checks remain TLS-08.
- On-wire managed activation is still required by TLS-08. No live-provider calls,
  task-document edits, or commits were made by the management agent.

Review reopening:

- Independent management audit reproduced a P2 credential-route contract
  mismatch: Go JSON decoder-equivalent Unicode case-fold aliases, such as a long
  s in the TLS field prefix, escaped the `strings.ToLower` guard. An authorized
  credential PUT then returned saved success and committed its replacement key;
  canonical ASCII spelling was rejected. No authorization bypass, profile-file
  read, or TLS-profile change occurred.
- Original management agent owns a focused JSON-equivalent prefix guard and
  credential-route no-commit regressions in `internal/httpapi/admin_providers.go`
  and `admin_providers_tls_test.go`. Preserve unrelated unknown-field behavior
  and reject equivalent aliases consistently in provider/credential routes.
- TLS-05 completion evidence above remains historical. Reclose only after
  focused normal/race database tests and independent reproduction recheck pass;
  TLS-07/08 remain gated on this correction.

Correction delivered, independent recheck pending:

- Original management agent changed only the API source and dedicated TLS tests
  to use a rune-based Unicode simple-fold prefix matcher consistently in both
  guards; the allowed selector name retains JSON-equivalent case folding.
- Before-fix regressions had fourteen failing subtests. Updated serial PostgreSQL
  normal/race runs passed twelve top-level tests and 99 pass events with zero
  skips, covering both authorized admin roles, no-save/no-credential-mutation,
  canonical/mixed/long-s aliases, and unrelated-field compatibility.
- Coordinator inspected the shared matcher/guard changes and queued independent
  reproduction recheck with the reviewer. Stable public/TLS/authorization APIs
  remain unchanged; this section does not yet reclose TLS-05.

Independent correction recheck:

- Separate auditor independently verified the original authorized long-s
  credential PUT returns 400, no saved header, and unchanged encrypted credentials
  and revision. Canonical/mixed/Unicode aliases and null/empty declarations were
  rejected for system and workspace administrators; allowed selector aliases and
  unrelated fields remained compatible.
- Selected isolated serial database tests passed normally and under race: six
  top-level tests, 77 pass events each, zero skips. Auditor reported the prior P2
  resolved and no remaining finding in this narrow fix. TLS-05 is completed again;
  earlier reopening/delivery evidence is retained.

### Task TLS-06: Build a local Bun ClientHello capture/export and comparison workflow

Status: completed

Assigned role: Bun capture agent

Kind: improvement

Priority: P2 — obtain evidence-backed Bun profiles without inventing a Bun-to-Chrome mapping.

Dependencies: TLS-02

Primary ownership:

- `scripts/tls-clienthello/` (proposed developer-tool location, finalized in TLS-01).
- Export/comparison helpers and sanitized fixtures in `internal/tlsprofile/`.
- Tool-specific usage instructions and capture metadata fixtures.

Finding:

An installed Bun runtime can emit a measurable ClientHello, but the project has
no capture/export utility or verified Bun profile. uTLS fingerprinting returns a
Go specification; producing accepted, portable JSON is a separate step.

References:

- Upstream README (`Fingerprinter.FingerprintClientHello` expects a full TLS record).
- Upstream JSON decoder and extension-specific schema listed above.
- `scripts/` (current repository tooling location) and TLS-02's accepted file schema.

Requirements:

1. Capture fresh ClientHellos from an explicitly selected installed Bun executable
   and supported API, initially `fetch` unless TLS-01 chooses otherwise. Record
   runtime version, API, capture conditions, and relevant non-secret TLS options
   separately from the loadable specification.
2. Use a bounded loopback listener with read/process deadlines and correct TCP/TLS
   record and handshake reassembly. Prevent inherited proxy settings from sending
   capture traffic externally without modifying the parent environment. Do not
   require certificates, root access, provider credentials, or verification bypass.
3. Convert captures to accepted JSON while preserving structural ordering and
   supported extension data. Replace dynamic SNI/random/key material with runtime
   generation; fail explicitly on unsupported extensions or resumption-dependent
   data rather than exporting a misleading fake profile.
4. Compare multiple fresh Bun captures and generated uTLS captures, normalizing
   legitimate volatile values while checking suite/extension order, groups,
   signature schemes, ALPN, padding behavior, and key-share structure. A matching
   JA3/JA4 hash alone is insufficient evidence.
5. Keep fixture-based parser/export tests hermetic and independent of Bun in
   normal CI. Never make Bun a serving dependency or overwrite an existing profile
   without explicit operator intent; record exact usage commands after tooling exists.

Acceptance criteria:

- A documented local run using an installed Bun produces a specification that
  TLS-02 accepts and TLS-03 can use for a successful supported TLS connection.
- Evidence names the tested Bun version/API and supported capture conditions;
  no claim is made that the output covers every Bun version/API or resumed handshake.
- Truncated/fragmented/oversized input, timeout, missing-runtime, unsupported-data,
  and proxy-environment cases have controlled outcomes and tests.
- Generated profiles and committed evidence contain no machine-specific paths,
  credentials, private keys, real session secrets, or fixed real key exchanges.
- Structural comparison results and any residual differences are recorded; full
  Bun/HTTP-stack indistinguishability is not claimed.

Verification: new helper/fixture tests included by `make test`; run and record the
actual local capture/export/comparison command after TLS-01 establishes it.
Working directory is `<repo-root>`. The real-Bun smoke test requires Bun; if it
cannot run, keep its acceptance criterion/task blocked rather than treating
fixture-only checks as runtime evidence.

Completion evidence:

- Bun capture agent added the standalone `scripts/tls-clienthello` capture/compare
  tool, safe asdf-shim resolution, isolated child environment, bounded record/export
  helpers, secure output publishing, and sanitized fixtures/example metadata.
- Measured recipe and metadata: `examples/tls/bun-1.4.4-fetch.json` and
  `examples/tls/bun-1.4.4-fetch.metadata.json`; generated outputs use mode 0600.
  Helpers expose `ReadClientHello`, `ExportClientHello`, `NormalizeClientHello`,
  and `SummarizeClientHello` plus structural summary types and capture bounds.
- Normal/race profile/helper tests passed: 54 functions, 246 subtests and ten fuzz
  seeds. The optional installed-runtime test was skipped in fixture-only runs
  and passed separately with real Bun. Targeted vet and 257 capture fuzz executions passed.
- The interim `TestBunCaptureCancellation` failure was corrected, not labeled
  pre-existing. Exact cancellation test passed 50 normal and 50 race repetitions;
  cancellation/reaping/socket-descriptor checks passed ten repeated race runs.
- Agent measured three fresh Bun 1.4.4 fetch and three uTLS captures: 1500-byte,
  HTTP/1.1-only initial records matched with no residual structural differences
  after only the specified volatile normalization. Export passed `ParseCustom`
  and verified local TLS 1.2/1.3 synthetic-trust handshakes.
- Coordinator inspected the tool, environment/metadata and helper APIs; reran
  `go run ./scripts/tls-clienthello compare --bun bun --api fetch --samples 3 --spec examples/tls/bun-1.4.4-fetch.json`
  successfully against installed Bun and confirmed matched=true, three/three
  samples, 1500 bytes, no differences. Independent focused race tests passed:
  `go test -race -count=1 ./internal/tlsprofile ./scripts/tls-clienthello`.
- Fetch/first-flight only; padding policies and unsupported/resumption data are
  rejected rather than inferred. Metadata contains no machine paths or secrets.
  Serving activation remains TLS-08; no live-provider/WAF/full-stack compatibility
  or non-Linux runtime claim was made.

Review reopening:

- Independent frozen audit reproduced two P2 capture-file issues. With forced
  publication, changing a parent directory into an alias after preflight could
  make metadata overwrite the profile destination while returning success.
  Unix `openRegularRead` also lacked `O_NOCTTY`, so rejected PTY input could
  acquire a controlling terminal in a fresh session.
- Original Bun capture agent owns directory-identity/anchored publication
  hardening, Unix no-controlling-terminal opening, and focused race/PTY/output
  regressions in the capture tool. Preserve overwrite/partial-publication policy,
  parent environment, source schema and all inference behavior.
- TLS-06 remains reopened until focused normal/race fixes and independent
  reproduction recheck pass. Earlier completion evidence is retained. TLS-07/08
  started after the original gate and may continue in non-overlapping files, but
  neither may close before this dependency is reverified.
- The same independent review found no confirmed TLS-03/04 defect; frozen guard,
  CONNECT, retirement, probe candidate/snapshot and discovery cases passed, as
  did independent HRR downgrade/cipher-change, proxy-port and prepared-close
  probes. That is scoped code evidence, not final TLS-08 acceptance.

Correction and independent re-verification:

- Capture agent hardened preflight parent identities and anchored staging,
  rename/link, rollback and cleanup with `os.Root`. Pair identity/collisions are
  rechecked within validated directories; detected parent changes reject before
  replacement. Forced publication remains nontransactional, but check-operation
  races cannot redirect writes outside the anchored original directories.
- Unix regular-file opening now includes `O_NOCTTY`. Both original counterexample
  regressions failed before fixes and passed afterward; 25 normal and 25 race
  repetitions, focused helper/profile normal/race/vet, all-five-target CGO-disabled
  helper compilation, and real Bun three/three comparison passed.
- Independent auditor reproduced the original parent-alias sequence and verified
  rejection before changing original outputs, checked gap/rollback/descriptor
  safety, and confirmed rejected PTY input leaves `/dev/tty` unavailable. Normal
  checks and three race repetitions passed with no remaining concrete finding.
- Independent real Bun 1.4.4 comparison passed again: three/three, 1500 bytes,
  no differences. Coordinator inspected anchored code/open flags. TLS-06 is
  completed again; historical reopening/evidence remains above.

### Task TLS-09: Unblock required website verification with scoped dependency resolution

Status: completed

Kind: defect

Priority: P1 — the explicitly required documentation build cannot pass with the baseline website dependency-resolution configuration.

Assigned role: TLS documentation/build agent

Dependencies: none

Primary ownership:

- `website/postcss.config.js`
- `website/docusaurus.config.ts`
- Scoped resolution/build verification; no dependency/toolchain or component upgrade.

Finding:

Required `pnpm docs:build` failed in the delivered docs and independently isolated
HEAD website. PostCSS's loader resolved unqualified `tailwindcss` from its own
dependency context to hoisted v4.3.3, instead of the website's declared v3.4.19.
Explicit website resolution fixed CSS compilation and exposed a second baseline
SSR error: duplicate peer-scoped theme-common contexts put `useColorMode` outside
its provider. The combined two-config correction passed an isolated current-docs
build without package/lockfile/CSS/component changes.

References:

- `website/postcss.config.js` (unqualified plugin keys).
- `website/package.json` (declared Tailwind 3 and Docusaurus 3.10.1).
- `website/docusaurus.config.ts` (website plugin/webpack resolution boundary).
- Installed `postcss-loader` resolution and isolated HEAD build evidence from the documentation agent.

Requirements:

1. Resolve Tailwind/autoprefixer explicitly from the website config using existing
   packages, not a Tailwind v4 migration or new dependency.
2. Canonicalize exact theme-common entrypoints in website webpack resolution so
   server provider/consumer contexts share the same installed website instance.
3. Keep the correction scoped to the required verification prerequisite. Preserve
   user tool-version edits, package versions/locks, components, CSS and web UI.
   Record baseline React-ref typecheck errors as unrelated limitations, not fixes.

Acceptance criteria:

- Current `pnpm docs:build` passes with the TLS documentation and existing dependencies.
- Resolution probes show website-declared Tailwind and one canonical theme-common
  provider/consumer context, and generated CSS has no unprocessed Tailwind directives.
- Docs contract/format checks pass; no package/lock/toolchain or unrelated UI/source changes.
- Final reviewer inspects the narrow configuration change and upgrade caveat;
  interactive browser/color-mode behavior is not implied by static-site build evidence.

Verification: `pnpm docs:build`, `make docs-contract`, scoped configuration
format/resolution checks from `<repo-root>`, and independent evidence review.

This necessary build prerequisite was discovered during TLS-07 verification;
it is not a TLS transport defect or permission to expand unrelated site work.

Delivery and verification pending final review:

- Documentation/build agent changed only the two scoped configs. Current-checkout
  `pnpm docs:build`, `make docs-contract`, scoped Prettier/whitespace and artifact
  checks passed. Resolution uses Tailwind 3.4.19/autoprefixer 10.5.4 and one
  theme-common provider/consumer context; four TLS pages/seven anchors and processed
  CSS were verified. Packages, locks and protected inputs remained unchanged.
- No required site-build blocker remains. Final independent config/doc review is
  pending; baseline React-ref typecheck errors and untested interactive color-mode
  behavior remain explicit limitations, not claimed fixes.

Completion evidence:

- Coordinator reran current `pnpm docs:build` successfully and inspected scoped
  config/resolution changes; docs contract/config/Markdown formatting passed.
- Independent reviewer accepted TLS-09: canonical website Tailwind 3.4.19 and
  autoprefixer 10.5.4 resolution, exact website/classic theme-common aliases,
  generated TLS anchors and processed CSS were verified without actionable findings.
- No dependency/toolchain/component migration was introduced. Revalidate exact
  internal-entry aliases when upgrading Docusaurus; interactive browser/color-mode
  behavior and the baseline React-ref typecheck issues remain outside this evidence.

### Task TLS-07: Document the configuration, capture workflow, and limitations

Status: completed

Assigned role: TLS documentation agent

Kind: improvement

Priority: P1 — public configuration and impersonation claims must agree with tested behavior.

Dependencies: TLS-04, TLS-05, TLS-06, TLS-09

Resolved prerequisites: TLS-06 filesystem fixes and TLS-09 website configuration repair independently accepted; current website build passes.

Primary ownership:

- `docs/design.md`, `README.md`, `AGENTS.md`.
- `website/docs/configuration.md`, `website/docs/api-reference.md`, `website/docs/operations.md`.
- Relevant examples and the repository's release-note/changelog surface.

Finding:

There are no public ClientHello profile settings or Bun capture instructions.
The earlier discussion included illustrative names/JSON, not tested presets or
verified Bun data; documentation must not turn those examples into unsupported claims.

References:

- Existing provider configuration docs and the related timeout/plain-HTTP tasks.
- `Makefile:54-55` and `scripts/check-doc-contracts.sh`.
- TLS-01's final decisions and TLS-03/TLS-06 verification evidence.

Requirements:

1. Document the actual allowlist, native default, alias-versus-pinned behavior,
   attribute scope/inheritance, custom-file schema/resolution/bounds, managed
   provider restrictions, reload rules, and supported HTTP/proxy combinations.
2. Explain that `custom` loads supplied configuration and never detects Bun;
   document the capture/export workflow, metadata, and verified Bun version/API.
   Clearly mark illustrative versus measured profiles.
3. State ClientHello-only scope, fresh crypto/SNI generation, continued certificate
   verification, unsupported resumption/extension cases, HTTP-stack differences,
   and lack of WAF/bot-acceptance guarantees. Keep user-agent settings independent.
4. Update public examples and release notes together. Do not alter endpoint or
   provider-capability matrices, since this feature adds no inference capability.

Acceptance criteria:

- Every published value/example corresponds to implemented, tested behavior.
- Documentation does not imply `bun = chrome`, current-browser auto-downloads,
  complete BoringSSL impersonation, or successful live WAF bypass verification.
- Configuration/API/tool instructions agree, and docs contract/build checks pass.

Verification: `make docs-contract` and `pnpm docs:build` from `<repo-root>`, plus
format/link/example review. Run configuration examples through validation with
synthetic credentials and the documented fixture prerequisites.

Delivered work and verification limitations:

- Documentation agent updated README/AGENTS/design/changelog, configuration/API/
  operations/config-examples pages, `examples/tls-client-hello.hcl`, and the TLS
  example README. Exact selectors, cached custom recipes, managed/inheritance
  restrictions, reload/proxy/safety behavior and measured Bun limits are documented.
- `make docs-contract` passed with support matrices byte-identical. Scoped format,
  forty local links/allowlist/privacy checks, and 22 offline CLI snippet/selector/
  edit/conversion checks passed. Historical changelog formatting was not rewritten.
- `pnpm docs:build` failed with the Tailwind/PostCSS plugin error, reproduced
  against isolated HEAD using existing dependencies. CSS-only correction exposed
  a baseline duplicate ColorMode context error; TLS-09 owns the tested two-config
  resolution prerequisite. Do not relabel the failed build as successful TLS-07.
- Separate website typechecking reproduced identical baseline React-ref errors in
  `website/src/theme/CodeBlock/Buttons/Button/index.tsx` and
  `website/src/theme/MDXComponents/Details.tsx`. These are outside this objective
  and were not changed. Static-site success does not establish interactive browser testing.
- With TLS-09's scoped resolution repair applied, current website build and docs
  contracts pass. Public docs and prerequisite configuration await final independent
  acceptance; the earlier failed baseline/current build evidence is retained.

Completion evidence:

- Independent reviewer accepted docs against selectors, source/inheritance rules,
  API omission/reset/privacy, proxy/ALPS and retirement behavior, and measured
  Bun limitations. Six TLS snippets and the full example validated offline;
  docs contracts and canonical website artifacts were checked independently.
- Coordinator current `pnpm docs:build`, `make docs-contract`, and scoped Prettier
  passed after all code/config corrections. No full-stack/live-WAF or interactive
  website claims were introduced. TLS-06 and TLS-09 closure prerequisites are met.

### Task TLS-08: Verify integration, safety boundaries, and delivered evidence

Status: completed

Assigned roles: TLS independent reviewer; TLS binary integration agent (test-only support)

Kind: improvement

Priority: P1 — close the feature only after end-to-end and regression evidence is complete.

Dependencies: TLS-01, TLS-02, TLS-03, TLS-04, TLS-05, TLS-06

Final closure prerequisite: TLS-07 completed and independently checked against the implementation.

Primary ownership:

- `internal/integration/` hermetic binary-level coverage where needed.
- Cross-path capture/proxy/reload tests and this document's completion evidence.
- Review only of other implementations unless a concrete integration correction is needed.

Finding:

This feature crosses configuration, mutable TLS state, pooling, live reload,
proxies, alternate outbound paths, and provider management. Unit-level preset
selection or a fingerprint hash cannot establish the full contract.

References:

- `internal/integration/binary_test.go` and `Makefile:61-72`.
- `AGENTS.md` (verification and behavior-preservation requirements).
- All task acceptance criteria and TLS-01's final decision record.

Requirements:

1. Verify native/preset/custom behavior end-to-end across direct/alias inference,
   HTTP/ALPN/proxy cases, health/model requests, edited/stored configuration, and
   successful/failed reloads. Include concurrency and long-stream lifecycle cases.
2. Review strict input bounds, extension support, certificate verification,
   credential/redirect isolation, custom-file authorization, immutable specs,
   transport retirement, and CGO-disabled build compatibility.
3. Run the shared checks below sequentially where they rebuild shared outputs;
   record actual results and skipped prerequisites without relabeling failures
   or fixture-only evidence as verified runtime compatibility.
4. If execution is delegated, obtain final review from someone other than the main
   implementer. Otherwise report self-review accurately; do not claim independent
   review or start subagents without authorization.

Acceptance criteria:

- Each implemented contract and each task acceptance criterion has concrete
  test/capture/documentation evidence, including the installed-Bun local smoke run.
- Required targeted, database-backed, race, build, integration, UI, and docs
  checks pass; unresolved required checks leave the relevant task blocked.
- No new credentials/private profile data cross authorization boundaries, no
  profile silently changes or downgrades, and native behavior remains the default.
- Deferred advanced profiles/features have explicit rationale and residual limits;
  live-provider/WAF compatibility remains unverified and outside this objective.

Verification: the shared final checks below and an acceptance-criterion review.

Binary integration support delivery (not final task closure):

- Separate test-only agent added `internal/integration/tlsprofile_binary_test.go`
  and `tlsprofile_fixtures_test.go`, without production edits or shared builds.
- Eight new binary tests cover CLI/configuration/conversion/lexical source rules,
  72 direct inference requests across TLS 1.2/1.3 and h1/h2 direct/HTTP CONNECT,
  ordered Bun recipes, admitted alias retries, active SSE/same-path reload and
  rollback, certificate and selected-signature rejection, health and discovery.
- Agent normal binary suite passed: sixteen top-level tests, 100 subtests,
  37.851s. Filtered new-test race harness passed: eight top-level tests, 93
  subtests, 35.967s; targeted vet/format/whitespace checks passed, zero skips.
  Bun is not required for fixture-based binary CI.
- Binary HTTPS-CONNECT and managed-database cases were not added in this delivery;
  existing transport matrices and serialized database/app tests cover those
  layers. Final reviewer must assess the combined evidence and residual limits.
  The first race run instrumented the harness, not the unchanged child binary.
- Coordinator inspected the test/fixture boundary and started an independent
  complete binary run plus a separately race-instrumented child build. Results,
  public-doc agreement, TLS-06 correction recheck, and final independent review
  remain required before TLS-08 can close.
- Coordinator complete binary suite passed (55.597s). A separately built
  race-instrumented server and race-instrumented harness passed all new binary
  TLS tests (33.936s), using `GORACE='atexit_sleep_ms=0 halt_on_error=1'` and a
  temporary child binary. This closes the instrumentation limitation of the
  first harness-only race run, not the remaining review/docs/filesystem gates.

Completion evidence:

- Separate final reviewer inspected actual binary assertions and independently
  passed eight new tests/93 subtests, zero skips, against a copied coordinator
  server with a race harness. The coordinator's separately instrumented server
  run provides actual child-process race evidence. Reviewer implemented neither
  production code nor these binary tests and reported no remaining actionable finding.
- Independent final review accepted TLS-07 and TLS-09 and found the missing
  binary HTTPS-CONNECT/managed-database combinations acceptable layered-coverage
  gaps: existing route matrices, serial database/app tests and reviewed shared
  factory/catalog paths cover those compositions. No acceptance criterion was
  silently dropped or substituted with fingerprint hashes alone.
- Required coordinator verification passed: final `make vet test`,
  `make test-race`, full `make build`, `make integration`, complete/new TLS binary
  tests including race-instrumented server, serial PostgreSQL 17 race tests,
  UI typecheck/218 tests, current docs build/contract/format checks, module
  verify/tidy checks, and CGO-disabled serving compilation for all five release targets.
- Installed Bun 1.4.4 capture/export/comparison was independently rechecked after
  filesystem fixes: three/three, 1500 bytes, no residual differences, secure
  isolated publication. Source/transport/credential/file boundaries and reload/
  retirement/cancellation regressions are delivered and verified.
- Unrelated tool/Helm input hashes stayed unchanged. No commits or provider calls
  were made. Live WAF/provider and non-Linux runtime behavior, interactive site
  behavior, release-archive reproducibility and unrelated baseline website
  React-ref typechecking remain explicitly unverified/out of scope.

## Shared verification and definition of done

All commands below run from `<repo-root>`; replace that placeholder with the
checkout before executing any shell command that needs a directory. Prerequisites
are the repository-declared Go/Node/pnpm tools and installed dependencies. Database
tests require a disposable `AIPROXY_TEST_DATABASE_URL`; the local Bun experiment
requires Bun but no external provider credentials.

Run focused package tests with each task. Final checks:

```sh
make vet test
make test-race
make build
make integration
make docs-contract
pnpm --filter @aiproxy/web-ui typecheck
pnpm --filter @aiproxy/web-ui test
pnpm docs:build
```

`make build` and `make integration` rebuild shared web/binary outputs; do not run
them concurrently with other builds. Fixture tests must not require internet
access, live providers, or installed browsers/Bun.

Follow `.github/workflows/test.yml` for database-backed verification: shared
database packages must run serially, separately from the default unit suite.
With `AIPROXY_TEST_DATABASE_URL` set to the disposable database, additionally run:

```sh
go test -race -p 1 -count=1 ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app
```

Use the five target platforms in `go-release.json` for CGO-disabled compatibility
checks. Release archives/reproducibility checks are optional unless dependency or
packaging changes require them; do not overwrite release artifacts accidentally.

Done means TLS-01's decisions are recorded, TLS-02 through TLS-09 are completed
with evidence, custom-source and transport safety boundaries are tested, an
installed-Bun profile has measured local evidence, and implementation/types/docs
agree. Saving this plan does not complete any implementation task.

## Decision record

Finalized by TLS-01; subsequent agents must implement this conservative v1
contract or bring an evidence-backed change to the coordinator before expanding it.

- [x] Pin `github.com/refraction-networking/utls v1.8.2`; shipping selectors are
      `native`, `chrome`, `chrome-133`, `safari`, `safari-16.0`, and `custom`. Map
      aliases explicitly to `HelloChrome_133` and `HelloSafari_16_0`; native uses host
      Go TLS, not the fork's `HelloGolang`. Firefox 120 is deferred for the reproduced
      [second-key-share failure](https://github.com/refraction-networking/utls/issues/402).
      Other legacy, randomized, PSK, draft-Kyber and unpublished-version candidates
      are deferred. Do not mutate presets to make a broken profile appear supported.
- [x] Use standard Go 1.27 transports with a routing wrapper and an internal
      `ConnectionState() crypto/tls.ConnectionState` adapter. Freeze each proxy route
      into its own inner transport; set inner `Proxy = nil` and have the custom TLS
      dialer perform direct dialing or HTTP/1.1 CONNECT before upstream uTLS. HTTPS
      proxies use separately verified standard Go TLS with HTTP/1.1 ALPN. Preserve
      CONNECT-buffered bytes, allow only proxy-URL Basic credentials in CONNECT, and
      cap CONNECT response headers at the standard 10 MiB bound. Native upstream
      HTTP stays unchanged. Non-native SOCKS5/SOCKS5h, HTTP/2 CONNECT, QUIC/HTTP/3,
      real ECH and resumed/PSK handshakes are explicitly unsupported, not rerouted.
- [x] Preserve 30s TCP dial/keepalive, 10s TLS handshake deadlines, 1-minute CONNECT
      deadline, standard idle/header/body settings, context cancellation and no
      stream-read deadline. Factory header timeout zero means disabled; inference,
      probes and discovery retain their separate current deadline semantics. A
      bounded prebuilt generation per runtime pools by effective timeout/profile;
      retire old generations after successful activation, closing idle connections
      immediately and after active response EOF/Close/errors. No historical pool map.
- [x] Provider-only fields, no new root default or bypass env knob. Omission means
      native; explicit empty/unknown/wrong-case selectors fail. Custom requires a
      nonempty source; every spec-file declaration on non-custom fails, even empty.
      Static-derived providers inherit both fields/profile with the endpoint and
      reject all local declarations, including explicit defaults. Validate disabled
      providers' profiles too, without changing disabled credential behavior. HTTP
      requests do not use upstream TLS, but absolute HTTPS health URLs use the profile.
- [x] Resolve relative custom references against the named config file's absolute
      lexical directory. Source-less/inline-env configs require an absolute reference;
      do not depend on invocation cwd. In-place edits keep declared spelling and
      validate using the real config source. Conversion rewrites references to their
      resolved absolute paths to survive relocated output, without copying profiles.
      This preserves host-local usability, not automatic portability. No tilde,
      shell, URL or custom-JSON env expansion. Allow operator-owned regular-file
      symlinks, reject devices/FIFOs/directories without blocking, and cap reads.
- [x] Use an immutable private canonical recipe/digest including dependency and
      schema/policy version. Decode fresh mutable specs per connection; do not share
      key material or mutate global uTLS dictionaries. Keep file I/O in candidate
      construction; runtime validation/materialization never reopens the source.
- [x] Managed providers support built-in profiles only, including inherited
      effective profiles. Reject custom/file/inline data before persistence/file
      access for every admin role; a managed derivative cannot extend a static custom
      base. Persist one selector column with SQL default empty meaning omitted,
      preserving native/inheritance defaults. Reject explicit local derived fields;
      a concrete-to-derived update clears the old selector only if its input omitted
      the field. Explicit empty/null is invalid. No spec-file column or path/content
      in API views; static custom metadata is read-only/static-only.
- [x] Developer tool lives under `scripts/tls-clienthello/` in the root Go module.
      Initial API is Bun `fetch` only, fresh subprocesses, literal loopback request,
      proxy disabled, synthetic localhost SNI, no provider credentials, no HTTP body,
      isolated HOME/cwd and minimal child environment. Parent environment is unchanged.
      Use a dedicated empty Bun config, disable dotenv loading/auto-install, and bound
      listener/process/output handling. Capture and metadata are separate outputs;
      regular-file publishing does not overwrite without explicit `--force` intent.

### Strict custom JSON v1 and negotiated-crypto safety

- File <=64 KiB, valid UTF-8, one object, depth <=8, tokens <=4096; reject duplicate
  keys, unknown fields, null required collections, trailing data and contradictions.
  Source references <=4096 UTF-8 bytes with no controls/NUL. Errors identify the
  provider/field/index without echoing source content or arbitrary paths.
- Suites: 1..64 unique ordered names, at most one GREASE; initial names cover the
  measured 17 Bun suites (nine modern AEAD choices plus eight advertised-only
  legacy AES CBC/static-RSA choices). Compression exactly `["NULL"]`.
- Extensions: 1..32 ordered entries, unique names except up to two GREASE entries.
  Initial allowed names: `GREASE`, `server_name`, `extended_master_secret`,
  `renegotiation_info`, `supported_groups`, `ec_point_formats`, `session_ticket`,
  `application_layer_protocol_negotiation`, `status_request`,
  `signature_algorithms`, `signed_certificate_timestamp`, `key_share`,
  `psk_key_exchange_modes`, `supported_versions`, and `padding`. Require SNI,
  groups and signatures; TLS 1.3 also requires versions/key shares. Reject custom
  ALPS, certificate compression, raw/generic/unknown, ECH, NPN, QUIC, cookie,
  delegated-credential, PSK identity/early-data and resumption-specific extensions.
- Groups: 1..16 unique real entries; X25519MLKEM768, x25519, P-256/384/521 and
  optional GREASE. v1.8.2 supports MLKEM crypto but lacks its JSON symbol: use an
  explicit private `X25519MLKEM768` adapter, not a global dictionary edit.
- Key shares: 1..3, at most one GREASE and one real classical share; an MLKEM share
  requires x25519 fallback. No multiple distinct classical shares or MLKEM-only
  input. Omit every real `key_exchange`; GREASE may use canonical dummy `[0]` only.
- Signatures: 1..32 unique supported modern ECDSA/RSA-PSS/RSA-PKCS1 SHA-256/384/512
  or Ed25519 names; measured SHA-1 names may be advertised but must be rejected if
  selected. No DSA, SHA-224, Ed448 or fake schemes. Points exactly uncompressed.
- ALPN absent, h1-only, or h2+h1 in either declared order; no duplicate/empty/unknown
  or h2-only recipes. Versions TLS 1.2/1.3 plus optional GREASE, 1..3 entries,
  consistent with optional numeric bounds 771/772; otherwise infer TLS 1.2 only.
  Require a safe usable suite for each enabled version. PSK modes, if present,
  are exactly `psk_dhe_ke`; tickets are empty first-flight advertisements only.
- Padding `len` is 0..4096, zero meaning BoringPaddingStyle rather than a fixed
  count. Dry-run ClientHello with synthetic SNI must be <=16 KiB; discard generated
  dry-run crypto. Fresh SNI/random/key values come from the actual connection.
- Keep bundled browser advertisements unchanged; actual negotiation must select
  TLS 1.2/1.3 and modern ECDHE/AEAD or TLS 1.3 AEAD suites. Clamp versions after
  `ApplyPreset`; never enable weak ciphers, skip certificate verification, or strip
  legacy advertisements while retaining the original profile label.
- Add a bounded read-only initial-server-handshake guard below uTLS: inspect
  ServerHello and TLS-1.2 ServerKeyExchange across record/TCP fragmentation,
  skipping certificate payloads rather than buffering them. Reject weak selected
  versions/suites, SHA-1/unknown signatures and unsupported ECDHE groups. Suggested
  bounds: TLS record payload <=18432, ServerHello <=65536, ServerKeyExchange <=4096.
  The prototype proved cipher/version checks alone permit trusted SHA-1 handshakes.
  Do not let warning alerts disable inspection. uTLS still validates cryptography,
  certificates, transcripts, downgrade markers and TLS-1.3 signatures.
- Explicitly reject present negotiated ALPS, even an empty settings payload;
  standard HTTP/2 does not consume it. Keep session cache nil and disable
  renegotiation after the initial handshake without changing its initial bytes.

### Local capture/export contract and measured result

Proposed tool invocation, to be implemented by TLS-06:

```sh
go run ./scripts/tls-clienthello capture --bun bun --api fetch --samples 3 --out tls/bun-fetch.json --metadata tls/bun-fetch.metadata.json
go run ./scripts/tls-clienthello compare --bun bun --api fetch --samples 3 --spec tls/bun-fetch.json
```

Tested Bun flag ordering is `bun --config=<empty-config> --no-env-file --no-install
run <fetch-script> <loopback-url>`; options after `run` were not equivalent.
Record version/revision/platform/API, capture options, dependency/schema version,
digest, comparison result and limitations without machine paths or secrets.
Capture deadlines: accept/read 3s, child 5s; <=16 TLS records, <=18432 record
payload, <=32768 total handshake bytes, <=64 KiB child stdout/stderr. Always reap
canceled children and close accepted sockets/listeners.

Use non-blunt uTLS fingerprinting, no automatic padding, no real-PSK import, and
an explicit exporter: ordinary Go JSON marshaling of `ClientHelloSpec` is not the
accepted native JSON recipe. Fail on unsupported/resumption-dependent data or
padding behavior that cannot be established from the evidence. Compare actual
record/handshake structure, not a JA3/JA4 hash; normalize only specified volatile
random/session/key bytes. Commit secret-free recipes/metadata or sanitized raw
fixtures, not private captures.

Measured Bun 1.4.4 `fetch`, Linux x64, default experimental h2/h3 flags off:
1500-byte first record, 17 suites, NULL compression, TLS 1.3 then 1.2, h1-only
ALPN; groups 4588/29/23/24, key shares 4588/29, ordered extensions
0/23/65281/10/11/35/16/5/13/18/51/45/43. No GREASE, ALPS or padding was observed.
Three generated uTLS captures matched the normalized Bun captures, and verified
TLS 1.2/1.3 replay passed across direct/HTTP-CONNECT/HTTPS-CONNECT. This does not
verify other Bun APIs/versions, HTTP/2, resumed handshakes, full-stack fidelity,
or live-provider/WAF acceptance.

## Verification evidence

At creation: source/task analysis and worktree inspection only. Go/UI/runtime
tests, Bun capture, and transport prototypes were not run. Record task-file
formatting/contract checks below once performed, separately from implementation
verification.

Task-file checks performed from `<repo-root>` against `eafc7dd` plus this new document:

- `pnpm exec prettier --check docs/tasks/20261112-173948-upstream-tls-client-hello-profiles.md`
  passed after correcting the initial table-alignment failure.
- `make docs-contract` passed: documentation contract matrices match.
- Task-document structure/privacy audit passed: eight pending tasks, all required
  fields, acyclic/resolved dependencies, three valid relative links, balanced
  fences, and no absolute local paths, private session identifiers, or trailing whitespace.
- `git diff --check` passed for tracked changes; the new untracked task file's
  whitespace was checked separately by the document audit.
- Worktree inspection confirmed that this document is the only added file and
  the pre-existing unrelated modifications remain present. No implementation
  task was started or marked completed.

### Execution environment and baseline

The following observations apply to the subsequently authorized implementation,
not to the original task-file creation evidence above:

- Available toolchain: Go 1.27.4, Bun 1.4.4, pnpm 11.24.0, and Node 26.13.0.
  Node differs from the package-declared 26.8.1; do not rewrite the unrelated
  tool-version edits to hide this warning.
- An isolated disposable PostgreSQL 17 test container was started on loopback
  for database-backed verification; existing local services were not reused.
- Baseline `pnpm --filter @aiproxy/web-ui typecheck` passed before code changes.
- Baseline `pnpm --filter @aiproxy/web-ui test` passed: 11 files, 211 tests.
- Baseline `make vet test` without database environment passed.
- Baseline `make test-race` without database environment passed.
- Baseline `make docs-contract` passed before TLS source changes.
- A CGO-disabled host binary built before TLS source changes passed hermetic
  binary tests: `AIPROXY_BINARY='<baseline-binary>' go test -tags=integration ./internal/integration`
  (5.593s). `<baseline-binary>` denotes the isolated temporary output, not a
  release artifact; this baseline does not verify the new TLS feature.
- An initial database-backed `make vet test` failed while running packages in
  parallel, with shared migration/table races in dbmerge and HTTP API tests.
  This occurred before any TLS source implementation. CI explicitly serializes
  these packages; do not repeat that parallel database invocation.
- Baseline `go test -p 1 -count=1 ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app`
  with the disposable database passed all four packages after the parallel failure.
- TLS design agent completed TLS-01 in an isolated prototype; production
  implementation starts with TLS-02. Coordinator owns this document's status/evidence.

### Shared integration prechecks

These are post-implementation coordinator checks, not completion of TLS-08 or a
substitute for its independent review/documentation/binary/database criteria:

- Default `make vet test` passed across the delivered source, with database
  environment unset; capture cancellation and the new profile packages passed.
- Default `make test-race` passed across the delivered source with database
  environment unset, including upstream, health, application, and capture code.
- `go mod verify` passed. An initial `go mod tidy -diff` identified that the
  capture publisher's direct `golang.org/x/sys/unix` use needed direct dependency
  classification. Coordinator promoted the existing v0.48.0 requirement without
  changing its version; the repeated tidy-diff check was clean.
- Scoped `gofmt -l` and `git diff --check` were clean. Hashes of the three
  unrelated user-modified tool/Helm files remain unchanged.
- Full `make build` passed, including UI typecheck/production bundling and the
  CGO-disabled host binary. Existing hermetic `make integration` passed (6.220s);
  new TLS-specific binary cases/review are still required by TLS-08.
- Production binaries compiled with `CGO_ENABLED=0`, `-trimpath`, and
  `-buildvcs=false` for all five `go-release.json` targets: linux/amd64,
  linux/arm64, windows/amd64, darwin/amd64, and darwin/arm64. Outputs went to an
  isolated temporary verification directory, not release archives. Non-host
  runtime behavior was not tested.
- Full PostgreSQL-backed `go test -race -p 1 -count=1 ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app`
  passed against the isolated PostgreSQL 17 fixture (24.823s, 1.545s, 125.302s,
  26.164s). Packages were serialized as CI requires.
- Final additional binary coverage, public docs checks, and final reviewer
  acceptance remain pending; do not treat these prechecks as all acceptance
  criteria met. The disposable database remains available for final review.

### Post-correction final reruns

- `make vet test` passed again after the capture filesystem corrections.
- Current `pnpm docs:build` passed with the scoped website resolution repair;
  `make docs-contract` and scoped Markdown/configuration Prettier passed.
- `go mod tidy -diff`, scoped Go formatting, and `git diff --check` remained clean.
  Protected tool/Helm hashes were unchanged.
- A fresh final capture into isolated temporary outputs using installed Bun
  matched three Bun and three uTLS samples at 1500 bytes, without overwrite flags
  or residual differences. This additionally exercised anchored publication.
- Final `make test-race` rerun passed after all code corrections. Independent
  TLS-07/09/08 acceptance remains in progress; the earlier passed binary/race-server
  results are recorded in TLS-08.

### Final acceptance

- Independent review accepted TLS-09, TLS-07 and TLS-08 with no remaining
  evidence-backed actionable findings. Required coordinator sanity reruns passed
  before these tasks were closed; all nine task IDs are completed.
- Previous partial results, baseline failures, review reopenings and their
  correction evidence are retained above rather than rewritten as initial success.
- All additions/updates made by this implementation belong to this objective and
  the narrow required website verification prerequisite. Existing tool/Helm modifications
  were preserved. No full AI session identifiers or private machine paths are
  recorded in this document.
- Final task-document audit passed: nine completed tasks, required fields,
  dependency/link resolution, formatting, privacy and whitespace checks clean.
  The coordinator removed only the owned disposable PostgreSQL test container
  after final review; existing local services and user files were not touched.
- Operational limitation: this is opt-in ClientHello profiling, not a complete
  browser/Bun networking stack or a verified WAF/bot-acceptance bypass.
