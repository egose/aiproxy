# Provider Inheritance For Repeated Credentials

Created: 2026-08-22 16:07:13 PDT

## Objective

Reduce repeated provider configuration when several credentials use the same
provider type, endpoint, timeout, and model inventory.

Use an `extends` attribute on the existing two-label `provider` block rather
than adding a synthetic provider type such as `provider_duplicate`:

```hcl
provider "openai-compatible" "nvidia-1" {
  display_name = "Nvidia - j.dev"
  base_url     = "https://integrate.api.nvidia.com/v1"

  api_key_ref {
    key = "nvidia-1"
  }

  model "z-ai/glm-5.2" {
    display_name = "GLM 5.2"
    capabilities = ["chat", "responses"]
  }

  model "deepseek-ai/deepseek-v4-flash-0731" {
    display_name = "DeepSeek V4 Flash 0731"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "nvidia-2" {
  extends      = "nvidia-1"
  display_name = "Nvidia - corean"

  api_key_ref {
    key = "nvidia-2"
  }
}

provider "openai-compatible" "nvidia-3" {
  extends      = "nvidia-1"
  display_name = "Nvidia - naver"

  api_key_ref {
    key = "nvidia-3"
  }
}
```

`extends` is preferred over `base` because it states that the new provider
inherits configuration rather than routes through or aliases another
provider. It also preserves the real provider type in the first label; a
`provider_duplicate` type would incorrectly mix a configuration mechanism
with transport types such as `openai-compatible` and `anthropic`.

## Contract

A provider that declares `extends` is a derived provider. Its locally declared
surface is intentionally limited to account identity and credentials:

- The second provider label supplies the new provider `name`.
- `display_name` is optional and overrides the base display name. This is
  included because repeated accounts need distinct labels, as in the motivating
  configuration. When omitted, it inherits the base display name.
- Exactly one of `api_key` or `api_key_ref` must be declared locally. A derived
  provider must never silently reuse the base provider's resolved credential.
- `type`, `base_url`, `upstream_header_timeout`, enabled state, and all `model`
  blocks are inherited unchanged from the base provider.

The type label remains required and must equal the base provider's type:

```hcl
provider "openai-compatible" "nvidia-2" {
  extends = "nvidia-1"
  api_key = env("NVIDIA_2_API_KEY")
}
```

After loading, derived providers are ordinary independent runtime providers.
Aliases, direct routing, health state, metrics, billing, dashboard inventory,
and reload behavior continue to identify them by their own provider names.

## Scope

- HCL provider schema and provider inheritance resolution.
- Validation and field-specific diagnostics.
- Credential resolution for `api_key` and `api_key_ref`.
- Runtime flattening before provider and alias validation.
- Live configuration reload.
- Interactive and non-interactive provider configuration editing.
- User-facing configuration documentation and examples.
- Unit and regression tests.

## Working Rules

- Read `AGENTS.md` before editing.
- Preserve unrelated worktree changes and do not revert concurrent work.
- Set the task to `in_progress` before implementation and add completion
  evidence only after verification passes.
- Keep inheritance a configuration-loading concern. Do not add inheritance
  branches to request dispatch or provider adapters.
- Do not expose resolved credentials in diagnostics, rendered config, logs,
  dashboard data, or tests.
- Follow the repository convention of avoiding source comments unless the
  surrounding code requires one.

## Non-Goals

- Adding `provider_duplicate`, `duplicate`, `template`, or another runtime
  provider type.
- Allowing arbitrary provider fields or individual models to be overridden.
- Merging model lists between base and derived providers.
- Supporting multi-level inheritance. A provider named by `extends` must be a
  concrete provider that does not itself declare `extends`.
- Generating aliases or expanding one alias target into every derived provider.
- Changing alias retry, provider health, billing, or routing semantics.
- Adding inheritance to listener, auth, alias, or other HCL blocks.

## Baseline

- `internal/config/schema.go:68-78` models every provider as one
  `rawProvider`; there is no inheritance reference.
- `internal/config/build.go:69-86` builds providers in declaration order and
  immediately resolves each provider credential.
- `internal/config/build.go:198-248` applies root defaults, resolves
  credentials, and builds each model inventory directly from one block.
- `internal/config/validate.go:141-188` validates already-built providers and
  requires each enabled provider to have models and a resolved credential.
- `internal/config/types.go:123-135` contains only flattened runtime provider
  data, which should remain sufficient after this feature.
