# Codebase Health Follow-Up

Created: 2026-08-23 11:24:37 local time

## Objective

Address residual readability, security, performance, correctness, release, and
architectural-health gaps found in a repository-wide follow-up review after the
completion of the 2026-08-04 remediation plan.

This plan is written for independent sub-agents. It prioritizes confirmed
runtime and release defects, then resource ownership and test coverage, then
larger encapsulation improvements. It does not reopen completed tasks unless
the current source demonstrates a remaining gap or regression.

## Scope

- Go request dispatch, streaming, reload, and application lifecycle behavior.
- CLI daemon portability and the supported release platform contract.
- Build, archive, container, and release workflow integrity.
- Dashboard transport and authentication consistency.
- Provider inheritance integration evidence.
- Runtime catalog encapsulation and provider-policy maintainability.
- Public documentation where it conflicts with current behavior.

## Working Rules

- Read `AGENTS.md` and this entire task file before editing.
- Preserve unrelated worktree changes. Never reset, revert, or overwrite work
  outside the assigned task.
- Set one task to `in_progress` before implementation. Mark it `completed` only
  after its acceptance criteria and verification commands pass, then append
  completion evidence.
- Add a regression test that fails against the old behavior for every confirmed
  defect.
- Prefer the smallest shared enforcement point. Do not combine protocol fixes,
  platform policy, and broad package refactors in one change.
- Keep externally visible behavior stable unless the task explicitly defines a
  contract correction.
- Source comments are discouraged by repository convention. Prefer clear APIs,
  names, tests, and user-facing documentation.
- Do not weaken daemon process-identity checks, request bounds, authentication,
  or secret handling to obtain portability or compatibility.
- Agents editing shared hotspots must follow the sequencing rules below.

## Non-Goals

- Replacing HCL, Cobra, Prometheus, Redis, or the HTTP transport stack.
- Adding a new AI provider or broadening translated-provider feature support.
- Dynamic provider plugins.
- Cosmetic file splitting without an ownership, correctness, or testability
  outcome.
- Real-provider integration tests that require paid credentials or unstable
  external services.
- Repeating work already completed in
  `docs/tasks/20260804-125911-codebase-health-review-remediation.md` or
  `docs/tasks/20260809-132127-remediation-decision-closure.md`.

## Baseline Verification

Review baseline on 2026-08-23:

- The worktree was clean before this task file was created.
- Static review covered Go source and tests, `Makefile`, `Dockerfile`, GitHub
  workflows, package metadata, public docs, and all existing task documents.
- `go version` could not run because asdf has no selected Go version for this
  workspace. Installed candidates include Go 1.26.1, 1.26.5, and 1.26.6.
- Consequently, `make vet test`, `make test-race`, `make build`, and cross-build
  verification were not run while creating this plan.
- At creation time, the prior closure document still had two pending P3 coverage items:
  `FU-26-01` for dashboard `/logs` rate limiting and `FU-26-02` for disabled
  provider alias pruning.
- The provider-inheritance task is marked completed, but its recorded
  verification commands all failed because Go was unavailable.

Before implementation, select the repository's declared Go 1.26 toolchain and
record a fresh baseline with:

```sh
make vet test
make test-race
make build
```

Run focused package tests after each task. Run full checks only after agents
that share runtime or generated outputs have merged their changes.

## Priority Definitions

- P0: Direct credential/data exposure, unsafe process signaling, or remotely
  triggerable unbounded resource use with immediate severe impact.
- P1: Confirmed production correctness, availability, release-integrity, or
  materially misleading security/support contract defect.
- P2: Resource ownership, reload behavior, integration evidence, or operational
  contract gap with concrete impact.
- P3: Defense in depth, low-risk coverage, or maintainability improvement.

No new P0 issue was confirmed by this review.

## Finding Classification

| Task               | Classification                                                                                                                                           |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| STREAM-01          | Confirmed unbounded observer state and duplicated framing logic.                                                                                         |
| OBS-01             | Confirmed metrics double count.                                                                                                                          |
| APP-01             | Confirmed resource-ownership gap on error exits.                                                                                                         |
| BUILD-01           | Confirmed build/archive failure-propagation defect.                                                                                                      |
| PORT-01            | Confirmed mismatch between source portability and advertised targets.                                                                                    |
| RELEASE-01         | Confirmed publication-gating and artifact-integrity gap; image version is an investigation requirement until the external action's behavior is verified. |
| TOOLCHAIN-01       | Confirmed metadata/tooling drift.                                                                                                                        |
| ROUTE-01           | Confirmed reload loss of stateful selector data.                                                                                                         |
| CATALOG-01         | Confirmed encapsulation and test-fixture debt; no current production mutation was found.                                                                 |
| PROVIDER-POLICY-01 | Investigation; implementation requires measured benefit.                                                                                                 |
| DASH-01            | Confirmed configuration and documented transport-contract mismatch.                                                                                      |
| SECURITY-01        | Defense-in-depth hardening plus a confirmed missing route regression test.                                                                               |
| INHERIT-01         | Confirmed verification and cross-boundary coverage gap; not a confirmed inheritance runtime defect.                                                      |
| TEST-01            | Confirmed assembled-system coverage gap.                                                                                                                 |
| TEST-02            | Confirmed missing regression coverage for existing behavior.                                                                                             |
| DOCS-01            | Confirmed public contract contradictions.                                                                                                                |
| REVIEW-01          | Independent verification task, not a separate finding.                                                                                                   |

## Wave 1: Runtime Correctness And Bounds

### Task STREAM-01: Bound OpenAI-Compatible SSE Observation

Status: completed

Completion evidence:

- Changed files: `internal/provider/sse.go`, `internal/provider/openai.go`,
  `internal/provider/provider_test.go`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: OpenAI-compatible pass-through streams now use the shared SSE
  field parser and a bounded observer with `maxSSELineBytes` and
  `maxSSEEventBytes`; observer overflow disables optional usage/error
  observation while continuing pass-through and preserving upstream body close
  ownership.
- Tests: Added provider regressions for exact pass-through byte preservation
  with comments and CRLF, fragmented EOF-delimited usage/error extraction, and
  line/event overflow continuing pass-through without retaining observer state.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test -race ./internal/provider ./internal/httpapi`.
- Result: passed.
- Follow-up: none.

Priority: P1

Suggested agent: streaming protocol and resource-safety engineer

Dependencies: none

Primary ownership:

- `internal/provider/openai.go`
- `internal/provider/sse.go`
- focused tests in `internal/provider/provider_test.go`
- HTTP stream tests only when needed to prove final outcome propagation

Finding:

The OpenAI-compatible pass-through stream observer independently accumulates an
unbounded current line and `data:` lines while extracting usage and stream
errors. A hostile or malformed upstream can therefore grow proxy memory even
though translated Anthropic and Gemini streams use the shared decoder's 1 MiB
line and 32 MiB event limits. The duplicated framing logic can also drift on
CRLF, comments, multi-line data, and EOF-delimited events.

References:

- `internal/provider/openai.go:53-56`
- `internal/provider/openai.go:66-131`
- `internal/provider/sse.go:12-15`
- `internal/provider/sse.go:31-151`
- `internal/provider/provider_test.go:1377-1407`
- `internal/provider/provider_test.go:1441-1456`

Implementation requirements:

1. Reuse one bounded SSE framing primitive for translated event consumption
   and pass-through observation, or extract a small bounded observer from the
   existing decoder.
2. Preserve pass-through response bytes exactly; observation must not rewrite
   valid OpenAI-compatible streams.
3. Define overflow behavior explicitly. Prefer stopping optional usage/error
   observation while continuing bounded pass-through unless terminating a
   malformed stream is required for consistent health semantics.
4. Handle fragmented reads, CRLF, comments, multiple `data:` lines, and a final
   event at EOF consistently across all provider paths.
5. Preserve upstream close, downstream cancellation, and stream-completion
   ownership.

Acceptance criteria:

- A fragmented line larger than `maxSSELineBytes` cannot cause unbounded
  observer allocation.
- A multi-line event larger than `maxSSEEventBytes` cannot cause unbounded
  observer allocation.
- Valid pass-through bytes are unchanged, including comments and CRLF framing.
- Usage and upstream stream-error extraction still work across fragmented reads
  and an EOF-delimited final event.
- The selected overflow outcome is documented in tests and does not leak the
  upstream body or a goroutine.
- `go test -race ./internal/provider ./internal/httpapi` passes.

### Task OBS-01: Count Alias Streaming Attempts Exactly Once

Status: completed

Completion evidence:

- Changed files: `internal/httpapi/dispatch.go`,
  `internal/httpapi/handler_test.go`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: Alias dispatch now records upstream attempts at header receipt
  only for non-streaming results, matching direct dispatch; streaming attempts
  are recorded by the existing stream finalizer at close/completion.
- Tests: Added regressions for direct streaming metric counts, alias streaming
  success latency/counting, alias mid-stream upstream error classification,
  alias downstream cancellation classification, and streaming alias retry
  attempt/in-flight cleanup.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test -race ./internal/httpapi ./internal/provider`.
- Result: passed.

Priority: P1

Suggested agent: HTTP observability and stream-lifecycle engineer

