# GitHub Copilot Device Flow

Created: 2026-09-07 02:26:59 (local)

## Objective

Add explicit GitHub.com device authorization and a `github-copilot` provider supporting `/v1/chat/completions` JSON and SSE. The maintainer selected a configurable OAuth client ID and a chat-first release. This document authorizes no assumption that arbitrary GitHub OAuth applications can access Copilot inference.

Non-goals: Enterprise domains, Responses/Messages translation, generic OAuth infrastructure, automatic model synchronization, browser callback servers, importing another application's credentials, or bundling OpenCode's OAuth client ID. Do not add token refresh or token exchange unless the verified upstream contract requires it.

## Analysis and Constraints

Read-only analysis covered provider/config registries, adapters, CLI configuration, credential persistence, load/reload behavior, routing, and relevant tests. No baseline tests, builds, or authenticated upstream requests were run. The worktree was clean at analysis time. Implementation and all verification remain pending.

Evidence:

- `internal/config/helpers.go`: `resolveProviderCredential` resolves `api_key_ref` into a static `Provider.APIKey` at load time. Writing the referenced file alone does not change a running server.
- `internal/config/types.go` and `internal/config/capabilities.go`: `ProviderType`, `providerTypePolicies`, and `ProviderTypes` define the supported provider/config surface.
- `internal/provider/provider.go`: `providerDescriptors`, `Request`, and `adapter.Do` define dispatch registration; `internal/provider/openai.go` provides chat pass-through and stream usage handling.
- `internal/httpapi/operations.go`: `ensureOperationSupported` gates operations. `internal/httpapi/dispatch.go` implements direct/alias dispatch and health classification.
- `cmd/aiproxy/main.go`: `newRootCommand` and `defaultSecretsPath` are CLI integration points. `cmd/aiproxy/configure.go` handles both scripted and interactive credential configuration.
- `internal/configedit/configedit.go`: `ReadSecretsFile` expects a flat string map. `BuildSecretsUpdate` uses read-modify-write; atomic file replacement alone does not prevent lost concurrent updates.
- `internal/filestore/filestore.go`: existing secret persistence protections can be reused, but multi-file replacement is not a crash-atomic transaction.
- `internal/app/app.go`: `App.Reload` reloads credentials and preserves the active runtime when candidate validation fails.
- `cmd/aiproxy/models_upstream.go`: upstream model listing has a separate provider switch and must not retain inconsistent authentication behavior.

External evidence inspected: [OpenCode Copilot plugin](https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/plugin/github-copilot/copilot.ts). At inspection it used GitHub device authorization and sent the resulting access token directly to Copilot, storing the same token under both `access` and `refresh` with `expires: 0`. This mutable source demonstrates that application's behavior, not eligibility for an arbitrary client ID. Pin the source revision and verify current official documentation in COPILOT-01.

Related work, not duplicate objectives:

- `20260906-101616-opencode-provider-zen-go.md`: provider addition and hermetic verification precedent.
- `20260822-160713-provider-inheritance.md`: derived providers retain local credentials.
- `20260823-112437-codebase-health-follow-up.md`: separate config/adapter policy registries and parity checks.
- `20260804-125911-codebase-health-review-remediation.md`: persistence and pure-validation boundaries.

## Execution Rules

Priority P1 means a prerequisite or core feature required for release; P2 means dependent integration/release closure, not optional verification. All tasks below are improvements except the bounded contract investigation.

Provisional CLI: `aiproxy login github-copilot --client-id <id> --credential <name>`. Final names must follow existing CLI conventions. Require an explicitly supplied client ID; do not request a client secret for a public device client.

Prefer the existing flat secret store and `api_key_ref` only if COPILOT-01 establishes a durable bearer-token contract. Preserve offline validation, explicit missing-credential errors, local credentials on derived providers, static inventories, direct-request isolation, alias retry policy, and separate inbound API authentication. Login must not run from `serve`, `validate`, or inference handling.

Only COPILOT-01 can start immediately. After its completion, COPILOT-02 and COPILOT-03 may run concurrently with a fixed shared credential contract and non-overlapping ownership. COPILOT-04 owns subsequent CLI/config integration; sequence shared-file edits. The coordinator maintains this document. COPILOT-05 must include a reviewer who was not the main implementer.

## Tasks

### COPILOT-01: Establish the Authentication Contract

Status: completed

Kind: investigation

Priority: P1, client eligibility determines whether the feature can work.

Suggested agent: upstream protocol investigator

Dependencies: none

Primary ownership: this document's contract evidence and relevant rationale in `docs/design.md`; no runtime changes.

Finding: GitHub device login and Copilot inference authorization are distinct contracts. The inspected OpenCode implementation does not prove that credentials issued to the configured application will be accepted.

References: external plugin link above; `internal/config/helpers.go` (`resolveProviderCredential`); `internal/provider/provider.go` (`Request`).

Requirements:

1. Verify official device-flow endpoints, required scopes, public-client requirements, polling rules, lifetime, cancellation/error handling, and client eligibility for Copilot inference. Pin external source revisions and record documentation URLs/access dates.
2. Establish inference origin/path, supported chat model selection, required headers and their semantics, token lifetime/revocation behavior, and whether exchange or refresh is actually necessary. Do not impersonate another editor to obtain access.
3. Resolve safe treatment of user/agent initiation and vision metadata for a generic proxy rather than copying OpenCode's application-specific assumptions.
4. Recommend implement, defer, or revise scope. If structured expiring credentials are required, amend dependent tasks and storage ownership before implementation; do not disguise them as static API keys.

Acceptance criteria:

- An evidence-backed contract describes login, persistence, inference, re-login, header policy, and release eligibility.
- Unknowns are explicitly listed. If client eligibility cannot be established, dependent implementation remains blocked with a maintainer prerequisite rather than treating mock tests as proof.

Verification: review official documentation and pinned source; an authorized live experiment only with a maintainer-provided eligible client/account. Never include tokens or device codes in evidence.

Completion evidence (added 2026-09-07, investigation only; no runtime changes, no authenticated upstream requests, no tokens/device codes recorded):

Findings:

- Login (device flow, public client, official GitHub docs): `POST https://github.com/login/device/code` with `client_id` (required) and `scope` (space-delimited; OpenCode requests `read:user`), then the user authorizes at `https://github.com/login/device` with the `user_code`, then the client polls `POST https://github.com/login/oauth/access_token` with `client_id`, `device_code`, `grant_type=urn:ietf:params:oauth:grant-type:device_code`. Device flow must be enabled in the OAuth app settings. No `client_secret` is required for a public device client (official `incorrect_client_credentials` semantics). Device/user codes expire after 900s (15 min). Polling must honor the server `interval` (default 5s per RFC 8628 when absent). `authorization_pending` means keep polling; `slow_down` means add 5s to the interval (server returns the new interval) and continue; `access_denied` (user cancelled) and `expired_token`/`token_expired` (codes expired) mean stop polling and require a fresh device-code request. OpenCode additionally applies a 3s safety margin on top of the interval and prefers the server-supplied interval on `slow_down`, which is a reasonable policy to adopt.
- Inference origin/path: `https://api.githubcopilot.com` for GitHub.com; enterprise maps to `https://copilot-api.<domain>` (OpenCode `base()`). Model catalog is server-driven via `GET {base}/models` with `Authorization: Bearer <oauth-token>`, `User-Agent`, and `X-GitHub-Api-Version`. Catalog entries carry `id`, `model_picker_enabled`, `supported_endpoints` (contains `/chat/completions`, `/responses`, `/v1/messages`), `capabilities.supports` (`tool_calls`, `streaming`, `vision`), `limits`, and `policy.state`. Chat-first release means selecting only models whose `supported_endpoints` include `/chat/completions`. The exact chat POST path is SDK-resolved against the base (OpenCode delegates to `@ai-sdk/github-copilot`); the presumed path is `{base}/chat/completions`, but this is an unknown pending the authorized live test. Proxy inventory stays static config; no runtime model sync.
- Required headers and semantics (observed in pinned OpenCode source, application-specific): `Authorization: Bearer <oauth-token>` (same token stored as both `access` and `refresh`); `User-Agent: opencode/<version>`; `X-GitHub-Api-Version: 2026-06-01` (models call); `Openai-Intent: conversation-edits`; `x-initiator: user|agent` (declares user vs agent initiation for billing/policy); `Copilot-Vision-Request: true` (declares image input); session-correlated `X-Interaction-Id` / `X-Interaction-Type` and `anthropic-beta` (messages shim only). Header policy for the generic proxy: send an allowlist only (`Authorization` from server-side stored credential, proxy-controlled `User-Agent: aiproxy/<version>`, `X-GitHub-Api-Version` for models, `Openai-Intent`, derived `x-initiator`, derived `Copilot-Vision-Request`); omit session-correlated `X-Interaction-*` headers (no session concept in the proxy); strip inbound `Authorization`, cookies, `x-api-key`, and any caller-supplied Copilot metadata headers.
- Token lifetime/revocation: default OAuth app issues non-expiring tokens (OpenCode stores `access == refresh`, `expires: 0`); revocation is via the user's GitHub settings/connections page. If the app opts into expiring tokens (global setting or `offline_access` scope), access tokens live 8h with a 6-month refresh token. No token exchange step was observed (OAuth token is sent directly as the inference bearer). No refresh is necessary for the default contract; refresh (`POST /login/oauth/access_token` with `grant_type=refresh_token`, no `client_secret` required for device-flow tokens) is only needed if the maintainer enables expiring tokens, which is currently out of scope per the task non-goals.
- User/agent initiation and vision (generic proxy, not OpenCode's assumptions): never trust inbound `x-initiator`/`Copilot-Vision-Request` values. Default `x-initiator: user`; derive vision server-side by inspecting the outbound request body for image parts (`image_url`/`input_image`); OpenCode's session/compaction/subagent inspection (`sdk.session.message/get`, synthetic compaction parts, `parentID`) is application-specific and must not be copied. The proxy has no equivalent session graph, so agent-initiation detection beyond body-shape heuristics stays `user` unless a future explicit inbound contract is designed.
- Client eligibility: official Copilot SDK docs ("GitHub OAuth setup") explicitly authorize arbitrary OAuth App / GitHub App client IDs: create an OAuth app, run the standard flow, pass the resulting user token (`gho_`/`ghu_`/`github_pat_`; `ghp_` unsupported) to the SDK, and Copilot requests run against that user's subscription (each user needs an active Copilot subscription). This establishes that a configurable client ID can work, at least through the SDK/CLI runtime path. What is NOT established is whether a raw OAuth token sent directly to `https://api.githubcopilot.com` (bypassing any SDK/CLI token exchange such as the `copilot_internal` token endpoint the official runtimes may use) is accepted for an arbitrary maintainer-registered client ID. OpenCode's direct-Bearer success proves only its own bundled client ID (`Ov23li8tweQw6odWQebz`), which must not be bundled or impersonated. Direct-path acceptance for the maintainer's client ID therefore remains gated on an authorized live test; it is a release prerequisite, not a reason to stall hermetic implementation.
- Persistence consequence: a structured, expiring-capable credential is required, not a static API key. Minimum record: `{client_id, domain, access_token, refresh_token_or_same, expires_at (0 = non-expiring), obtained_at}`. The current flat string map (`internal/configedit` `ReadSecretsFile` → `map[string]string`; `internal/config/helpers.go` `resolveProviderCredential` materializing a static `Provider.APIKey` at load; key-file-only changes inert until SIGHUP/restart per `internal/app/app.go` `Reload`) cannot represent this without disguising it. Dependent-task amendment required before implementation: COPILOT-02 must own a dedicated credential record plus locked read-modify-write storage (JSON object value under one secrets key or a sidecar file reusing `internal/filestore` protections, never a bare string token mixed into the flat map, never printed/logged); COPILOT-03 must read the structured credential in the adapter (not `Provider.APIKey` string reuse) and share the auth construction with COPILOT-04's `models_upstream.go` listing path; `configure` must reference the saved login by name without embedding OAuth networking or displaying tokens. Login must never run from `serve`, `validate`, or inference handling.
- Re-login: on `401`/`403` from inference or `GET /models`, on token revocation/expiry, or on `expired_token` during polling, surface an explicit re-login-required error (no automatic generation replay, no retry after streaming starts, per existing alias/direct semantics); the operator re-runs the login command with the same client ID/credential name, then SIGHUP/restart activates it; a failed reload candidate must leave the old runtime intact.

Contract recommendation: implement (chat-first, explicitly configured OAuth client ID, GitHub.com only for release; enterprise mapping documented but out of scope). Proceed with COPILOT-02/COPILOT-03 hermetically only after amending them for structured-credential storage/ownership as above. Release stays gated on an authorized live verification with a maintainer-provided eligible client ID and Copilot-subscribed account: device login, `GET /models`, chat JSON plus SSE against an eligible chat model, and the `401` re-login path. Bundling or impersonating another application's client ID is prohibited.

Unknowns (explicit):

1. Exact chat POST path and whether direct-Bearer (no SDK exchange) is accepted for an arbitrary client ID (release-gated live test).
2. Whether any intermediate token exchange (`copilot_internal` or equivalent) is required before inference; none observed in OpenCode, but official runtimes may perform one.
3. Full required-vs-optional header set enforced server-side (e.g. whether `Openai-Intent` or `X-GitHub-Api-Version` is mandatory for chat, exact `User-Agent` constraints).
4. Token lifetime for the maintainer's app configuration (non-expiring default vs 8h expiring) and consequent refresh need.
5. Supported chat model list at release time (server-driven; proxy inventory remains static).
6. Enterprise (`copilot-api.<domain>`) inference behavior (documented mapping only; out of release scope, not verified).

Pinned sources, revisions, access dates (all accessed 2026-09-07 UTC; no authenticated requests made):

- `https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps` (device flow: endpoints, `client_id`/`scope`, `interval`/`expires_in` 900s, polling errors `authorization_pending`/`slow_down`/`expired_token`/`access_denied`/`incorrect_client_credentials`, expiring-token 8h/6mo + `offline_access`, refresh without `client_secret` for device-flow tokens, device-flow enablement prerequisite).
- `https://docs.github.com/en/copilot/how-tos/copilot-sdk/setup/github-oauth` (arbitrary OAuth App / GitHub App client IDs supported via SDK; `gho_`/`ghu_`/`github_pat_` work, `ghp_` does not; per-user Copilot subscription required; token lifecycle is the app's responsibility).
- `https://docs.github.com/en/copilot/how-tos/copilot-sdk/auth/authenticate` (auth methods, token types, priority order).
- `https://docs.github.com/en/copilot/how-tos/copilot-sdk/auth/server-to-server-tokens` (installation-token path; confirms user-token vs service-token separation).
- `https://docs.github.com/en/copilot/concepts/models/utility-models` (matches OpenCode `UTILITY_MODELS`: `gpt-5.4-nano`, `gpt-4.1`, `gpt-4o`, `gpt-4o-mini`).
- `https://www.rfc-editor.org/rfc/rfc8628.txt` (device authorization grant: `interval` default 5s, `slow_down` +5s, `authorization_pending`/`access_denied`/`expired_token` polling rules, public-client treatment).
- OpenCode plugin `packages/opencode/src/plugin/github-copilot/copilot.ts`, fork `anomalyco/opencode`, branch `dev`, pinned commit `c0f09afef5056cfbebdf5123162267cb6efbd960` (2026-09-04T04:24:55Z, via GitHub commits API filtered by file path; raw content verified identical via `raw.githubusercontent.com` for the `sst/opencode` mirror): device-code request (`client_id`, `scope: read:user`), `+3000ms` polling margin, `slow_down` handling with server `interval` preference, `access == refresh == access_token` with `expires: 0`, inference base `https://api.githubcopilot.com` (enterprise `https://copilot-api.<domain>`), direct-Bearer fetch wrapper headers (`x-initiator`, `Openai-Intent: conversation-edits`, `Copilot-Vision-Request`, `User-Agent`, `X-GitHub-Api-Version: 2026-06-01`, `X-Interaction-Id/Type`), bundled `CLIENT_ID = Ov23li8tweQw6odWQebz` (do not reuse).
- OpenCode plugin `packages/opencode/src/plugin/github-copilot/models.ts` (same branch/commit window; fetched 2026-09-07): `GET {base}/models` with bearer headers, `supported_endpoints` (`/chat/completions`, `/responses`, `/v1/messages`), `model_picker_enabled`, `capabilities.supports`/`limits`, `policy.state` filtering.

Files reviewed (read-only; no modifications): `internal/config/helpers.go` (`resolveProviderCredential`), `internal/config/types.go`, `internal/config/capabilities.go`, `internal/provider/provider.go`, `internal/provider/openai.go`, `internal/httpapi/operations.go`, `internal/httpapi/dispatch.go`, `cmd/aiproxy/main.go`, `cmd/aiproxy/configure.go` (credential/model option plumbing), `internal/configedit/configedit.go` (`ReadSecretsFile` flat map, `BuildSecretsUpdate` read-modify-write), `internal/filestore/filestore.go` (`WriteFile`/`ReplaceFiles` protections), `internal/app/app.go` (`Reload` preserves runtime on invalid candidate; key-file-only changes need reload), `cmd/aiproxy/models_upstream.go` (separate provider switch for upstream listing).

Verification performed: code reads listed above; web fetches listed above; no `make vet/test`, no builds, no authenticated upstream requests, no token/device-code material handled (per task instructions).

### COPILOT-02: Implement Device Login and Safe Persistence

Status: completed

Kind: improvement

Priority: P1, required credential acquisition and storage boundary.

Suggested agent: CLI and credential implementation agent

Dependencies: COPILOT-01

Primary ownership: focused device-flow package under `internal/`, new login command under `cmd/aiproxy/`, command registration in `main.go`, necessary `internal/configedit`/`internal/filestore` persistence changes and their tests. Avoid provider adapter files and configure UX edits owned by COPILOT-04.

Finding: No upstream device-login command exists. Current flat secret updates can lose independent changes from concurrent writers.

References: `cmd/aiproxy/main.go` (`newRootCommand`, `defaultSecretsPath`); `internal/configedit/configedit.go` (`BuildSecretsUpdate`, `ReadSecretsFile`); `internal/filestore/filestore.go` (`WriteFile`, `ReplaceFiles`).

Requirements:

1. Implement explicit client-ID input, URL/code display, cancellable polling, issuer interval handling, cumulative slowdown, deadline enforcement, denial and malformed-response handling. Bound HTTP timeouts and response sizes.
2. Keep OAuth origins independent of configurable inference `base_url`; prohibit credential-leaking redirects. Use injectable transport/time for hermetic tests without exposing unsafe production endpoint overrides.
3. Persist credentials only after successful authorization using restrictive permissions and existing secret-path protections. Never print tokens, raw token responses, or secrets in errors/logs.
4. Coordinate all writers sharing the chosen store around read-modify-write, using a stable lock or equivalent mechanism. Preserve unrelated keys and existing credentials on cancellation or persistence failure. Do not mix object records into the flat string map.
5. Report persistence separately from runtime activation. Do not silently edit HCL or signal a running server. Document re-login and SIGHUP/restart requirements under the established contract.

Acceptance criteria:

- Hermetic tests cover success, pending, repeated slowdown, denial, expiry, cancellation, malformed/oversized responses, redirect refusal, and network/persistence failure.
- Tests verify redacted output, file permissions, unsafe destination rejection, and concurrent independent updates without lost keys, including the existing configure writer.
- Login requires no loaded valid provider config and never runs implicitly from serving or validation.

Verification: targeted Go tests for changed CLI/device-flow/configedit/filestore packages; shared final checks below.

Completion evidence (added 2026-09-07, COPILOT-02 only; no COPILOT-03/04/05; no CHANGELOG.md):

Storage choice: sidecar file per credential (`<secrets-dir>/copilot-<name>.json` via `internal/copilotlogin.SidecarPath`), not JSON objects inside the flat `keys.json` map. Rationale: preserves the `ReadSecretsFile → map[string]string` contract and `config/helpers.go extractKeyValue` string-value expectation; flat-map concurrent writers are fixed separately with a stable file lock so no existing `configure` behavior changes shape. Sidecar writes reuse `internal/filestore` protections (0600 file, 0700 dir, symlink/non-regular refusal, atomic rename); per-credential files make concurrent independent logins independent (no shared read-modify-write), same-name races stay atomic last-writer-wins, and cancel/failure never touches the flat map or other sidecars.

Changed files:

- `internal/copilotlogin/device.go` (new): production device-flow client (`DeviceCodeURL`/`TokenURL` constants, 15s HTTP timeout, 256KiB response bound, redirect refusal, `client_id`+`scope`/`device_code`+`grant_type` posts, no `client_secret`, default 5s interval / 900s expiry, cumulative +5s `slow_down` preferring larger server `interval`, `authorization_pending` continue, `access_denied`/`expired_token` terminal, deadline + context cancellation, loopback-http allowance only for hermetic tests).
- `internal/copilotlogin/credential.go`, `internal/copilotlogin/store.go` (new): structured record `{client_id, domain, access_token, refresh_token_or_same, expires_at, obtained_at}`, name validation matching lowercase provider rules, sidecar save/load with redacted errors.
- `internal/copilotlogin/device_test.go`, `internal/copilotlogin/credential_test.go` (new): hermetic success/pending/slowdown (incl. cumulative + server-interval preference)/denial/expiry/deadline/cancel/malformed/oversized/redirect/network tests; perms, symlink/unsafe-name rejection, concurrent independent credentials, cancel-preserves-existing, flat-map untouched.
- `internal/filestore/lock.go`, `internal/filestore/lock_unix.go`, `internal/filestore/lock_windows.go`, `internal/filestore/lock_other.go` (new): stable cross-writer lock (`path.lock` + in-process registry; `flock(LOCK_EX)` on unix, in-process mutex elsewhere).
- `internal/filestore/lock_test.go` (new): lock serializes holders.
- `internal/configedit/configedit.go`: `WriteSecretsUpdate`/`WriteProviderFiles` now hold `filestore.Lock` across locked read-modify-write; `BuildSecretsUpdate`/`ReadSecretsFile` shape unchanged.
- `internal/configedit/locked_test.go` (new): concurrent `WriteSecretsUpdate` keys preserved; mixed `WriteSecretsUpdate` + `WriteProviderFiles` (existing configure writer) without lost keys.
- `cmd/aiproxy/login.go` (new): `aiproxy login github-copilot --client-id <id> --credential <name> [--secrets-path ...] [--scope read:user]`; requires explicit client ID, no `--client-secret` flag, no endpoint-override flags, no config load, no HCL edit, no server signaling; prints verification URI + user code only, then `saved credential ... (0600)` plus `restart or SIGHUP` and `re-run on 401/403` guidance.
- `cmd/aiproxy/main.go`: registered `newLoginCommand()`.
- `cmd/aiproxy/login_test.go` (new): CLI success/pending/denial/expiry/cancel/malformed/oversized/redirect/network/persistence-failure, redacted output, perms, unsafe-dest (symlink) rejection, flat-map preservation, concurrent logins + configure writer, no-config-required, `validate` creates no login artifacts, command/flag registration.

Commands/results (2026-09-07, hermetic, no GitHub credentials):

- `go vet ./internal/copilotlogin/ ./internal/configedit/ ./internal/filestore/ ./cmd/aiproxy/` → pass.
- `go test ./cmd/aiproxy/ ./internal/copilotlogin/ ./internal/configedit/ ./internal/filestore/ -count=1` → all ok.
- `go test -race ./internal/copilotlogin/ ./internal/filestore/ -count=1` → ok; `go test -race ./internal/configedit/ -run TestConcurrent -count=3` → ok.
- `make vet test` (`go test ./...`) → all packages ok.

Follow-ups for COPILOT-03/04 (not implemented): adapter must read the sidecar structured credential (not `Provider.APIKey` reuse) and share auth construction with `models_upstream.go`; `configure` must reference the saved login by name without OAuth networking or token display; release still gated on authorized live verification per COPILOT-01.

### COPILOT-03: Implement the Chat-Only Provider

Status: completed

Kind: improvement

Priority: P1, core inference support.

Suggested agent: provider adapter implementation agent

Dependencies: COPILOT-01

Primary ownership: `internal/config/{types,capabilities,validate}.go`, related config construction only as needed, `internal/provider/`, focused `internal/httpapi` and e2e tests. Avoid CLI registration and persistence files owned by COPILOT-02.

Finding: Neither config policy nor runtime adapter registries include Copilot. Registering it as unrestricted OpenAI-compatible would advertise unsupported operations and omit its authentication/header contract.

References: `internal/config/capabilities.go` (`providerTypePolicies`); `internal/provider/provider.go` (`providerDescriptors`); `internal/provider/openai.go` (`doOpenAI`); `internal/httpapi/operations.go` (`ensureOperationSupported`).

Requirements:

1. Add explicit `github-copilot` type with chat-only capabilities and verified default origin/path. Apply existing transport URL validation and safe redirect handling.
2. Implement a dedicated adapter using the COPILOT-01 token/header contract. Reuse model rewrite, JSON/SSE response handling, cancellation, cleanup, and usage extraction where practical.
3. Reject unsupported operations before any auth or inference I/O. Never forward inbound authorization, cookies, arbitrary headers, or unvalidated client metadata.
4. Preserve direct routing and alias retry/health semantics, including upstream authentication/quota errors. Do not automatically replay generation requests on authentication failure or after streaming starts.
5. Preserve local credential isolation for derived providers and maintain registry parity tests. No runtime model synchronization.

Acceptance criteria:

- Tests verify exact request path, bearer credential, required headers, metadata policy, upstream model rewrite, JSON/SSE and tool/usage handling, and cancellation.
- Unsupported operations make zero upstream calls; direct failures do not change targets; aliases follow existing configured status policy.
- Config tests cover defaults, invalid capability declarations, missing credentials, disabled providers, and derived local credentials.

Verification: targeted tests in `internal/config`, `internal/provider`, `internal/httpapi`, and `internal/e2e`; shared final checks below.

Completion evidence (added 2026-09-07, COPILOT-03 only; no COPILOT-01/02/04/05 changes; no CHANGELOG.md):

Credential choice: new `credential_ref { path?, name }` block for `github-copilot` only, resolved at config load into the dedicated `Provider.CopilotToken` field (never `Provider.APIKey`). Rationale: reusing `api_key_ref` is impossible without breaking the flat string-map contract (`config/helpers.go extractKeyValue` expects a string value; a sidecar JSON object would fail parsing). `path` defaults to `defaultKeyFilePath()` so the sidecar is located via the existing `copilotlogin.SidecarPath(secretsPath, name)` derivation; no new global secrets-path plumbing. Load-time resolution preserves offline validation, explicit missing/expired-credential errors, and SIGHUP/restart activation semantics; per-request file reads were rejected (wrong layer, bypasses reload boundaries). Auth construction is shared via the new `internal/copilotlogin/auth.go` helper (`BuildHeaders`/`ApplyHeaders`, `HasImageContent`, `UserAgent`, contract constants) for reuse by COPILOT-04's `models_upstream.go` listing path. Derived providers must declare their own local `credential_ref` (base token is cleared, never inherited); `api_key`/`api_key_ref` are rejected on copilot blocks and `credential_ref` is rejected on all other types.

Changed files:

- `internal/copilotlogin/auth.go` (new): shared contract constants (`DefaultBaseURL https://api.githubcopilot.com`, `ChatCompletionsPath /chat/completions`, `APIVersion 2026-06-01`, `Openai-Intent conversation-edits`, `x-initiator user` default), `BuildHeaders`/`ApplyHeaders` allowlist (Authorization from stored token, proxy `User-Agent: aiproxy/<version>`, `X-GitHub-Api-Version`, `Openai-Intent`, derived `x-initiator`, vision `Copilot-Vision-Request: true` only when the body contains image parts; explicit deletion of Cookie/`x-api-key`/`anthropic-beta`/`X-Interaction-*` and conditional vision), server-side vision detection (`image_url`/`input_image` structural scan, not substring), deterministic Accept (SSE iff `stream:true`).
- `internal/copilotlogin/auth_test.go` (new): allowlist exactness, vision derivation (incl. text mention of `image_url` not triggering), streaming Accept, version default, inbound-value overwrite/strip.
- `internal/config/types.go`: `ProviderTypeGitHubCopilot`, `Provider.CopilotCredentialRef`/`CopilotToken`, `CopilotCredentialRef` type.
- `internal/config/schema.go`: `credential_ref` block (`path` optional, `name` required).
- `internal/config/capabilities.go`: copilot registered last in `providerTypeOrder`, policy default `[chat]` / supported `[chat]` (chat-only).
- `internal/config/helpers.go`: copilot structural rules in `validateProviderCredentialStructure`, new `resolveCopilotCredential` (sidecar load, expired-token rejection with re-login hint, token into `CopilotToken` only).
- `internal/config/build.go`: `buildProvider`/`buildDerivedProvider` wire + resolve `credential_ref`; derived copilot requires local `credential_ref`, rejects `api_key`/`api_key_ref`, clears inherited token.
- `internal/config/validate.go`: enabled copilot requires resolved `CopilotToken` with login-oriented error; disabled copilot may omit it.
- `internal/config/catalog.go`: clone new credential fields.
- `internal/config/capabilities_test.go`: copilot parity row (default/supported chat-only).
- `internal/config/copilot_test.go` (new): defaults (token resolved, `APIKey` empty, default caps `[chat]`), all non-chat capabilities rejected, missing/unknown/expired credentials rejected, disabled without credential accepted, `api_key` on copilot and `credential_ref` on non-copilot rejected, protocol/`user_agent` rejected, derived local-credential isolation (distinct tokens, inherited models), base_url transport validation (remote http rejected, loopback allowed).
- `internal/provider/provider.go`: `Request.CopilotToken`, copilot descriptor with `copilotlogin.DefaultBaseURL`.
- `internal/provider/githubcopilot.go` (new): `doGitHubCopilot` rejects non-chat ops before any body/auth I/O, rejects empty token before I/O, reuses model rewrite + `openAIPassthroughHandlers` (JSON/SSE, usage, tool calls, cancellation via request context, cleanup via `executeUpstream`); POST `{base}/chat/completions`.
- `internal/provider/githubcopilot_test.go` (new): exact `/chat/completions` path, default origin, bearer from `CopilotToken` (`APIKey` ignored), required headers, inbound metadata stripping (auth/cookies/`x-api-key`/`x-initiator`/`X-Interaction-*`/caller intent+version), model rewrite, JSON usage + tool_calls, SSE usage, all five unsupported ops with zero upstream calls, missing credential with zero calls, 401 verbatim single-call passthrough, context cancellation/deadline.
- `internal/httpapi/dispatch.go`: plumb `CopilotToken` in direct + alias dispatch (routing/retry/health logic untouched).
- `internal/httpapi/copilot_test.go` (new): unsupported ops → `unsupported_operation` 400 with adapter never called; token plumbing (`APIKey` empty); direct 401 verbatim single call; alias failover on 502 to next target and no retry on 401 under default status policy.
- `internal/e2e/copilot_stub_test.go` (new): full-stack JSON chat (path/bearer/UA/initiator/vision/metadata/model-rewrite assertions), SSE chat, unsupported ops with zero upstream calls, missing-credential build failure.

Commands/results (2026-09-07, hermetic, no GitHub credentials):

- `go build ./...` → pass.
- `go vet ./internal/config/ ./internal/provider/ ./internal/httpapi/ ./internal/e2e/ ./internal/copilotlogin/` → pass.
- `go test ./internal/config/ ./internal/provider/ ./internal/httpapi/ ./internal/e2e/ ./internal/copilotlogin/ -count=1` → all ok.
- `make vet test` (`go test ./...`) → all packages ok (no regressions in `cmd/aiproxy`, `internal/app`, `internal/configedit`, etc.).

Unknowns/follow-ups (for COPILOT-04/05, not implemented here):

1. Exact chat POST path `{base}/chat/completions` is the COPILOT-01 presumption; no live request was made — pending authorized live verification (COPILOT-05).
2. Whether `X-GitHub-Api-Version` / `Openai-Intent` are mandatory server-side for chat is unverified; both are sent from the allowlist (extra headers are low-risk, a missing required one would break) — confirm live.
3. Upstream redirect behavior reuses the existing shared upstream client (same as all providers); no per-provider redirect refusal was added — review in COPILOT-05 whether credential-bearing inference requests need the stricter device-flow redirect policy.
4. `cmd/aiproxy/models_upstream.go` still returns `unsupported provider type` for copilot and `configure` has no copilot UX — owned by COPILOT-04, which should reuse `copilotlogin.BuildHeaders`.
5. Public matrices/docs untouched per task sequencing — owned by COPILOT-05.

### COPILOT-04: Integrate Configuration, Model Listing, and Reload

Status: completed

Kind: improvement

Priority: P2, closes alternate entry paths and operator workflow.

Suggested agent: CLI/runtime integration agent

Dependencies: COPILOT-02, COPILOT-03

Primary ownership: `cmd/aiproxy/configure.go`, `cmd/aiproxy/models_upstream.go`, `internal/configedit` rendering, app reload tests, and their focused tests.

Finding: Configure assumes existing API-key choices, model listing dispatches independently of inference, and credentials are activated only through configuration loading/reloading.

References: `cmd/aiproxy/configure.go` (`promptProviderInput`, `applyProviderCredentialOptions`); `cmd/aiproxy/models_upstream.go` (`listUpstreamModels`); `internal/app/app.go` (`App.Reload`).

Requirements:

1. Let interactive and scripted configure reference the saved login with accurate credential terminology. Do not embed OAuth networking in config rendering or display tokens in review output.
2. Keep non-interactive behavior explicit; no surprise browser launch or polling. Preserve credential references on edit and same-type inheritance rules.
3. Add upstream model listing using the verified Copilot authentication contract, without changing the proxy-owned static inventory. Share request authentication rather than duplicating incompatible logic.
4. Verify key-file-only changes leave the active runtime unchanged until reload, successful reload activates the new credential, and an invalid candidate leaves the old runtime intact.

Acceptance criteria:

- TUI/line-oriented and scripted configuration tests produce valid equivalent references without token exposure.
- Upstream model listing tests verify correct origin, credentials, headers, and failure behavior.
- Tests demonstrate offline validation without login/network/persistence side effects, credential rotation on reload, failed-reload rollback, and derived-account isolation.

Verification: targeted tests in `cmd/aiproxy`, `internal/configedit`, and `internal/app`; shared final checks below.

Completion evidence (added 2026-09-07, COPILOT-04 only; no COPILOT-01/02/03/05 changes; no CHANGELOG.md):

Credential/config choice: `credential_ref { path?, name }` referenced by saved-login name. `configure` never performs OAuth networking (no `copilotlogin` device-flow import in `cmd/aiproxy/configure.go`) and never prints tokens; review output contains only config path, action, and the rendered `credential_ref` name/path. Scripted flags are `--credential <name>` / `--credential-path <path>` (github-copilot only); `--api-key`/`--api-key-env`/`--secrets-key` are rejected for copilot with `--credential` guidance, and `--credential*` is rejected for all other types. `path` defaults to the shared secrets path so sidecars resolve via `copilotlogin.SidecarPath`; default-path refs omit `path` on render. Edits preserve existing refs when no credential flags are given; derived copilot providers require a local `credential_ref` and stay compact (no `base_url`/models); same-type inheritance is enforced by the existing base-choice filter plus type-match validation. Copilot models are chat-only (`supported`/`default` `["chat"]`); `base_url` is an optional transport override only.

Changed files:

- `internal/copilotlogin/auth.go`: added `BuildModelsHeaders`/`ApplyModelsHeaders` sharing `DefaultBaseURL`, `APIVersion`, `UserAgent` constants (Authorization + User-Agent + `X-GitHub-Api-Version` + Accept, inbound Cookie/`x-api-key`/`X-Interaction-*` stripped).
- `internal/copilotlogin/auth_models_test.go` (new): minimal allowlist exactness, `UserAgent` sharing, inbound stripping.
- `internal/configedit/configedit.go`: `ProviderCredentialInput` gains `CopilotPath`/`CopilotName`; `renderProviderCredential` renders `credential_ref` (path omitted when default).
- `internal/configedit/copilot_render_test.go` (new): default-path omission, custom-path render, derived compactness, `ValidateGeneratedConfig` round-trip.
- `cmd/aiproxy/configure.go`: `providerOptions` gains `Credential`/`CredentialPath` with `--credential`/`--credential-path` flags; `applyProviderCredentialOptions` branches for `github-copilot`; `existingProviderInput` parses `credential_ref`; `promptProviderInput` handles copilot in non-interactive (explicit `--credential` requirement, derived-local check, disabled preservation) and in both TUI (`GitHub Copilot` type option, base-URL override prompt, `promptCopilotCredentialInteractive`) and line-oriented (`promptCopilotCredentialLineOriented`) flows with login terminology and no token I/O; `defaultCapabilities`/`supportedCapabilities` return `["chat"]` for copilot; `providerTypeDescription` mentions copilot.
- `cmd/aiproxy/configure_copilot_test.go` (new): line-oriented create, scripted create + offline `credential_ref` validation failure, line/scripted equivalence, API-key rejection, missing-credential requirement, ref preservation on edit, derived local-credential requirement + compact render, no-token-exposure assertions.
- `cmd/aiproxy/models_upstream.go`: `upstreamBaseURL` returns `copilotlogin.DefaultBaseURL` for copilot; new `listGitHubCopilotModels` uses `provider.CopilotToken` only (empty token fails before I/O), `GET {base}/models` with `ApplyModelsHeaders`, 401/403 mapped to explicit re-login hint (`re-run login, then restart or SIGHUP`), `{"data":[...]}` plus bare-array parsing, static inventory untouched.
- `cmd/aiproxy/models_upstream_copilot_test.go` (new): correct GET origin/path, bearer + User-Agent + API-version headers, configured/not-in-config annotation, 401 re-login hint, zero-call missing-credential failure, default vs override origin.
- `internal/app/copilot_reload_test.go` (new): sidecar rotation inert until `Reload` then active; corrupt-sidecar and expired-credential candidates fail reload with old runtime intact; derived isolation (base unchanged, derived rotated, no base-token leak); offline missing-credential build failure creates no secrets/sidecar files.

Commands/results (2026-09-07, hermetic, no GitHub credentials):

- `go vet ./cmd/aiproxy/ ./internal/configedit/ ./internal/app/ ./internal/config/` → pass.
- `go vet ./internal/copilotlogin/` → pass.
- `go test ./cmd/aiproxy/ ./internal/configedit/ ./internal/app/ ./internal/config/ -count=1` → all ok.
- `go test ./internal/copilotlogin/ -count=1` → ok.
- `go build ./...` → pass.

Follow-ups for COPILOT-05 (not implemented): public matrices/docs/examples untouched; exact chat POST path `{base}/chat/completions` still presumed pending live verification; whether `X-GitHub-Api-Version`/`Openai-Intent` are mandatory for chat unverified; no binary-level login/config/serve coverage added; release still gated on authorized live device-login + `GET /models` + chat JSON/SSE + 401 re-login path.

### COPILOT-05: Document and Independently Verify the Release

Status: blocked (prerequisite: maintainer-authorized live verification — eligible OAuth client ID + Copilot-subscribed account for device login, `GET /models`, chat JSON/SSE, and 401 re-login path; no real GitHub calls performed)

Kind: improvement

Priority: P2, mandatory release closure and independent verification.

Suggested agent: independent integration reviewer, not the main implementer

Dependencies: COPILOT-02, COPILOT-03, COPILOT-04

Primary ownership: `README.md`, `AGENTS.md`, `docs/design.md`, affected `website/docs/` provider/configuration/operations/API pages, `examples/`, `scripts/check-doc-contracts.sh`, and binary integration tests.

Finding: Public support matrices and operator documentation currently have no Copilot/device-login contract; mocks alone cannot establish real application entitlement.

References: `scripts/check-doc-contracts.sh`; `internal/integration/binary_test.go`; `internal/e2e/opencode_test.go` as an existing provider integration test precedent.

Requirements:

1. Update public matrices and executable examples together: chat JSON/SSE only, proxy-owned inventory/accounting, configurable client ID, credential location, headless provisioning, re-login, and reload behavior. Examples contain no real secrets or another application's client ID.
2. Add hermetic binary-level login/config/serve coverage where feasible, keeping test-only issuer transport seams inaccessible as insecure production defaults.
3. Independently review every acceptance criterion, alternate credential consumers, redirect/header isolation, concurrency, routing, cancellation, and docs/implementation agreement. Record findings as actionable follow-ups or fix them before closure.
4. Run shared checks serially where they rebuild common outputs. Separately verify authorized real-provider device login plus JSON and SSE chat with an eligible model/client/account. Do not perform paid or authenticated calls without authorization.

Acceptance criteria:

- All shared checks pass and independent review findings affecting this scope are resolved.
- Real-provider evidence establishes client acceptance without publishing secrets; if credentials/authorization are unavailable, record delivered work and keep this task blocked for release verification.
- Deferrals state rationale and residual risk, and the task document records actual commands/results rather than inferred success.

Verification: shared checks below, independent code review, and separately authorized real-provider smoke tests.

Completion evidence (added 2026-09-07, COPILOT-05 only; no COPILOT-01/02/03/04 changes; no CHANGELOG.md):

Docs/examples/contracts (no real secrets, no other app's client ID; placeholders are `YOUR_GITHUB_OAUTH_CLIENT_ID` / `test-client-id` / `test-copilot-token` in tests only):

- `scripts/check-doc-contracts.sh`: public matrix gains `github-copilot` (Proxy-owned models/billing/metrics; chat `JSON and SSE`; `No` for embeddings/responses/images/audio); capability matrix gains `` `github-copilot` | `chat` | None ``.
- Same two matrices synced in `README.md`, `AGENTS.md`, `docs/design.md`, `website/docs/api-reference.md` (+ capability matrix also in `website/docs/providers-and-routing.md`).
- `README.md`: provider-type bullet, new `GitHub Copilot` section (login, `credential_ref` HCL, derived-local rule, rejections, SIGHUP activation, 401 re-login, `base_url` transport-only, `{base}/chat/completions` + `{base}/models` listing), CLI `login` + scripted `configure provider --credential` examples, `credential_ref` sidecar note, chat-only/header-allowlist behavior bullets.
- `AGENTS.md`: matrices + `github-copilot` convention bullet (login/sidecar/`credential_ref`/reload/re-login, derived-local, inference path + allowlist, configure/models-listing sharing, no endpoint-override flags).
- `docs/design.md`: provider-type list, `credential_ref` attribute + derived rule, new `#### github-copilot` subsection (device grant, sidecar record, activation/reload, re-login, inference path, allowlist/stripping, gating), credential-resolution exception, validation rules (`credential_ref` misuse, `protocol`/`user_agent` rejection), `login` CLI command, final-decision credential note.
- Website: `providers-and-routing.md` (type-table row + `GitHub Copilot` section), `configuration.md` (attributes, Copilot HCL + derived rule, transport-override default, `credential_ref` subsection, validation rules), `operations.md` (login CLI, scripted Copilot configure, headless note, sidecar mount + reload), `api-reference.md` (chat-only coverage note), `config-examples.md` (Copilot chat section), `intro.md` (provider list).
- `examples/github-copilot.hcl` (new): chat-only config with `credential_ref { name = "copilot-main" }`, commented login/activation/re-login flow, optional loopback `base_url` comment only. `validate` without a sidecar correctly fails (`credential "copilot-main" not found; run login first`); `examples/opencode-zen.hcl` still validates (`config is valid`).

Binary coverage (no new production endpoint-override flags; test OAuth seams stay in-process only):

- `internal/integration/copilot_binary_test.go` (new, `integration && linux`): `TestBinaryGitHubCopilotChatServe` writes the sidecar via `copilotlogin.Save` (simulated login output, no OAuth network), serves a `github-copilot` provider against a loopback stub asserting exact `POST /chat/completions` path, bearer token, `aiproxy/` UA, intent/version/`x-initiator: user`, empty vision on text body, no cookie/`x-api-key` leak, model rewrite; covers JSON + SSE, `POST /v1/responses` → 400 `unsupported_operation` with zero new upstream calls, and proxy-owned `/v1/models` with zero upstream calls. `TestBinaryGitHubCopilotLoginHasNoEndpointOverrides` asserts `login github-copilot --help` exposes `--client-id`/`--credential`, exposes no `device-code-url`/`token-url`/`endpoint`/`base-url`/`client-secret` flags, and missing `--client-id` fails with a flag hint without network.

Independent review (COPILOT-02/03/04 acceptance, consumers, isolation, concurrency, routing, cancellation, docs/impl agreement):

- COPILOT-02: device polling (pending/slow_down cumulative + server-interval preference/denial/expiry/deadline/cancel/malformed/oversized/redirect/network), redacted errors, 0600/0700 + symlink refusal, per-credential sidecars (concurrent independent logins independent; same-name atomic last-writer-wins), cancel/failure preserves flat map + sidecars, file-locked `configedit` writers, no-config-required login, no login path from `serve`/`validate`/inference (verified: login lives only in `cmd/aiproxy/login.go` + `main.go` registration; `configure.go` has no `copilotlogin` device-flow import), no `--client-secret`/endpoint-override flags — all hold.
- COPILOT-03: exact `{base}/chat/completions` (default `https://api.githubcopilot.com`), `CopilotToken`-only bearer (`APIKey` ignored), allowlist headers + server-side vision derivation + streaming Accept, inbound overwrite/strip, model rewrite, JSON/SSE usage/tool handling, cancellation via request context, non-chat rejection before body/auth I/O with zero upstream calls, direct 401 verbatim single call, alias default-policy failover (502 retries, 401 does not), config defaults/rejections (non-chat caps, missing/unknown/expired credentials, disabled-without-credential ok, `api_key`↔`credential_ref` cross-type rejections, `protocol`/`user_agent` rejection, derived-local isolation + inherited models, `base_url` loopback rule), `CopilotToken` plumbed in direct + alias dispatch with routing/retry/health untouched — all hold.
- COPILOT-04: scripted/line/TUI flows render equivalent `credential_ref` without OAuth networking or token display; API-key flags rejected on Copilot and `--credential*` rejected elsewhere; refs preserved on edit; derived-local compactness; chat-only caps; `GET {base}/models` shares `BuildModelsHeaders` with 401/403 re-login hint and `{"data":[...]}`/bare-array parsing, static inventory untouched; sidecar rotation inert until `Reload` then active, corrupt/expired candidates fail with old runtime intact, derived isolation, offline missing-credential build creates no files — all hold.
- Redirect/header isolation: device-flow client refuses redirects (`ErrUseLastResponse` + 3xx-as-error); inference reuses the shared upstream pool (default Go redirect policy, Authorization stripped cross-host), identical to all other providers — reviewed as acceptable, no Copilot-specific tightening added (known residual, low risk; see follow-ups).
- Concurrency/routing/cancellation: sidecar-per-name files + `filestore` lock remove the flat-map lost-update class; alias retry/health/cancellation reuse the existing tested pipeline; docs now state SIGHUP/restart activation and no post-stream replay.
- Small doc/impl mismatches fixed in this task: matrices + prose now state chat-only `No` for responses (was undocumented), `user_agent` rejection on Copilot, and `credential_ref`-only validation. No implementation changes were needed.

Follow-ups (actionable, none blocking hermetic use):

1. Live-gated (release prerequisite): authorized device login + `GET /models` + chat JSON/SSE on an eligible model + 401 re-login with the maintainer's client ID/account; confirms presumed `{base}/chat/completions` path, direct-Bearer acceptance for that client ID, whether `X-GitHub-Api-Version`/`Openai-Intent` are mandatory, token lifetime/refresh need, and whether any SDK-side exchange is required. No live calls made here.
2. Low-risk residual: consider a Copilot-specific inference redirect-refusal (matching the device-flow policy) if threat modeling later requires stricter-than-shared-pool behavior; current shared-pool behavior matches every other provider.
3. Scope reaffirmed: GitHub.com only; enterprise `copilot-api.<domain>` mapping stays documented-but-unverified; no refresh/exchange, responses/messages translation, model sync, or bundled foreign client IDs.

Commands/results (2026-09-07, serial, hermetic, no GitHub credentials):

- `make vet test` → pass (`go vet ./...` clean; all unit packages ok).
- `make test-race` → pass (all packages ok, incl. `cmd/aiproxy`, `internal/app`, `internal/config`, `internal/configedit`, `internal/copilotlogin`, `internal/e2e`, `internal/httpapi`, `internal/provider`).
- `make integration` → pass (builds `dist/aiproxy`; `internal/integration` ok, incl. new `TestBinaryGitHubCopilotChatServe` and `TestBinaryGitHubCopilotLoginHasNoEndpointOverrides` — verified via `go test -tags=integration -v -run GitHubCopilot`: both PASS).
- `make docs-contract` → pass (`documentation contract matrices match`).
- `make build` → pass (`built dist/aiproxy`).
- Spot: `go run ./cmd/aiproxy validate --config examples/github-copilot.hcl` → fails as designed pre-login (`credential "copilot-main" not found; run login first`); `... --config examples/opencode-zen.hcl` → `config is valid`.

## Shared Verification and Definition of Done

Use repository-provided Go/toolchain dependencies; hermetic tests must not require GitHub credentials. Final commands:

```sh
make vet test
make test-race
make integration
make docs-contract
make build
```

Current results (2026-09-07, COPILOT-05, hermetic, no GitHub credentials): `make vet test` pass; `make test-race` pass; `make integration` pass (incl. 2 new Copilot binary tests); `make docs-contract` pass; `make build` pass. Release verification stays blocked on maintainer-authorized live device-login + models + chat JSON/SSE + 401 re-login.

Mark a task `in_progress` only once dependencies are satisfied. Mark it `completed` only after its acceptance criteria and required verification pass; append changed files, commands/results, and follow-ups. Use `blocked` with a named prerequisite when verification or the application contract cannot be established. Update this document as discoveries change scope rather than silently adding refresh infrastructure or claiming supported access.

The objective is complete only when all five tasks are completed, public contracts agree with runtime behavior, credential boundaries are independently reviewed, and the configured application's real Copilot access is verified. Until then, distinguish implemented mock-tested behavior from release-ready support.