- `internal/configedit/configedit.go:223-291` always renders complete provider
  blocks.
- `cmd/aiproxy/configure.go:230-262` and
  `cmd/aiproxy/configure.go:1464-1524` expose and validate complete provider
  input in interactive and non-interactive flows.
- Configuration documentation currently describes only complete two-label
  provider blocks in `docs/design.md`, `README.md`, and
  `website/docs/configuration.md`.
- Baseline tests were not run while creating this task.

## Task PROVIDER-INHERIT-01: Add Restricted Provider Inheritance

Status: completed

Priority: P2

Suggested agent: Go HCL configuration and CLI engineer

Dependencies: none

Primary ownership:

- `internal/config/schema.go`
- `internal/config/build.go`
- `internal/config/validate.go`
- `internal/config/load_test.go`
- `internal/config/types.go` only if resolution metadata cannot remain raw-only
- `internal/configedit/configedit.go`
- `internal/configedit/configedit_test.go`
- `cmd/aiproxy/configure.go`
- `cmd/aiproxy/configure_test.go`
- `README.md`
- `docs/design.md`
- `website/docs/configuration.md`
- `website/docs/config-examples.md`
- `AGENTS.md`

Finding:

Operators with multiple credentials for one upstream must repeat the same
provider type, base URL, timeout, and model blocks for every account. This is
verbose and allows model inventories or capabilities that should be identical
to drift between providers.

Implementation requirements:

1. Add optional provider attribute `extends`, containing the name of another
   provider in the same file.
2. Resolve base references independently of declaration order so a derived
   provider may appear before or after its base.
3. Require the referenced base to exist, have a different name, and not itself
   declare `extends`. Require the base to be enabled because derived providers
   cannot override enabled state. Reject self-reference, disabled bases, and
   inheritance chains with provider-specific errors.
4. Require the derived block's provider type label to equal the base type.
   Keep the type label mandatory so every provider block retains the existing
   two-label shape and remains readable without resolving another block.
5. Permit only `extends`, optional `display_name`, and exactly one locally
   declared credential form (`api_key` or `api_key_ref`) in a derived block.
   Reject local `base_url`, `upstream_header_timeout`, `enabled`, or `model`
   declarations rather than silently ignoring them. Detect declarations
   syntactically, including fields explicitly assigned empty or zero values;
   do not infer presence only from decoded Go zero values.
6. Require a local credential even if the base has one. Preserve the existing
   mutual-exclusion, `api_key_ref.key`, default key-file path, resolution, and
   secret-safety rules for that credential.
7. Flatten each derived provider before normal runtime validation by copying
   the base type, base URL, effective upstream header timeout, enabled state,
   and complete model inventory, then replacing name, optional display name,
   and credential with locally declared values.
8. Ensure copied model slices, capability slices, lookup maps, and credential
   metadata do not share mutable state that could couple base and derived
   providers during reload or runtime use.
9. Preserve base-provider behavior: the base remains a routable provider using
   its own credential and may be referenced by aliases like any other provider.
10. Preserve existing duplicate-provider-name checks across concrete and
    derived blocks.
11. Resolve aliases only after all providers have been flattened. Alias targets
    may reference derived providers exactly as they reference concrete ones.
12. Apply inheritance changes atomically on successful `SIGHUP` reload. An
    invalid reference, forbidden override, or unresolved derived credential
    must reject the candidate config and leave the active runtime unchanged.
13. Extend `aiproxy configure provider` to read, create, update, and render
    derived providers without expanding inherited fields into the HCL. Add an
    `--extends <provider-name>` non-interactive option and an equivalent
    interactive base-provider choice.
14. In configure flows, reject inherited-field flags when `--extends` is used,
    require a local credential, verify the selected base when an existing config
    is available, and preserve unrelated blocks and secrets-file behavior.
15. Document syntax, allowed local fields, direct-only inheritance, declaration
    order independence, local credential requirement, reload behavior, and the
    fact that aliases still enumerate each provider target explicitly.

Acceptance criteria:

- The motivating Nvidia configuration can define its endpoint and models once,
  then define each additional account with `extends`, `display_name`, and an
  `api_key_ref` of its own.
- A derived provider has the base provider's type, base URL, effective timeout,
  models, upstream model names, and capabilities, but its own name, display
  name, and resolved credential.
- Both `api_key` and `api_key_ref` work as local credential forms, and declaring
  neither or both fails without revealing secret values.