Dependencies: none; serialize with ROUTE-01 at `internal/httpapi/dispatch.go`

Primary ownership:

- `internal/httpapi/dispatch.go`
- metrics regressions in `internal/httpapi/handler_test.go`
- `internal/observability/metrics.go` only if the test needs a narrower query

Finding:

Direct dispatch records upstream request metrics at header receipt only for
non-streaming results. Alias dispatch records every result immediately and then
attaches the common stream finalizer, which records the same streaming attempt
again at completion. Successful alias streams therefore increment upstream
request and latency observations twice.

References:

- `internal/httpapi/dispatch.go:47-53`
- `internal/httpapi/dispatch.go:69-76`
- `internal/httpapi/dispatch.go:148-154`
- `internal/httpapi/dispatch.go:171-177`
- `internal/httpapi/dispatch.go:259-289`
- `internal/httpapi/handler_test.go:1477-1525`
- `internal/httpapi/handler_test.go:1613-1655`

Implementation requirements:

1. Apply the minimal correctness fix so alias streams are not recorded at
   header receipt and again at completion.
2. Consolidate direct and alias attempt finalization only as far as needed to
   prevent future sequencing drift.
3. Keep alias lease release and in-flight gauge ownership explicit and exactly
   once.
4. Preserve one observation per retry attempt and existing provider-health
   semantics.

Acceptance criteria:

- Each direct or alias upstream attempt increments
  `aiproxy_upstream_requests_total` exactly once.
- Stream latency is observed at completion rather than header receipt.
- A mid-stream upstream error and downstream cancellation each produce one
  correctly classified observation.
- Every alias retry attempt is counted once and lease/in-flight counts return
  to zero.
- `go test -race ./internal/httpapi ./internal/provider` passes.

### Task APP-01: Close Resources On Every RunReady Exit

Status: completed

Completion evidence:

- Changed files: `internal/app/app.go`, `internal/app/app_test.go`,
  `internal/providerhealth/providerhealth.go`,
  `internal/providerhealth/providerhealth_test.go`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: `RunReady` now owns app cleanup on every return after it
  consumes an `App`; listener startup resources are released before serve
  ownership transfers, failed shutdown forces server close, and cleanup errors
  are deterministically joined after the primary startup/serve/shutdown error.
  `App.Close` and provider-health `Tracker.Close` are idempotent, so repeated
  closes do not double-close health backends or idle transports.
- Tests: Added focused cleanup coverage for listener failure, dashboard-token
  persistence failure, readiness callback failure with close-error aggregation,
  unexpected serve failure, shutdown failure, and context cancellation. Each
  path verifies an instrumented health backend closes exactly once; repeated
  `App.Close` is covered on the listener-failure path.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test -race ./internal/app ./internal/providerhealth ./cmd/aiproxy`.
- Result: passed.

Priority: P2

Suggested agent: Go application lifecycle engineer

Dependencies: none

Primary ownership:

- `internal/app/app.go`
- `internal/app/app_test.go`
- focused provider-health test doubles when required

Finding:

`Build` creates a health tracker and HTTP client pool, but `RunReady` closes app
resources only on the successful context-cancellation path. Listener failure,
dashboard-token persistence failure, readiness callback failure, unexpected
`Serve` failure, and server-shutdown failure return without consistently
closing app-owned resources. The CLI does not provide an outer deferred close.

References:

- `internal/app/app.go:83-101`
- `internal/app/app.go:108-123`
- `internal/app/app.go:130-158`
- `internal/app/app.go:213-225`
- `cmd/aiproxy/main.go:58-69`

Implementation requirements:

1. Define one lifecycle owner: once `RunReady` consumes an app, every return
   path must release app-owned resources.
2. Make `App.Close` safe for repeated calls if callers retain explicit cleanup.
3. Close listeners, active health backends, and idle transports on all failure
   paths without hiding the primary startup/serve error.
4. Define deterministic aggregation or precedence for shutdown and close
   errors.
5. Preserve readiness-handshake and normal shutdown logging behavior.

Acceptance criteria:

- Listener failure, token persistence failure, readiness callback failure,
  unexpected serve failure, shutdown failure, and context cancellation have
  focused cleanup tests.
- A Redis-backed or instrumented health backend is closed exactly once for each
  exit path.
- Repeated `Close` calls do not panic or double-close unsafe resources.
- `go test -race ./internal/app ./internal/providerhealth ./cmd/aiproxy` passes.

## Wave 2: Build, Platform, And Release Integrity

### Task BUILD-01: Make Cross-Build And Archive Failures Atomic

Status: completed

Completion evidence:

- Changed files: `Makefile`, `scripts/validate-build-atomicity.sh`,
  `.github/workflows/test.yml`, `.github/workflows/release.yml`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: `build-all` now stops on the first failed target;
  `build-single` removes stale target output, compiles into a temporary
  directory, and publishes only after a non-empty expected executable exists;
  archive creation and validation reject missing, empty, wrong-name, or extra
  target contents while preserving archive names.
- Regression: `make validate-build-atomicity` uses a fake Go command to fail a
  non-final matrix entry, verifies stale failed-target output is removed and the
  failure is non-zero, proves successful archives contain exactly one expected
  executable, and proves wrong-name/empty executables are rejected.
- CI: test and release workflows run archive content validation before upload or
  release publication.
- Verification: `make validate-build-atomicity`; `ASDF_GOLANG_VERSION=1.26.6
make build`; `ASDF_GOLANG_VERSION=1.26.6 make OS_ARCH_PAIRS="linux:amd64
linux:arm64 linux:386 linux:arm" build-all build-archive validate-archives`;
  `ASDF_GOLANG_VERSION=1.26.6 make vet test`.
- Result: passed. Full configured `make build-all` now fails closed at the
  pre-existing `windows:amd64` Unix-only daemon compile error tracked by blocked
  `PORT-01`, rather than masking the failure.
- Follow-up: `PORT-01` remains responsible for the advertised non-Linux platform
  compile/support contract.

Priority: P1

Suggested agent: build and release engineer

Dependencies: none

Primary ownership:

- `Makefile`
- a small build-validation script or test if needed
- `.github/workflows/test.yml`
- `.github/workflows/release.yml` only for artifact validation wiring

Finding:

`build-all` expands recursive makes as semicolon-separated commands, so an
intermediate target can fail while later targets continue and the recipe can
return the final target's success status. `build-single` creates its target
directory before compilation, and `build-archive` packages every matching
directory without checking that the expected binary exists and is non-empty.
This can produce a green job and empty release archive.

References:

- `Makefile:45-59`
- `Makefile:61-69`
- `.github/workflows/test.yml:20-27`
- `.github/workflows/release.yml:20-29`

Implementation requirements:

1. Stop on the first failed target or collect failures and return non-zero.
2. Build each target into a temporary output and publish the target directory
   only after successful compilation.
3. Reject archive creation when the expected platform executable is missing,
   empty, or has the wrong name.
4. Add a deterministic validation that simulates an intermediate build failure
   and proves it cannot be masked.
5. Keep existing artifact naming unless PORT-01 changes the supported matrix.

Acceptance criteria:

- An intentionally failing non-final matrix entry makes `make build-all`
  return non-zero.
- Failed builds leave no archiveable target directory or stale executable.
- Every generated archive contains exactly one expected non-empty executable.
- CI checks artifact contents before upload.
- Host and supported cross-build checks pass.

### Task PORT-01: Make The Supported Platform Contract Honest

Status: completed

Completion evidence:

- Changed files: `cmd/aiproxy/daemon.go`, `cmd/aiproxy/daemon_linux.go`,
  `cmd/aiproxy/daemon_unsupported.go`, `cmd/aiproxy/daemon_test.go`,
  `cmd/aiproxy/daemon_unsupported_test.go`, `cmd/aiproxy/lifecycle_test.go`,
  `cmd/aiproxy/main.go`, `cmd/aiproxy/signal_unix.go`,
  `cmd/aiproxy/signal_windows.go`, `internal/app/app.go`,
  `internal/app/signal_unix.go`, `internal/app/signal_windows.go`,
  `bin/download`, `AGENTS.md`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: DEC-01 option 2 is implemented. Foreground `serve` now
  cross-compiles for the advertised release matrix; daemon lifecycle process
  detachment, file locking, signal delivery, and `/proc` executable/start-time
  identity verification are isolated to Linux build-tagged files. Non-Linux
  daemon lifecycle entry points return the stable error
  `daemon lifecycle is unsupported on this platform` rather than PID-only
  behavior. App reload/shutdown signal registration is build-tagged so Windows
  builds do not require Unix signals. CLI help and agent guidance document that
  daemon lifecycle commands are Linux-only, and `bin/download` rejects local
  OS/architecture pairs that do not have release artifacts.
- Tests: Added non-Linux compile-time unsupported-platform lifecycle coverage;
  kept Linux lifecycle tests build-tagged so LIFE-01 executable/start-identity
  and locking regressions remain Linux-native.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test ./cmd/aiproxy
./internal/app`; `ASDF_GOLANG_VERSION=1.26.6 make build-all`;
  `ASDF_GOLANG_VERSION=1.26.6 GOOS=darwin GOARCH=amd64 go test -c
./cmd/aiproxy -o /tmp/opencode/aiproxy-darwin-amd64.test`;
  `ASDF_GOLANG_VERSION=1.26.6 GOOS=windows GOARCH=amd64 go test -c
./cmd/aiproxy -o /tmp/opencode/aiproxy-windows-amd64.test.exe`;
  `ASDF_GOLANG_VERSION=1.26.6 make build-archive validate-archives`;
  `ASDF_GOLANG_VERSION=1.26.6 make vet test`;
  `ASDF_GOLANG_VERSION=1.26.6 make test-race`;
  `ASDF_GOLANG_VERSION=1.26.6 make build`.
- Result: passed.
- Follow-up: DOCS-01 remains responsible for final README, design, and website
  reconciliation across all platform and dashboard contracts.

Priority: P1

Suggested agent: cross-platform Go and process-lifecycle engineer

Dependencies: BUILD-01, DEC-01

Primary ownership:

- `cmd/aiproxy/daemon.go` and new platform-specific files
- `cmd/aiproxy/main.go`
- `internal/app/app.go` signal registration boundary
- `Makefile` platform matrix
- `bin/download`
- lifecycle tests and installation documentation

Finding:

The release matrix advertises Windows, Darwin, FreeBSD, OpenBSD, and NetBSD,
but the command package unconditionally compiles Unix-specific `syscall` APIs.
Daemon identity additionally depends on Linux `/proc`, so non-Linux Unix
targets can compile portions of the CLI while `serve -d`, `status`, `stop`, and
`restart` cannot satisfy their safety contract. The asdf downloader also
accepts OS/architecture combinations for which no archive is built.

References:

- `Makefile:17-30`
- `Makefile:45-59`
- `cmd/aiproxy/daemon.go:15`
- `cmd/aiproxy/daemon.go:93-99`
- `cmd/aiproxy/daemon.go:171-191`
- `cmd/aiproxy/daemon.go:268-305`
- `cmd/aiproxy/daemon.go:332-358`
- `cmd/aiproxy/main.go:10,66`
- `internal/app/app.go:14,126-128`
- `bin/download:12-42`

Implementation requirements:

1. Implement DEC-01 without weakening executable/start-identity verification
   before signaling a process.
2. Put platform-specific process, lock, detach, signal, and identity behavior
   in build-tagged files behind a small lifecycle boundary.
3. If lifecycle commands are unsupported on a platform, return a stable,
   explicit error rather than PID-only behavior.
4. Keep the build matrix, downloader matrix, help text, README, and website in
   agreement.
5. Add compile checks for every advertised target and runtime lifecycle smoke
   tests on representative supported operating systems.

Acceptance criteria:

- Every advertised artifact cross-compiles with strict failure propagation.
- Every OS/architecture accepted by `bin/download` has a release artifact;
  unsupported pairs fail locally before download.
- Linux daemon lifecycle retains all identity and locking guarantees from
  completed task `LIFE-01`.
- Other platforms either pass native lifecycle tests or return a documented
  unsupported-platform error.
- `make vet test`, `make test-race`, and the selected cross-build matrix pass.

### Task RELEASE-01: Gate And Attest Published Artifacts

Status: completed

Completion evidence:

- Changed files: `.github/workflows/test.yml`, `.github/workflows/release.yml`,
  `.github/workflows/publish.yaml`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: Added `workflow_call` quality gating to the test workflow and
  made both tag-triggered release and image publication jobs depend on it, so
  vet, unit, race, host build, strict cross-build, build-atomicity, and archive
  validation failures block publication. Release artifacts are archive-validated
  before upload and published with `dist/checksums.txt` containing SHA-256 sums
  for every archive. Docker publication now normalizes the tag version, passes it
  explicitly as `VERSION`, verifies the built image's `aiproxy version` output,
  runs Trivy HIGH/CRITICAL scanning before registry login/push, uploads SARIF,
  and only then pushes semver tags.
- Verification: `git diff --check`; `ASDF_ACTIONLINT_VERSION=1.7.12
ASDF_SHELLCHECK_VERSION=0.11.0 actionlint .github/workflows/test.yml
.github/workflows/release.yml .github/workflows/publish.yaml`;
  `ASDF_GOLANG_VERSION=1.26.6 make vet test`; `ASDF_GOLANG_VERSION=1.26.6
make test-race`; `ASDF_GOLANG_VERSION=1.26.6 make build`;
  `ASDF_GOLANG_VERSION=1.26.6 make build-all`; `ASDF_GOLANG_VERSION=1.26.6