- A missing or disabled base, self-reference, inheritance chain, type mismatch,
  duplicate name, or forbidden local field fails with an error naming the
  derived provider and offending field or reference.
- Derived providers may be declared before their bases.
- Direct requests and alias targets route independently to the derived provider
  name, with independent health and accounting identity.
- Editing or rendering a derived provider keeps the compact `extends` form and
  does not materialize inherited models, endpoint, or timeout into the file.
- A successful reload can add, change, or remove derived providers; an invalid
  reload preserves the previous runtime.
- Existing complete provider configurations load unchanged.
- Focused tests cover raw resolution, validation failures, deep-copy isolation,
  alias references, configure round trips, and reload behavior.
- `go test ./internal/config ./internal/configedit ./cmd/aiproxy ./internal/app`
  passes.
- `make vet test`, `make test-race`, and `make build` pass before completion.

## Definition Of Done

- `PROVIDER-INHERIT-01` is marked `completed` with changed-file and verification
  evidence.
- Runtime provider consumers require no inheritance-specific behavior because
  all providers are fully resolved during configuration loading.
- Documentation and configuration tooling consistently use `extends`; no
  synthetic duplicate provider type is introduced.
- The compact Nvidia example is documented and validated without changing its
  direct or alias routing behavior.

## Completion Evidence

Changed files:

- `internal/config/schema.go`: added raw `extends` and provider syntax metadata.
- `internal/config/load.go`: records declared provider attributes and nested blocks for zero-value override detection.
- `internal/config/build.go`: resolves declaration-order-independent provider inheritance, validates derived-provider restrictions, deep-copies inherited runtime data, and flattens providers before aliases.
- `internal/config/load_test.go`: covers successful inheritance, local `api_key_ref`, alias target resolution, declaration-order independence, deep-copy isolation, and invalid inheritance cases.
- `internal/configedit/configedit.go`: renders derived providers in compact `extends` form and exposes inherited model names for editor model discovery.
- `internal/configedit/configedit_test.go`: covers compact rendering and derived model discovery.
- `cmd/aiproxy/configure.go`: adds `--extends`, parses existing derived providers, rejects inherited-field flags for derived providers, verifies base providers from existing config, and keeps derived providers compact.
- `cmd/aiproxy/configure_test.go`: covers non-interactive derived-provider creation and inherited-field flag rejection.
- `README.md`, `docs/design.md`, `website/docs/configuration.md`, `website/docs/config-examples.md`, `website/docs/operations.md`, `AGENTS.md`: document provider inheritance syntax, restrictions, local credential requirement, reload/flattening behavior, explicit alias targets, and configure usage.

Verification attempted:

- `go test ./internal/config ./internal/configedit ./cmd/aiproxy`: failed because `go` is not on `PATH`.
- `make vet test`: failed at `go vet ./...` because `go` is not on `PATH`.
- `make test-race`: failed because `go` is not on `PATH`.
- `make build`: failed because `go` is not on `PATH`.

Runtime provider consumers require no inheritance-specific branches because derived providers are flattened during configuration loading before normal validation and alias construction.

Follow-up verification on 2026-08-23:

- Added `TestLoadDerivedProviderAcceptsInlineLocalCredential` to explicitly prove
  inline `api_key` succeeds for derived providers alongside the existing
  `api_key_ref` success coverage.
- Added `TestEndToEndDerivedProviderDirectAndAliasRouting` to prove direct and
  alias HTTP requests route to a derived provider using its local credential and
  inherited endpoint/model mapping.
- Added `TestDerivedProviderIdentityAcrossRuntimeSurfaces` to prove derived
  provider identity is independent across routing, provider health, Prometheus
  metrics, accounting summaries, and dashboard snapshots.
- Added `TestReloadAddsChangesRemovesDerivedProviderAndRollsBackInvalidCandidate`
  to prove successful derived-provider add/change/remove reloads and that an
  invalid inherited-provider candidate rolls back without breaking the previous
  derived route.
- Added `TestConfigureProviderInteractiveChoosesDerivedBaseProvider` and updated
  interactive configure behavior so `extends` is selected from `none` plus
  eligible existing concrete, enabled base providers of the selected type.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 go test ./internal/config
./internal/configedit ./cmd/aiproxy ./internal/app ./internal/e2e` passed.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make vet test` passed.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make test-race` passed.
- Verified: `ASDF_GOLANG_VERSION=1.26.6 make build` passed.