make build-archive validate-archives`; `ASDF_GOLANG_VERSION=1.26.6
make validate-build-atomicity`; `sha256sum ./*.tar.gz > checksums.txt && test
-s checksums.txt` from `dist/`.
- Result: passed.
- Local limitation: Docker image build/version smoke and Trivy execution were not
  run locally because the WSL environment has no `docker` command; the publish
  workflow now enforces both before push.
- Follow-up: none.

Priority: P1

Suggested agent: CI/CD and software supply-chain engineer

Dependencies: BUILD-01; PORT-01 if the platform matrix changes

Primary ownership:

- `.github/workflows/test.yml`
- `.github/workflows/release.yml`
- `.github/workflows/publish.yaml`
- `Dockerfile`
- release validation scripts only when necessary

Finding:

Tag-triggered binary and container publication run independently of the test
workflow, so failing tests do not prevent publication. Binary releases have no
content verification or checksum manifest, image scanning is disabled, and the
container workflow does not visibly pass the release version to Docker's
`VERSION` build argument, risking images that report `dev`.

References:

- `.github/workflows/test.yml:3-41`
- `.github/workflows/release.yml:3-35`
- `.github/workflows/publish.yaml:3-38`
- `Dockerfile:3-14`
- `cmd/aiproxy/main.go:20`
- `internal/app/app.go:84-86`

Implementation requirements:

1. Create or call a reusable quality workflow so binary and image publication
   cannot proceed unless vet, unit, race, host-build, and strict cross-build
   checks pass.
2. Validate every archive's expected executable before release upload.
3. Publish SHA-256 checksums at minimum; add provenance/SBOM generation when it
   fits the existing release tooling.
4. Pass the normalized release version explicitly into the Docker build and
   test the built image's `aiproxy version` output.
5. Re-enable image vulnerability scanning or document an owned, time-bounded
   exception with equivalent scanning elsewhere.

Acceptance criteria:

- A failed quality job prevents both GitHub Release and container publication.
- Every archive passes content/name/non-empty validation.
- The release contains a checksum manifest covering every artifact.
- The tagged image reports the expected version through the CLI and build-info
  surface rather than `dev`.
- Image scanning runs or an explicit approved exception and replacement control
  are documented.

### Task TOOLCHAIN-01: Align Declared Tool Versions

Status: completed

Completion evidence:

- Changed files: `.tool-versions`, `Dockerfile`, `Makefile`,
  `.github/workflows/test.yml`, `docs/toolchain.md`,
  `scripts/check-toolchain.sh`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Policy: `go.mod` remains the authoritative minimum Go release line at `1.26`;
  `.tool-versions` and Docker pin the exact supported patch on that line
  (`1.26.6`) for development, CI, and container builds. `package.json` remains
  authoritative for pnpm and `.tool-versions` now selects the same exact pnpm
  version (`10.14.0`).
- CI: The test workflow runs `make check-toolchain` before setup, and normal CI
  Go checks run through `.tool-versions`, testing the declared Go 1.26 release
  line.
- Regression: `scripts/check-toolchain.sh --self-test` includes intentional
  mismatched Go and pnpm fixtures and fails if either fixture is accepted.
- Verification: `make check-toolchain`; `git diff --check`;
  `ASDF_SHELLCHECK_VERSION=0.11.0 shellcheck scripts/check-toolchain.sh`;
  `ASDF_ACTIONLINT_VERSION=1.7.12 ASDF_SHELLCHECK_VERSION=0.11.0 actionlint
.github/workflows/test.yml`; `ASDF_GOLANG_VERSION=1.26.6 make vet test`;
  `ASDF_GOLANG_VERSION=1.26.6 make build`.
- Result: passed. An initial `actionlint` attempt with only
  `ASDF_ACTIONLINT_VERSION=1.7.12` failed because asdf had no shellcheck version
  selected; rerunning with RELEASE-01's `ASDF_SHELLCHECK_VERSION=0.11.0` passed.
- Local limitation: Docker image build was not run because this WSL environment
  has no `docker` command.

Priority: P2

Suggested agent: repository tooling maintainer

Dependencies: none; coordinate with RELEASE-01 workflow edits

Primary ownership:

- `go.mod`
- `.tool-versions`
- `Dockerfile`
- `package.json`
- setup actions and CI version checks

Finding:

The module and Docker builder declare Go 1.26 while `.tool-versions` selects Go
1.27. `package.json` declares pnpm 10.14.0 while `.tool-versions` selects pnpm
11.22.0. Normal CI therefore does not establish compatibility with the stated
Go version, and local/CI package-manager behavior can differ.

References:

- `go.mod:3`
- `Dockerfile:3`
- `.tool-versions:1-4`
- `package.json:15`
- `.github/actions/setup-tools/action.yml:1-20`

Implementation requirements:

1. Select and document one authoritative version policy.
2. Align exact tool versions or explicitly test both minimum and latest
   supported Go versions.
3. Add a lightweight CI check that catches accidental version drift.
4. Do not raise the module's minimum Go version solely to match an incidental
   local tool selection.

Acceptance criteria:

- CI tests the declared minimum Go version.
- `go.mod`, Docker, and development tooling agree or encode a documented
  minimum/latest matrix.
- pnpm metadata and selected tooling agree.
- The drift check fails on an intentional mismatched fixture or equivalent
  regression test.

## Wave 3: Runtime Ownership And Reload Health

### Task ROUTE-01: Preserve Unchanged Alias State Across Reload

Status: completed

Completion evidence:

- Changed files: `internal/modelresolver/resolve.go`,
  `internal/modelresolver/resolve_test.go`, `internal/app/app.go`,
  `internal/app/app_test.go`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: Resolver rebuilds now create a fresh catalog from the new
  runtime while reusing previous alias selector objects only when alias name,
  algorithm, and ordered targets are unchanged. App reloads retain the active
  resolver and publish a new resolver only after reload validation succeeds, so
  failed reloads leave the prior catalog and selector state active.
- Tests: Added resolver regressions for round-robin cursor preservation,
  least-connections lease preservation, algorithm/target changes getting fresh
  state, and removed target leases remaining releasable. Added app reload
  regressions for unchanged least-connections state transfer and failed reload
  rollback of selector state and model catalog.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test -race ./internal/alias ./internal/modelresolver ./internal/app ./internal/httpapi`.
- Result: passed.

Priority: P2

Suggested agent: concurrent routing and reload engineer

Dependencies: OBS-01, because both touch dispatch/routing lifecycle tests

Primary ownership:

- `internal/modelresolver/resolve.go`
- `internal/alias/alias.go` only if a state-transfer API is required
- `internal/app/app.go`
- focused resolver, alias, app, and HTTP tests

Finding:

Every dependency rebuild creates a new resolver and fresh alias selectors.
Reload therefore resets round-robin cursors and discards least-connections
counts. An active pre-reload stream retains a lease in the old selector while
post-reload requests select against zeroed counts in a new selector.

References:

- `internal/modelresolver/resolve.go:34-54`
- `internal/alias/alias.go:37-58`
- `internal/alias/alias.go:61-99`
- `internal/app/app.go:189-205`
- `internal/app/app.go:240-260`
- `internal/httpapi/handler.go:96-107`
- `internal/httpapi/handler.go:119-121`

Implementation requirements:

1. Separate immutable model catalog construction from stateful selector
   ownership.
2. Reuse selector state only when alias name, algorithm, and ordered targets are
   unchanged.
3. Create new state for changed aliases without invalidating leases held by old
   request snapshots.
4. Keep failed reload publication atomic and leave all active selector state
   unchanged.
5. Preserve direct-routing behavior and completed `ROUTE-01` lease guarantees
   from the prior remediation file.

Acceptance criteria:

- An unchanged alias retains its round-robin position across reload.
- A least-connections lease acquired before reload affects selection after an
  unchanged reload until released.
- Algorithm or target changes receive new state, while removed old selectors
  remain safe until old requests release their leases.
- A failed reload changes neither catalog nor selector state.
- `go test -race ./internal/alias ./internal/modelresolver ./internal/app ./internal/httpapi` passes.

### Task CATALOG-01: Encapsulate The Immutable Runtime Catalog

Status: completed

Completion evidence:

- Changed files: `internal/config/types.go`, `internal/config/catalog.go`,
  `internal/config/build.go`, `internal/config/capabilities.go`,
  `internal/config/validate.go`, `internal/modelresolver/resolve.go`,
  `internal/httpapi/handler.go`, `internal/httpapi/models.go`,
  `internal/httpapi/operations.go`, `internal/app/app.go`,
  `internal/dashrpc/dashrpc.go`, `internal/providerhealth/providerhealth.go`,
  and affected tests/fixtures.
- Implementation: Runtime now publishes one immutable `config.Catalog` instead
  of parallel exported provider/alias slices and lookup maps. Catalog lookup and
  ordered iteration are derived from the same private storage, and returned
  providers, models, aliases, capabilities, retry codes, and targets are copied.
- Tests: Added catalog immutability/lookup coverage and migrated fixtures to
  construct catalogs without manually synchronizing maps and ordered slices.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test -race ./internal/config
./internal/modelresolver ./internal/httpapi ./internal/app ./internal/dashrpc`.
- Result: passed.
- Additional verification: `ASDF_GOLANG_VERSION=1.26.6 go test ./...` passed.

Priority: P3

Suggested agent: Go package-boundary and data-ownership engineer

Dependencies: ROUTE-01

Primary ownership:

- `internal/config/types.go`
- `internal/config/build.go`
- a focused immutable catalog type/package
- `internal/modelresolver/`
- catalog consumers in `internal/httpapi`, `internal/app`, and `internal/dashrpc`
- affected fixtures

Finding:

Runtime providers and aliases are exported simultaneously as ordered slices and
lookup maps, and each provider exports both a model slice and model map.
Different consumers read different representations, while tests must manually
synchronize them. Because nested maps and slices remain mutable, an accidental
post-publication mutation can make routing, model listing, readiness, and
dashboard inventory disagree.

References:

- `internal/config/types.go:7-21`
- `internal/config/types.go:123-155`
- `internal/config/build.go:82-127`
- `internal/config/build.go:332-346`
- `internal/modelresolver/resolve.go:34-54`
- `internal/httpapi/handler.go:28-47`
- `internal/httpapi/models.go:22-49`
- `internal/httpapi/handler_test.go:118-200`

Implementation requirements:

1. Introduce one authoritative immutable catalog representation with ordered
   iteration and lookup methods.
2. Prevent consumers from mutating active maps or nested capability/target
   slices through returned values.
3. Let tests construct a valid catalog without manually synchronizing parallel
   fields.
4. Keep configuration decoding types and runtime catalog ownership separate if
   that avoids widening config package responsibilities.
5. Preserve order, model output, provider inheritance deep-copy isolation, and
   reload snapshot semantics.

Acceptance criteria:

- Lookup and iteration cannot diverge for providers, aliases, or models.
- Active runtime snapshots cannot be mutated through public map/slice fields.
- Resolver, model catalog, readiness, and dashboard consume the same catalog
  contract.
- Test fixtures no longer repair multiple representations manually.
- Existing API output and routing order are unchanged.
- `go test -race ./internal/config ./internal/modelresolver ./internal/httpapi ./internal/app ./internal/dashrpc` passes.

### Task PROVIDER-POLICY-01: Evaluate A Single Provider Capability Registry

Status: completed

Completion evidence:

- Changed files: `internal/config/capabilities.go`,
  `internal/config/capabilities_test.go`, `internal/config/validate.go`,
  `internal/provider/provider.go`, `internal/provider/openai.go`,
  `internal/provider/anthropic.go`, `internal/provider/gemini.go`,
  `internal/provider/provider_test.go`, `internal/httpapi/operations.go`,
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Investigation: a prospective provider type with a new capability previously
  required coordinated edits to config valid-type validation, provider-specific
  capability validation, default capability selection, adapter dispatch/default
  endpoint handling, and tests across three packages. A prospective operation
  required edits to HTTP route mapping, operation-to-capability mapping, OpenAI
  upstream path mapping, provider operation implementation switches, and tests.
- Decision: did not add a cross-package all-in-one provider registry. Because
  `provider` already imports `config`, making config validation consume adapter
  function descriptors would require a new shared policy package or a broader
  type-boundary reversal for only four built-in providers. The retained change
  instead keeps package ownership explicit while removing duplicated policy
  where it reduced extension cost without an import cycle.
- Implementation: config provider capability/default/base-url policy is now one
  descriptor table used by validation and effective defaults; provider dispatch
  and default endpoint policy are descriptor-backed; operation name, public HTTP
  path, OpenAI upstream path, and required capability are driven by one provider
  operation descriptor table consumed by `httpapi`.
- Extension cost after refactor: adding an operation now requires one operation
  descriptor registration plus provider implementation/tests, rather than
  unrelated route, capability, string, and OpenAI-path switch edits. Adding a
  provider type still intentionally requires one config policy registration and
  one provider adapter registration plus implementation/tests, with parity tests
  failing if either side is missing.
- Tests: Added `TestProviderTypePoliciesCoverCapabilityMatrix`,
  `TestDefaultCapabilitiesReturnsCopy`,
  `TestProviderDescriptorsCoverConfiguredProviderTypes`, and
  `TestOperationDescriptorsDriveHTTPPathNameAndCapability`.
- Verification: `git diff --check -- internal/config/capabilities.go
internal/config/capabilities_test.go internal/config/validate.go
internal/provider/provider.go internal/provider/openai.go
internal/provider/anthropic.go internal/provider/gemini.go
internal/provider/provider_test.go internal/httpapi/operations.go
docs/tasks/20260823-112437-codebase-health-follow-up.md`;
  `ASDF_GOLANG_VERSION=1.26.6 go test ./internal/config ./internal/provider
./internal/httpapi`.
- Result: passed.
- Follow-up: none.

Priority: P3

Suggested agent: provider architecture engineer

Dependencies: STREAM-01; no implementation dependency on CATALOG-01

Primary ownership:

- investigation and design note first
- `internal/provider/provider.go`
- `internal/config/capabilities.go`
- `internal/config/validate.go`
- `internal/httpapi/operations.go`
- provider tests only if the investigation justifies implementation

Finding:

Provider dispatch, valid types, supported capabilities, default capabilities,
and operation mapping are encoded in separate switch tables across packages.
Anthropic and Gemini files also combine transport, multiple operation
translations, error handling, stream translation, and usage extraction. This
is a concrete extension hotspot, but with four built-in providers a registry or
subpackage split may add more indirection than value.

References:

- `internal/provider/provider.go:118-126`
- `internal/provider/provider.go:176-193`
- `internal/config/validate.go:149-180`
- `internal/config/validate.go:263-284`
- `internal/config/capabilities.go:48-58`
- `internal/httpapi/operations.go:11-31`
- `internal/httpapi/operations.go:55-71`
- `internal/provider/anthropic.go:48-541`
- `internal/provider/gemini.go:98-690`

Implementation requirements:

1. Start with an investigation using one prospective provider or operation and
   enumerate every current switch/table edit required.
2. Prototype a compile-time internal descriptor that owns adapter behavior,
   supported operations/capabilities, defaults, and default endpoint policy.
3. Proceed with implementation only if it removes duplicated policy without an
   import cycle or loss of compile-time test coverage.
4. Do not add dynamic plugin loading or split files solely to reduce line count.

Acceptance criteria:

- The task records the measured current extension cost and a keep/refactor
  decision.
- If retained, table tests prove every configured provider type has matching
  adapter and capability policy.
- If refactored, adding the prototype provider/operation requires one policy
  registration plus its implementation and tests, not edits across unrelated
  switches.
- Existing provider behavior and `go test ./internal/config ./internal/provider ./internal/httpapi` pass.

## Wave 4: Security Contracts, Integration, And Documentation

### Task DASH-01: Implement A Coherent Remote Dashboard Transport

Status: completed

Resolution: DEC-02 selected option 3. The dashboard command is local-only;
unsupported remote HTTPS dashboard access must not be documented as deployable.

Completion evidence:

- Changed files: `cmd/aiproxy/dashboard.go`, `cmd/aiproxy/dashboard_test.go`,
  `internal/config/validate.go`, `internal/config/load_test.go`, `README.md`,
  `docs/design.md`, `website/docs/configuration.md`,
  `website/docs/deployment.md`, `website/docs/operations.md`,
  `docs/tasks/20260804-125911-codebase-health-review-remediation.md`,
  `docs/tasks/20260809-132127-remediation-decision-closure.md`, and
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: Listener validation rejects URL-shaped addresses as invalid
  TCP binds; the dashboard CLI derives only local plain-HTTP URLs, maps wildcard
  binds to loopback, and refuses concrete non-loopback listener hosts under the
  local-only contract. The legacy `allow_insecure_remote` attribute remains
  rejected as unsupported.
- Tests: Added/retained regressions for URL-shaped listener validation failure,
  non-loopback dashboard CLI rejection, loopback dashboard CLI access, and
  unsupported insecure-remote override rejection.
- Documentation: README, design docs, website docs, and historical task notes no
  longer advertise non-loopback HTTPS or insecure override dashboard access as a
  deployable path; remote dashboard support is documented as requiring a future
  explicit transport design.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test ./cmd/aiproxy
./internal/config ./internal/app ./internal/httpapi`.
- Result: passed.

Priority: P1

Suggested agent: HTTP transport and dashboard security engineer

Dependencies: DEC-02; coordinate config edits with DOCS-01

Primary ownership:

- dashboard/listener config schema, build, and validation
- `cmd/aiproxy/dashboard.go`
- `internal/app/app.go`
- dashboard transport tests
- dashboard operations/configuration documentation

Finding:

The dashboard CLI accepts an `https://` listener address as secure, but the app
passes the same value directly to `net.Listen("tcp", ...)` and serves plain HTTP
with `Server.Serve`. A URL-shaped listener cannot bind, while a normal bind
address behind an HTTPS reverse proxy is always interpreted as HTTP by the CLI.
The documented non-loopback HTTPS path is therefore not representable by the
current configuration model.

References:

- `cmd/aiproxy/dashboard.go:109-145`
- `internal/config/validate.go:95-99`
- `internal/app/app.go:108-113`
- `internal/app/app.go:130-135`
- `website/docs/operations.md:251-262`
- `website/docs/configuration.md:356-370`
- `docs/tasks/20260809-132127-remediation-decision-closure.md:213-220`

Implementation requirements:

1. Implement DEC-02 as one explicit bind-address and transport contract.
2. Reject URL-shaped bind addresses during validation unless native TLS is
   implemented and the schema explicitly supports it.
3. Do not overload the TCP bind address as a remote dashboard URL.
4. Keep loopback plain HTTP safe defaults and bearer authentication.
5. Reject non-loopback plain-HTTP dashboard CLI access under the local-only
   contract.

Acceptance criteria:

- Unsupported secure non-loopback dashboard access is no longer documented as a
  deployable path.
- URL-shaped listener bind addresses fail during config validation with a clear
  error.
- Plain HTTP remote access remains rejected by default.
- CLI URL derivation, server listener behavior, README, design, and website all
  describe the same contract.
- `go test ./cmd/aiproxy ./internal/config ./internal/app ./internal/httpapi` passes.

### Task SECURITY-01: Standardize Secret Comparison And Dashboard Route Tests

Status: completed

Completion evidence:

- Changed files: `internal/httpapi/dashboard.go`,
  `internal/httpapi/dashboard_test.go`,
  `docs/tasks/20260809-132127-remediation-decision-closure.md`, and
  `docs/tasks/20260823-112437-codebase-health-follow-up.md`.
- Implementation: dashboard bearer-token validation now keeps strict `Bearer `
  scheme parsing and compares the parsed token with
  `subtle.ConstantTimeCompare`, matching the existing metrics token gate. Static
  API-client token map lookup was not changed.
- Tests: `TestDashboardAuthFailureRateLimitsRepeatedBadTokens` now enumerates
  both snapshot and logs routes, verifies invalid credentials return `401`
  through the burst and `429` with `Retry-After` after the burst, and verifies
  valid credentials neither consume nor get blocked by the invalid-attempt
  limiter.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test -race ./internal/httpapi
./internal/auth`.
- Result: passed.
- Follow-up: `FU-26-01` in
  `docs/tasks/20260809-132127-remediation-decision-closure.md` marked
  completed with evidence.

Priority: P3

Suggested agent: HTTP authentication hardening engineer

Dependencies: DASH-01 only if it changes dashboard route wiring; otherwise none

Primary ownership:

- `internal/httpapi/dashboard.go`
- `internal/httpapi/dashboard_test.go`
- `internal/auth/auth.go` only after measuring and preserving client lookup
  behavior
- prior follow-up status in
  `docs/tasks/20260809-132127-remediation-decision-closure.md`

Finding:

Metrics token verification uses `subtle.ConstantTimeCompare`, while dashboard
token verification uses direct string equality. This is defense-in-depth rather
than a demonstrated remote exploit. Separately, prior follow-up `FU-26-01`
remains pending because repeated-invalid-token coverage exercises only the
snapshot route, not the logs route that is expected to share the same gate.

References:

- `internal/httpapi/handler.go:444-457`
- `internal/httpapi/dashboard.go:77-95`
- `internal/httpapi/dashboard.go:100-129`
- `internal/httpapi/dashboard_test.go:178-221`
- `docs/tasks/20260809-132127-remediation-decision-closure.md:540-566`

Implementation requirements:

1. Use constant-time comparison for the dashboard bearer secret after strict
   scheme parsing.
2. Keep malformed, missing, and wrong credentials indistinguishable at the HTTP
   response boundary.
3. Convert the auth/rate-limit test to enumerate both snapshot and logs routes.
4. Mark `FU-26-01` completed only after its original acceptance criteria pass.
5. Treat static API-client token lookup separately; do not replace its map with
   an O(client-count) scan without a measured threat/performance decision.

Acceptance criteria:

- Dashboard and metrics secret comparisons use constant-time primitives.
- Snapshot and logs return the same `401`, then `429` plus `Retry-After`, under
  repeated invalid credentials.
- Valid credentials are not consumed by the invalid-attempt limiter.
- `go test -race ./internal/httpapi ./internal/auth` passes.

### Task INHERIT-01: Complete Provider Inheritance Integration Evidence

Status: completed

Completion evidence:

- Changed files: `internal/config/load_test.go`, `internal/app/app_test.go`,
  `internal/e2e/e2e_test.go`, `cmd/aiproxy/configure.go`,
  `cmd/aiproxy/configure_test.go`,
  `docs/tasks/20260822-160713-provider-inheritance.md`, and this task file.
- Tests: Added `TestLoadDerivedProviderAcceptsInlineLocalCredential`,
  `TestEndToEndDerivedProviderDirectAndAliasRouting`,
  `TestDerivedProviderIdentityAcrossRuntimeSurfaces`,
  `TestReloadAddsChangesRemovesDerivedProviderAndRollsBackInvalidCandidate`,
  and `TestConfigureProviderInteractiveChoosesDerivedBaseProvider`.
- Coverage: Direct and alias HTTP requests prove derived local credentials,
  inherited endpoint/model mapping, and derived provider routing identity;
  runtime checks prove derived identity in health state, Prometheus metrics,
  in-process accounting, and dashboard snapshots; reload coverage proves
  successful add/change/remove and invalid candidate rollback with the prior
  derived provider still routable.
- Implementation: Interactive provider configuration now selects `extends` from
  `none` plus eligible existing concrete, enabled base providers of the selected
  provider type, while non-interactive `--extends` behavior remains validated by
  the existing contract.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test ./internal/config
./internal/configedit ./cmd/aiproxy ./internal/app ./internal/e2e`;
  `ASDF_GOLANG_VERSION=1.26.6 make vet test`;
  `ASDF_GOLANG_VERSION=1.26.6 make test-race`;
  `ASDF_GOLANG_VERSION=1.26.6 make build`.
- Result: passed.

Priority: P2

Suggested agent: config, reload, and provider integration engineer

Dependencies: ROUTE-01 if alias reload semantics are tested in the same cases

Primary ownership:

- `internal/config/load_test.go`
- `internal/app/app_test.go`
- `internal/e2e/`
- `cmd/aiproxy/configure.go`
- `cmd/aiproxy/configure_test.go`
- completion evidence in
  `docs/tasks/20260822-160713-provider-inheritance.md`

Finding:

Provider inheritance has useful config-level flattening tests but was marked
completed even though all recorded verification commands failed. Its stated
acceptance criteria also require runtime routing, independent identity, reload
rollback, both credential forms, and an interactive base-provider choice. The
current interactive field is free text and existing tests are primarily
config-level or non-interactive.

References:

- `docs/tasks/20260822-160713-provider-inheritance.md:215-251`
- `docs/tasks/20260822-160713-provider-inheritance.md:278-285`
- `internal/config/load_test.go:221-365`
- `internal/app/app_test.go:181-231`
- `internal/e2e/e2e_test.go:72-177`
- `cmd/aiproxy/configure.go:1554-1564`
- `cmd/aiproxy/configure_test.go:271-342`

Implementation requirements:

1. Add direct and alias HTTP tests proving the derived provider's local
   credential, inherited endpoint/model mapping, and own provider identity.
2. Verify health, metrics/accounting, and dashboard identity use the derived
   provider name independently from its base.
3. Test successful add/change/remove reload and invalid candidate rollback.
4. Add an explicit successful inline `api_key` case alongside `api_key_ref`.
5. Implement the promised eligible-base interactive choice or revise the task
   contract with maintainer approval and rationale.
6. Run and record every verification command required by the original task
   before retaining its `completed` status.

Acceptance criteria:

- Every unverified original acceptance criterion has a named test or an
  explicitly approved contract revision.
- A failed inherited-provider reload leaves the prior runtime routable.
- Derived provider identity remains independent across routing, health,
  accounting, metrics, and dashboard snapshots.
- `go test ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/app ./internal/e2e` passes.
- `make vet test`, `make test-race`, and `make build` pass and are recorded in
  the original task file.

### Task TEST-01: Add Hermetic Binary-Level Integration Coverage

Status: completed

Completion evidence:

- Changed files: `internal/integration/doc.go`,
  `internal/integration/binary_test.go`, `Makefile`,
  `.github/workflows/test.yml`, `README.md`, and this task file.
- Implementation: Added a Linux CI hermetic binary integration target that builds
  `dist/aiproxy`, starts the built binary with temporary HCL config, local
  listener addresses, and httptest upstreams, and keeps real-provider sandbox
  tests separate from normal CI.
- Coverage: Binary-level tests exercise readiness, health, graceful interrupt
  shutdown, API bearer auth, metrics bearer auth, OpenAI-compatible streaming,
  derived-provider routing with inherited model mapping and local credentials,
  successful config edit plus `SIGHUP` publication, and failed-candidate reload
  rollback.
- Routing coverage: HTTP-level alias tests verify multi-target round-robin,
  least-connections with an active stream lease, default transient `5xx` retry,
  configured retryable `429`, and no retry for ordinary `400`.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 make integration`;
  `ASDF_GOLANG_VERSION=1.26.6 make vet test`;
  `ASDF_GOLANG_VERSION=1.26.6 make test-race`.
- Result: passed.

Priority: P2

Suggested agent: Go integration and reliability test engineer

Dependencies: APP-01, OBS-01, ROUTE-01, INHERIT-01

Primary ownership:

- a separate hermetic integration package or test target
- `Makefile`
- `.github/workflows/test.yml`
- local provider stubs under `internal/e2e/` or the new package
- testing documentation

Finding:

Current e2e tests build `App` but serve only `a.Server.Handler` through
`httptest.NewServer`. They do not exercise the real listener, readiness
callback, graceful shutdown, `SIGHUP` reload, daemon lifecycle, packaged
binary, or multi-target alias selection/failover. The existing integration tier
is intentionally skipped pending sandbox services even though these scenarios
can use hermetic local stubs.

References:

- `internal/e2e/e2e_test.go:19-37`
- `internal/e2e/e2e_test.go:114-177`
- `internal/e2e/e2e_test.go:572-678`
- `internal/app/app.go:108-160`
- `.github/workflows/test.yml:43-45`
- `README.md:309-315`
- `docs/design.md:864-895`

Implementation requirements:

1. Separate hermetic binary/app integration tests from optional real-provider
   sandbox tests.
2. Start `App.RunReady` on an actual local listener or execute the built binary
   with a temporary config and local upstream stubs.
3. Exercise readiness, graceful shutdown, config edit plus `SIGHUP`, API auth,
   metrics auth, streaming, and derived-provider routing.
4. Exercise multi-target round-robin, least-connections behavior, transient
   retry, configured retryable 4xx, and no retry for ordinary 4xx.
5. Keep tests deterministic, credential-free, and suitable for Linux CI.

Acceptance criteria:

- The hermetic integration target runs in normal CI without external services.
- At least one test crosses the actual listener and lifecycle boundary.
- Reload tests prove successful publication and failed-candidate rollback.
- Multi-target routing and retry semantics are verified through HTTP, not only
  package-local calls.
- `make vet test`, `make test-race`, and the new integration target pass.

### Task TEST-02: Close Disabled-Provider Alias Coverage

Status: completed

Completion evidence:

- Changed files: `internal/config/load_test.go`,
  `docs/tasks/20260809-132127-remediation-decision-closure.md`, and this task
  file.
- Tests: Added `TestLoadAliasPrunesDisabledProviderTargets` for mixed aliases
  with disabled targets pruned while enabled targets remain in declaration order,
  and `TestLoadRejectsAliasWithOnlyDisabledProviderTargets` for all-disabled
  aliases failing validation with `alias "chat": at least one target is
required`.
- Verification: `ASDF_GOLANG_VERSION=1.26.6 go test ./internal/config`.
- Result: passed.

Priority: P3

Suggested agent: configuration contract test engineer

Dependencies: none

Primary ownership:

- `internal/config/load_test.go`
- prior follow-up status in
  `docs/tasks/20260809-132127-remediation-decision-closure.md`

Finding:

At creation time, prior follow-up `FU-26-02` remained pending. Build logic prunes disabled
providers from alias targets, but no explicit test covers a mixed alias or an
alias whose targets are all removed.

References:

- `internal/config/build.go:90-96`
- `internal/config/build.go:118-124`
- `internal/config/validate.go:223-258`
- `internal/config/load_test.go:755-803`
- `docs/tasks/20260809-132127-remediation-decision-closure.md:568-595`

Implementation requirements:

1. Add a mixed enabled/disabled target case and assert only the enabled target
   remains.
2. Add an all-disabled target case and assert the documented no-target
   validation result.
3. Mark `FU-26-02` completed only after its original acceptance criteria pass.

Acceptance criteria:

- Mixed aliases retain enabled targets in declaration order.
- All-disabled aliases fail with the expected stable validation error.
- `go test ./internal/config` passes.

### Task DOCS-01: Reconcile Public Behavior And Support Contracts

Status: completed

Completion evidence:

- Changed files: `AGENTS.md`, `README.md`, `docs/design.md`,
  `scripts/check-doc-contracts.sh`, `website/docs/operations.md`,
  `website/docs/providers-and-routing.md`, and this task file.
- Documentation: README, design, website, and agent guidance share the current
  public endpoint/provider and provider capability contract; local usage
  accounting, retryable `4xx` alias statuses, provider-health separation,
  restart-required reload changes, Linux-only daemon lifecycle, and local-only
  dashboard transport are documented consistently.
- Contract check: `make docs-contract` now validates every marked high-drift
  matrix, including the website provider capability table.
- Verification: `make docs-contract`; `pnpm typecheck` from `website/`; `pnpm
build` from `website/`; `ASDF_GOLANG_VERSION=1.26.6 go test
./internal/config ./internal/httpapi ./cmd/aiproxy ./internal/provider`.
- Result: passed.

Priority: P2

Suggested agent: technical writer with code-review ownership

Dependencies: DEC-01, DEC-02; implementation tasks that change their contracts

Primary ownership:

- `README.md`
- `docs/design.md`
- `AGENTS.md`
- `website/docs/`
- a lightweight documentation-contract check if practical

Finding:

Public documents contain conflicting current/future endpoint lists, describe
implemented accounting/rate-limit/audio/reload features as absent or deferred,
and state aliases retry only transport/5xx even though configured 400-599
statuses can be retried. Dashboard restart wording and HTTPS support also vary.
Markdown-only changes are excluded from the Go test workflow, so root contract
drift has no automated gate.

References:

- `README.md:20-44`
- `README.md:61-72`
- `README.md:431-480`
- `docs/design.md:28-36`
- `docs/design.md:59-88`
- `docs/design.md:843-895`
- `docs/design.md:1002-1005`
- `AGENTS.md:95-97`
- `internal/httpapi/dispatch.go:83-89`
- `internal/httpapi/dispatch.go:202-213`
- `website/docs/api-reference.md:68-74`
- `website/docs/providers-and-routing.md:77-82`
- `.github/workflows/test.yml:3-9`

Implementation requirements:

1. Establish one current endpoint/provider/capability matrix and use it across
   README, design, website, and agent guidance.
2. Distinguish external billing/quota systems from the implemented local usage
   accounting endpoint.
3. Document configurable retry statuses, including retryable 4xx and their
   separation from provider-health mutation.
4. State precisely which reload changes require restart.
5. Incorporate DEC-01 and DEC-02 outcomes; do not document unsupported
   platform or HTTPS behavior.
6. Add a lightweight contract checklist or table-driven docs check for the
   highest-drift endpoint and provider matrices.

Acceptance criteria:

- README, design, website, AGENTS, config validation, and runtime agree on
  endpoint support, provider capabilities, alias retry, reload, dashboard
  transport, and platform support.
- Obsolete milestone text is removed or explicitly labeled historical.
- Documentation-only pull requests run an appropriate docs/contract check.
- Website build and relevant Go contract tests pass.

## Deferred Decisions Requiring Maintainer Input

### DEC-01: Supported Operating Systems And Lifecycle Commands

Status: completed

Completion evidence:

- Decision selected and recorded before dependent implementation: option 2,
  foreground cross-platform support with Linux-only daemon lifecycle commands.
- Verified by PORT-01, DOCS-01, and REVIEW-01 completion evidence covering
  strict cross-builds, non-Linux unsupported lifecycle behavior, Linux lifecycle
  safety, downloader/build-matrix agreement, and documentation alignment.

Owner: maintainer/release

Decision:

Selected option 2. The foreground server ships cross-platform for the advertised
artifact matrix; daemon lifecycle commands are Linux-only and return
`daemon lifecycle is unsupported on this platform` elsewhere.

Considered options:

1. Implement safe native daemon lifecycle behavior for every advertised OS.
2. Ship the foreground server cross-platform but make daemon lifecycle commands
   explicitly Linux-only through build-tagged implementations.
3. Make the entire binary/release Linux-only and remove other artifacts.

Recommendation: option 2. It preserves useful cross-platform foreground usage
without weakening Linux process identity or requiring a large native process
manager implementation immediately.

Decision record:

- Supported build targets: `linux:amd64`, `linux:arm64`, `linux:386`,
  `linux:arm`, `windows:amd64`, `windows:386`, `darwin:amd64`,
  `darwin:arm64`, `freebsd:amd64`, `freebsd:arm64`, `openbsd:amd64`,
  `openbsd:arm64`, and `netbsd:amd64`.
- Supported runtime targets: foreground `serve` on all advertised targets.
- Lifecycle command availability by OS: full daemon lifecycle on Linux only;
  non-Linux returns the documented unsupported-platform error.
- CI compile and runtime-test expectations: strict cross-build and archive
  validation cover the full matrix; Linux CI runs hermetic binary integration.

### DEC-02: Secure Remote Dashboard Address Model

Status: completed

Completion evidence:

- Decision selected and recorded before dependent implementation: option 3,
  local-only dashboard command with unsupported remote HTTPS promise removed.
- Verified by DASH-01, DOCS-01, and REVIEW-01 completion evidence covering
  URL-shaped listener rejection, non-loopback dashboard CLI refusal, loopback
  bearer-auth behavior, and documentation alignment.

Owner: maintainer/security and operations

Decision:

Selected option 3. The dashboard command is local-only; unsupported remote HTTPS
dashboard access is removed from the deployable contract.

Considered options:

1. Add native TLS listener configuration and certificate ownership.
2. Keep the bind address as host/port and add a separate dashboard public/base
   URL for reverse-proxy TLS deployments.
3. Declare the dashboard command local-only and remove the unsupported remote
   HTTPS promise.

Rationale: option 3 is the smallest safe correction for the current schema. Do
not continue treating one string as both a TCP bind address and an HTTP URL.

Decision record:

- TLS termination owner: none in the current dashboard command.
- Bind address versus externally reachable URL: listener addresses remain TCP
  bind addresses only.
- Token transport requirements and insecure override policy: loopback plain HTTP
  with bearer authentication remains supported; remote dashboard access and
  insecure remote override are unsupported.
- Proxy/header trust assumptions: none for dashboard CLI access.

## Parallelization Guidance

| Agent | Tasks                     | Sequencing                                                           |
| ----- | ------------------------- | -------------------------------------------------------------------- |
| A     | STREAM-01                 | Independent provider protocol work.                                  |
| B     | OBS-01, then ROUTE-01     | Exclusive ownership of dispatch/routing lifecycle during changes.    |
| C     | APP-01                    | Independent app cleanup work; coordinate app tests with ROUTE-01.    |
| D     | BUILD-01, then RELEASE-01 | Own build/archive/release workflow integrity.                        |
| E     | PORT-01                   | Starts after BUILD-01 and DEC-01.                                    |
| F     | TOOLCHAIN-01              | May run independently, but workflow merges sequence with RELEASE-01. |
| G     | DASH-01, then SECURITY-01 | Starts after DEC-02; owns dashboard transport/auth files.            |
| H     | INHERIT-01                | Coordinate reload tests with ROUTE-01.                               |
| I     | TEST-02                   | Independent config-only coverage task.                               |
| J     | CATALOG-01                | Starts after ROUTE-01 to avoid moving routing ownership mid-fix.     |
| K     | PROVIDER-POLICY-01        | Investigation can run early; implementation follows STREAM-01.       |
| L     | TEST-01                   | Runs after runtime fixes settle.                                     |
| M     | DOCS-01                   | Final contract pass after DEC-01 and DEC-02.                         |

Shared hotspots:

- Sequence OBS-01 before ROUTE-01 for `internal/httpapi/dispatch.go` and alias
  lifecycle tests.
- Sequence ROUTE-01 before CATALOG-01 for resolver/catalog ownership.
- Coordinate APP-01, ROUTE-01, and INHERIT-01 changes to
  `internal/app/app.go` and `internal/app/app_test.go`; do not edit them in
  parallel.
- Sequence BUILD-01, PORT-01, and RELEASE-01 changes to `Makefile` and release
  workflows.
- Sequence DASH-01 and SECURITY-01 if both touch dashboard route tests.
- DOCS-01 should merge after behavior/platform decisions rather than guessing
  their outcomes.
- Do not run repository-wide formatting, cross-builds, or release artifact
  generation concurrently in one worktree.

## Final Integration Task

### Task REVIEW-01: Independently Verify Follow-Up Remediation

Status: completed

Priority: P1

Suggested agent: independent senior security, reliability, and release reviewer

Dependencies: all selected implementation tasks; DEC-01 and DEC-02 resolved or
explicitly deferred with residual risk

Primary ownership:

- review and verification evidence
- focused corrective edits require a new task or maintainer approval
- this task file and linked prior task status/evidence

Finding:

The plan crosses streaming ownership, metrics, reload state, process
portability, release publication, dashboard security, and public contracts.
Package-local passing tests cannot prove these boundaries remain coherent.

Implementation requirements:

1. Verify every completed acceptance criterion against current tests and source,
   not completion notes alone.
2. Re-test OpenAI-compatible SSE bounds, alias stream metric counts, stream
   cancellation, alias lease release, reload state retention, and every
   `RunReady` cleanup path.
3. Confirm all advertised release targets compile strictly and every artifact
   contains the expected binary.
4. Verify release/image publication cannot bypass quality checks and that
   artifacts report the release version.
5. Verify dashboard authentication and transport policy through alternate
   entry paths, including reverse-proxy/TLS behavior selected by DEC-02.
6. Confirm no secret or mutable internal catalog data crosses logs, dashboard,
   metrics, model-list, error, or billing boundaries.
7. Confirm request-controlled body, line, event, collection, metric-label, and
   retained-state inputs remain explicitly bounded.
8. Confirm all prior pending follow-ups and provider-inheritance evidence are
   accurately reflected in their original task files.
9. Record any residual issue as a uniquely numbered task rather than silently
   accepting it.

Acceptance criteria:

- `make vet test` passes.
- `make test-race` passes.
- `make build` passes.
- The strict selected cross-build and archive validation pass.
- The hermetic integration target passes.
- Website/docs checks pass.
- Each completed task has traceable tests or runtime/release evidence.
- No unrelated worktree changes were reverted or included.

Completion evidence:

- Source/test review: verified completed task evidence against current source and
  tests for bounded OpenAI-compatible SSE observation; single alias stream
  upstream metric finalization; downstream cancellation classification; alias
  lease release; reload selector/catalog retention; `RunReady` cleanup paths;
  strict build/archive scripts; release and image publication gating; dashboard
  auth and local-only transport; catalog immutability; model list, dashboard,
  metrics, billing, error, and log boundaries; request body, SSE line/event,
  retained log/accounting, and route metric-label bounds; `FU-26-01`,
  `FU-26-02`, and provider-inheritance evidence in linked task files.
- Corrective edit: marked `DEC-01` and `DEC-02` completed in this task file and
  recorded the implemented DEC-01 option-2 decision because dependent completed
  tasks and current docs/source/tests already reflect that resolved contract.
- Final cleanup: made creation-time references to pending `FU-26-01` and
  `FU-26-02` explicitly historical so the completed task file no longer reads as
  if those linked follow-ups are still open.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 go version`.
- Result: `go version go1.26.6 linux/amd64`.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make vet test`.
- Result: passed; all packages reported `ok` or `[no test files]`.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make test-race`.
- Result: passed; all packages reported `ok` or `[no test files]`.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make build`.
- Result: passed; built `dist/aiproxy` with version `25c9ded-dirty`.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make build-all`.
- Result: passed; built all advertised `OS_ARCH_PAIRS` targets.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make build-archive validate-archives`.
- Result: passed; created and validated 13 release archives, each with the
  expected non-empty executable name.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make validate-build-atomicity`.
- Result: passed; simulated failed intermediate target was not masked, stale
  output was removed, valid archives passed, wrong-name and empty executables
  were rejected.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make integration`.
- Result: passed; `internal/integration` completed successfully.
- Verified: `make docs-contract`.
- Result: passed; documentation contract matrices match.
- Verified: `pnpm typecheck` from `website/`.
- Result: passed; `tsc` completed successfully.
- Verified: `pnpm build` from `website/`.
- Result: passed; Docusaurus generated static files in `website/build`.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make build VERSION=review-verify &&
dist/aiproxy version`.
- Result: passed; binary printed `review-verify`.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make build-single
OS_ARCH=linux:amd64 VERSION=review-verify && dist/linux-amd64/aiproxy
version`.
- Result: passed; cross-built Linux artifact printed `review-verify/linux-amd64`.
- Verified: `make docker-build VERSION=review-verify`.
- Result: passed; built `aiproxy:review-verify` and `aiproxy:latest`.
- Verified: `docker run --rm --entrypoint /usr/local/bin/aiproxy
aiproxy:review-verify version`.
- Result: passed; image printed `review-verify`.
- Verified: `command -v trivy`.
- Result: failed with no output; local scanner is unavailable, but
  `.github/workflows/publish.yaml` enforces Trivy HIGH/CRITICAL scanning before
  registry login and push.
- Verified: `ASDF_ACTIONLINT_VERSION=1.7.12 ASDF_SHELLCHECK_VERSION=0.11.0
actionlint .github/workflows/test.yml .github/workflows/release.yml
.github/workflows/publish.yaml .github/workflows/docs-contract.yml`.
- Result: passed with no diagnostics.
- Verified: `git diff --check`.
- Result: passed with no diagnostics.
- Verified focused regressions: `ASDF_GOLANG_VERSION=1.26.6 go test -race
./internal/provider ./internal/httpapi ./internal/app ./internal/modelresolver
./internal/alias -run
'Test(OpenAIStreamObservationOverflowContinuesPassThrough|OpenAIStreamPreservesBytesAndObservesFragmentedEOFEvent|OpenAIStreamRecordsErrorEvent|HandlerMetricsCountAliasStreamingAttemptOnceAtCompletion|HandlerMetricsClassifyAliasMidStreamUpstreamErrorOnce|HandlerMetricsClassifyAliasDownstreamCancellationOnce|WriteResultStreamingDownstreamErrorClosesUpstream|ReloadPreservesUnchangedAliasLeastConnectionsLease|FailedReloadLeavesAliasSelectorAndCatalogUnchanged|RunReadyClosesResourcesOn|NewWithPreviousPreservesUnchangedLeastConnectionsLease|NewWithPreviousCreatesNewStateForChangedAlias|NewWithPreviousDoesNotInvalidateRemovedAliasLeases|LeastConnectionsReleaseReuses)$'`.
- Result: passed for `internal/provider`, `internal/httpapi`, `internal/app`,
  `internal/modelresolver`, and `internal/alias`.
- Verified focused dashboard regressions: `ASDF_GOLANG_VERSION=1.26.6 go test
-race ./cmd/aiproxy ./internal/httpapi ./internal/config ./internal/app -run
'Test(DashboardRejectsNonLoopbackPlainHTTP|DashboardAllowsLoopbackPlainHTTPWithoutOverride|DashboardRejectsInsecureRemoteOverride|RunDashboardAutoreadsPersistedTokenWhenBlockHasNoToken|RunDashboardErrorsWhenBlockAbsent|RunDashboardErrorsWhenNoServerRunning|DashboardAuthFailureRateLimitsRepeatedBadTokens|DashboardSnapshotEndpointRejectsUnauthenticatedRequests|DashboardLogsEndpointReturnsNewEntries|LoadRejectsURLShapedListenerAddress|LoadRejectsDashboardInsecureRemoteWithMintedToken|DashboardEndpointAbsentWithoutDashboardBlock|DashboardEndpointRejectsMissingToken|ReloadUpdatesDashboardSnapshotEndpoint)$'`.
- Result: passed for `cmd/aiproxy`, `internal/httpapi`, `internal/config`, and
  `internal/app`.
- Verified provider-inheritance evidence: `ASDF_GOLANG_VERSION=1.26.6 go test
-race ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/app
./internal/e2e -run
'Test(LoadDerivedProviderInheritsBaseAndUsesLocalCredential|LoadDerivedProviderAcceptsInlineLocalCredential|LoadRejectsInvalidDerivedProviders|EndToEndDerivedProviderDirectAndAliasRouting|DerivedProviderIdentityAcrossRuntimeSurfaces|ReloadAddsChangesRemovesDerivedProviderAndRollsBackInvalidCandidate|ConfigureProviderNonInteractiveCreatesDerivedProvider|ConfigureProviderInteractiveChoosesDerivedBaseProvider|RenderProviderBlockDerivedStaysCompact|AvailableProviderModelsIncludesDerivedProviderModels)$'`.
- Result: passed for `internal/config`, `internal/configedit`, `cmd/aiproxy`,
  `internal/app`, and `internal/e2e`.
- Verified prior follow-ups: `ASDF_GOLANG_VERSION=1.26.6 go test -race
./internal/httpapi ./internal/config -run
'Test(DashboardAuthFailureRateLimitsRepeatedBadTokens|LoadAliasPrunesDisabledProviderTargets|LoadRejectsAliasWithOnlyDisabledProviderTargets)$'`.
- Result: passed for `internal/httpapi` and `internal/config`.
- Verified catalog/exposure/bounds regressions: `ASDF_GOLANG_VERSION=1.26.6 go
test -race ./internal/config ./internal/dashrpc ./internal/httpapi
./internal/observability ./internal/accounting -run
'Test(CatalogBuildsLookupFromOrderedProvidersAliasesAndModels|CatalogReturnedValuesCannotMutateStoredCatalog|DashboardSnapshotEndpointServesJSON|DashboardLogsEndpointReturnsNewEntries|HandlerListModelsIncludesProvidersAndAliases|HandlerListModelsFiltersUnauthorizedModels|MetricsPathLabelUsesClosedRouteSet|HandlerMetricsDisabledWithoutToken|HandlerAccountingCollapsesUnknownModels|HandlerAccountingCollapsesForbiddenModels|HandlerBillingUsageFiltersToTenant|LogBufferRing|LogBufferSinceZero|PersistTokenTightensModeAndRejectsSymlink|LoadTokenErrorsWhenEmpty)$'`.
- Result: passed for `internal/config`, `internal/dashrpc`, `internal/httpapi`,
  `internal/observability`, and `internal/accounting`.
- Verified: `sha256sum dist/*.tar.gz > dist/checksums.txt && test -s
dist/checksums.txt`.
- Result: passed; checksum manifest contains all 13 release archives.
- Follow-up: none.

## Definition Of Done

- Every confirmed P1 and selected P2 task is completed with verification
  evidence, or explicitly deferred by the maintainer with rationale, owner, and
  residual risk.
- DEC-01 and DEC-02 are resolved before dependent tasks are marked complete.
- Runtime streaming attempts are bounded and measured exactly once.
- App and selector state have explicit ownership across failure and reload.
- Advertised builds fail closed, artifacts are validated, and publication is
  gated on tests.
- Platform, dashboard, provider inheritance, and retry contracts agree across
  code, tests, CLI help, README, design, website, and agent guidance.
- The two prior pending P3 follow-ups are completed or deliberately deferred in
  their original task file.
- Final independent verification records the exact commands and results.
