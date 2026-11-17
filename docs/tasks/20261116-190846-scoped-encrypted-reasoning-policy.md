# Scoped Encrypted Reasoning Policy

Created: 2026-11-16 19:08:46 (local time)

Status: completed; ER-01–ER-07 completed

## Objective and Scope

Extend the existing alias-only `encrypted_reasoning` policy to root, provider,
and model configuration, including recovery for direct `<provider>/<model>`
requests. Retain explicit alias overrides and bounded same-target recovery.

This is an improvement, not a claim that the current alias-only behavior is a
defect. This document records reviewed requirements and implementation tasks;
the contract must be finalized in ER-01 before downstream implementation.

In scope:

- Static HCL/JSON parsing, validation, defaults, policy resolution, inheritance,
  catalog copies, and live reload.
- Direct and alias dispatch for chat completions and Responses, both ordinary
  JSON and immediate error responses to streaming requests.
- Conversion and configuration-edit preservation; CLI and public documentation.
- Managed provider/model API, persistence, and web forms, subject to D3 below.
- Regression tests for compatibility, attempt bounds, streams, compressed errors,
  authorization, accounting, and peer-deferred execution boundaries.

Non-goals:

- New inference capabilities, native `/v1/messages` policy support, new error
  statuses, credential rotation, or choosing another target as mismatch recovery.
- Retrying established streams, changing normal alias retry/cooldown rules, or
  interpreting peer-control failures as upstream inference failures.
- Redesigning reasoning payload shapes, compressed inspection limits, guardrails,
  logging redaction, or the peer protocol.
- Implementing root-policy editing through a new global admin API.

## Analysis Coverage and Baseline

Inspected configuration schemas/types, alias normalization and validation,
static inheritance restrictions, catalog cloning, direct/alias dispatch,
reasoning helpers and tests, runtime reload activation, database provider
materialization and models, admin/provider and UI types, conversion, CLI/editor
entry points, package scripts, and relevant existing task findings.

Confirmed current behavior:

1. Only `rawAlias` / `config.Alias` declare the policy:
   `internal/config/schema.go` (`rawAlias`, `rawEncryptedReasoning`) and
   `internal/config/types.go` (`Alias`, `EncryptedReasoning`).
2. Defaults are `passthrough = true`, `on_caller_mismatch = "fail"`, and built-in
   match messages. Normalization is currently duplicated in
   `internal/config/build.go`, `internal/config/dynamic.go`, and
   `internal/httpapi/encrypted_reasoning.go`.
3. `internal/httpapi/dispatch.go` (`dispatchDirect`) performs no reasoning
   transformation or recovery. `TestDirectRequestNeverStripped` currently
   verifies that an alias's policy does not affect direct requests.
4. `dispatchAlias` resolves one alias policy before target selection. Its
   `strippedAttempted` flag is request-wide; recovery retries the same target,
   updates the shared `body`, and carries that stripped body to later failover
   targets. This is not currently a per-target retry allowance.
5. `isEncodedCallerMismatch` gates inspection on a non-streaming upstream `400`,
   request opaque markers, and matching decoded error text.
   `stripDispatchOpaqueBlocks` / `isDispatchCallerMismatch` also preserve
   deferred execution budgets and exclude peer-control outcomes.
6. Static derivatives inherit model definitions but currently allow only
   existing-name `deferred_transport` model overlays:
   `internal/config/build.go` (`validateDerivedModelOverlays`).
   Managed derivatives use a separate materialization path:
   `internal/dbmerge/dbmerge.go` (`BuildProvider`).
7. Stored aliases have a policy JSON column; stored providers/models do not:
   `internal/store/models.go`. Admin/UI provider and model types likewise omit it.
8. `internal/config/convert.go` (`jsonBlockLabels`, `writeHCLBody`) lacks an
   `encrypted_reasoning` block entry. JSON-to-HCL conversion therefore needs
   explicit coverage for existing aliases as well as the new scopes.

Related work, not duplicate objectives:

- [Upstream and CLI health review](20260926-163550-upstream-and-cli-health-review.md),
  SAFE-03 (completed): bounded compressed inspection. Preserve its implementation
  and regression coverage; do not reopen decoder redesign.
- [Admission and catalog workflows](20260926-191434-admission-and-catalog-workflows.md),
  FLOW-04: alias form round-trip evidence, including `passthrough = false`.
  Preserve those assertions when adding inheritance/reset behavior.

Checks actually run before creating this file, from the repository root on a
clean worktree:

```sh
go test ./internal/config ./internal/httpapi -run 'EncryptedReasoning|Strip|CallerMismatch|BoundedCompressedInspection' -count=1
```

Result: passed in both packages. This is filtered existing-test coverage, not
verification of the proposed feature.

Planning-document verification: `make docs-contract` passed; task-field,
dependency-reference, local-link, and private-metadata checks passed. The new-file
diff check emitted no whitespace diagnostics.

Limitations: no live-provider calls, PostgreSQL checks, web builds, binary
integration tests, full test suite, or race suite were run during planning.
Admin mutation handlers, peer execution, and CLI interactive forms were not
exhaustively audited; implementing tasks must inspect their complete call paths.

## Approved Shared Contract

The user approved the D1–D3 recommendations and sequential subagent implementation
on 2026-11-16. ER-01 records concrete examples and the managed overlay representation.

1. Reuse an unlabeled optional `encrypted_reasoning { ... }` block at root,
   provider, model, and alias scopes; retain existing field names and values.
2. Direct precedence: model → provider → root → built-in defaults.
   Alias precedence: explicit alias → selected target's model → selected target's
   provider → root → built-in defaults. Resolve target policies after selection,
   not from the first member of the pool.
3. Replace blocks wholesale, not field-by-field. Missing block means inherit;
   an explicit empty block resets to safe defaults and stops inheritance.
   Omitted fields use built-in defaults, including `passthrough = true`.
   A non-empty `match_messages` replaces built-ins; omitted/empty lists retain
   current built-in fallback behavior.
4. Preserve declaration presence separately from effective values. In particular,
   absent policy, `{}`, and `{ passthrough = false }` must remain distinguishable
   across parsing, storage, editing, and round trips. Never persist inherited
   effective values as if they were local declarations.
5. No declared policy anywhere preserves existing direct/alias behavior. An
   explicit alias block remains authoritative, including an empty safe-reset
   block overriding a root policy.
6. `passthrough = false` strips the currently supported reasoning shapes before
   the first inference attempt covered by that policy. It does not create an
   extra mismatch retry when the request is already stripped.
7. With passthrough enabled and `strip_and_retry`, recovery requires a recognized
   non-streaming upstream `400`, the existing opaque-marker gate, successful
   bounded decoding, and an actual changed rewrite. Retry the same provider,
   credential, endpoint, upstream model, and protocol under the same context.
8. Direct dispatch makes at most two inference attempts and never selects another
   target or consults/populates alias cooldown state. Normal alias failover remains
   separate and follows its configured statuses and transport-error rules.
9. Established streams, unrelated errors, non-`400` errors, unsupported/corrupt/
   over-budget compressed bodies, unmodified requests, and peer-control outcomes
   do not trigger reasoning recovery. Preserve original response bytes/headers
   when no recovery is performed.
10. Policy updates use the existing transactional reload path: new admissions use
    the new snapshot; admitted requests/streams retain their policy snapshot;
    rejected candidates leave the old runtime active.
11. Apply the same validation at every accepted scope, including disabled static
    providers: two accepted mismatch policies, at most 16 patterns, trimmed
    4–256-length entries under the existing validator's measurement, and
    case-insensitive duplicate rejection. Error messages identify the owning scope.
12. Reuse the existing stripping helpers. Ordinary message text and tool history
    must retain existing preservation behavior. Do not promise that _every_
    plaintext reasoning field is preserved: current helpers also remove typed
    reasoning/thinking items and chat reasoning fields without requiring a
    signature on each item. Pin this boundary in tests/docs rather than silently
    redesigning it in this feature.

### Confirmed Decisions

Owner: repository maintainer/user; all recommendations below were explicitly
approved. ER-01 finalizes their concrete implementation contract.

| ID  | Question                                                                               | Approved recommendation                                                                                                                                                                                                                                                                                                                                                                    |
| --- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| D1  | What happens to retry allowance and stripped bodies across mixed-policy alias targets? | Preserve the current single extra recovery attempt per alias request. Preserve sticky stripped-body behavior for explicitly configured aliases. Without an explicit alias block, evaluate each selected target against the original admitted body so one target's stripping cannot defeat a later target's passthrough override; share the one-recovery budget. Document this distinction. |
| D2  | Do derived providers inherit reasoning policies and permit local overrides?            | Inherit declared base provider/model policies by value, permit a local provider override, and permit policy-only overlays of existing inherited model names. Explicit empty blocks reset inherited policies. Preserve credential isolation, no static inheritance chains, and no new/redefined inherited models. Decide the managed-derivative overlay representation explicitly.          |
| D3  | Is the first release static-only or does it include managed configuration parity?      | Include nullable provider/model policy storage, admin read/write support, and web forms under existing RBAC. Root remains static configuration. If parity is deferred, explicitly reject unsupported local writes, preserve static policy visibility, and ensure managed targets still inherit root defaults. Revise ER-04/ER-06 and final dependencies before proceeding.                 |

Do not silently switch existing aliases to one extra retry per pool target or
restore their original body after stripping. Such a choice is a contract change
requiring revised requirements, regression expectations, and release notes.

### ER-01 Finalized Semantics (2026-11-16)

Owner: ER-01 contract subagent. D1–D3 approval above is the authorization for
these decisions. Managed parity and overlays are **not deferred**; D3's
conditional static-only alternative remains planning history, not an implementation
option. Downstream ordering/dependencies remain unchanged.

#### Policy Layers, Resets, and Fixtures

At each provider/model layer, distinguish the local declaration from a copied
base declaration. For a derivative, the model layer is its local model overlay,
otherwise the base's **declared model block**; the provider layer is its local
provider block, otherwise the base's **declared provider block**. Neither copied
block contains fallback values resolved from another layer.

Resolve the first present whole block:

```text
explicit alias block, when dispatching an alias
selected model layer: local declaration/overlay, else copied base model declaration
selected provider layer: local declaration, else copied base provider declaration
root declaration from the admitted runtime snapshot
built-in defaults
```

Default the chosen block's omitted fields from built-ins, never from a lower
block. Built-ins are `passthrough = true`, `on_caller_mismatch = "fail"`, and
`["not issued to this caller", "invalid_encrypted_content", "could not be verified",
"could not be decrypted", "item_id did not match", "invalid signature"]`.
Keep existing scalar/default normalization, including empty mismatch strings
defaulting to `fail`. Omitted/empty pattern lists select built-ins; a nonempty
list replaces them, and a list containing an empty/short/duplicate entry is invalid.
The existing 4–256 entry-length check measures Go string **bytes after
normalization/trim**, not Unicode rune count.

`nil` means no declaration at that layer; `{}` is a present, safe-default block.
Removing a child model overlay restores the base's model declaration, if any;
it does **not** skip the model layer to expose the child provider block. A child
provider reset cannot reset an inherited model block. Preserve local block/member
presence for serialization, even if serving also stores a normalized copy.

Fixture shorthand: `D` = `{}` (safe reset), `R` =
`{ on_caller_mismatch = "strip_and_retry" }`, `S` = `{ passthrough = false }`,
and `—` = no declaration. These are blocks, not field-wise patches.

| Case                    | Root                   | Provider | Selected model | Explicit alias | Winner and observable behavior                                                             |
| ----------------------- | ---------------------- | -------- | -------------- | -------------- | ------------------------------------------------------------------------------------------ |
| No declarations         | —                      | —        | —              | —              | Built-ins; intact body, no reasoning recovery. Normal alias failover still applies.        |
| Root fallback           | S                      | —        | —              | —              | Root; strip before the first direct/selected-target inference attempt.                     |
| Provider reset          | S                      | D        | —              | —              | Provider; intact body, mismatch fails without reasoning recovery.                          |
| Model reset             | R                      | S        | D              | —              | Model; intact body, mismatch fails without reasoning recovery.                             |
| Whole-block replacement | S with custom patterns | R        | —              | —              | Provider; omitted passthrough becomes true and patterns become built-ins, not root values. |
| Alias override          | R                      | S        | S              | D              | Alias; every selected target receives intact history; target/root policies are ignored.    |
| Alias strip override    | D                      | D        | D              | S              | Alias; every attempted target receives stripped history.                                   |

Unconditional stripping is independent of the mismatch detector's opaque-marker
gate: unsigned typed thinking/reasoning items and chat `reasoning` /
`reasoning_content` fields are stripped too. Conversely, an unsigned typed item
without any of the detector's opaque markers cannot trigger conditional recovery.
Use an unsigned chat part `{"type":"thinking","thinking":"plain"}` and an unsigned
Responses item `{"type":"reasoning","summary":[]}` as fixtures: S removes them,
whereas R sends them intact and cannot recover a matching 400 without another
opaque marker somewhere in the attempted body. Retain the existing
case-insensitive raw-body marker scan; do not redesign it as a shape-aware proof
that a particular item is caller-bound.
Preserve the helper's precise boundary:

- Chat removes those two message fields and content parts selected by
  `shouldDropChatPart`, including the existing typed parts, marker-bearing parts,
  and compact non-text `data` parts. Ordinary string/text content without such
  markers and separate `tool_calls`/tool history remain intact.
- Responses removes typed reasoning/thinking/redacted input items; on other input
  items it removes top-level `encrypted_content`/`signature` only. Do not add a
  recursive stripping promise or remove unrelated request-level reasoning options.
- Supported operations are chat completions and Responses only. `stream: true`
  does not prevent pre-dispatch stripping or recovery from an immediate ordinary
  `400`; an established `result.Streaming` response is never recovered/replayed.

For exact-body fixtures, let `B` be the immutable **admitted** body after existing
admission/guardrail processing but before reasoning rewriting, and `strip(B)` be
the existing operation-specific helper's changed output. For chat, this pair is:

```json
{
  "model": "alias/a",
  "messages": [
    {
      "role": "assistant",
      "content": [
        { "type": "reasoning", "encrypted_content": "blob" },
        { "type": "text", "text": "hi" }
      ],
      "tool_calls": [{ "id": "call-1", "type": "function", "function": { "name": "f", "arguments": "{}" } }]
    },
    { "role": "tool", "tool_call_id": "call-1", "content": "done" }
  ]
}
```

```json
{
  "model": "alias/a",
  "messages": [
    {
      "role": "assistant",
      "content": [{ "type": "text", "text": "hi" }],
      "tool_calls": [{ "id": "call-1", "type": "function", "function": { "name": "f", "arguments": "{}" } }]
    },
    { "role": "tool", "tool_call_id": "call-1", "content": "done" }
  ]
}
```

For Responses, use `{"model":"alias/a","input":[{"type":"reasoning",
"encrypted_content":"blob","summary":[]},{"type":"message","role":"user",
"content":"hi"}]}`; stripped `input` contains only the unchanged message item.
Compare rewritten JSON structurally (serialization order is not a contract).
Intact dispatch bodies remain byte-preserved; the adapter's existing model rewrite
and protocol translation still apply. Each attempt's cloned inbound body and
content length must agree with its dispatch body.
For direct fixtures, substitute `"model":"p1/m"` in both bodies.

#### Request-Wide Attempt State and Normal Failover

An inference attempt here is one dispatch `Adapter.Do` invocation, not a peer
poll/control/replay call. Direct dispatch has one initial attempt and at most
one extra recovery attempt (maximum **two**). Recovery never reselects a target,
re-resolves a credential, obtains a new deadline, or changes provider identity,
endpoint, upstream model, protocol, or admitted snapshot.

An alias with `N` targets has at most `N` normal target inference attempts plus
**one** extra reasoning recovery (maximum **N + 1**, not `2N`). Skipped
unhealthy/cooling/unresolvable targets cause no inference; normal selection/tried
state is unchanged. Spend the shared recovery allowance when issuing the extra
same-target attempt after a successful changed rewrite. Unconditional pre-stripping,
an unrecognized mismatch, or an unchanged rewrite does not consume that allowance.

- **Explicit alias block, including `{}`:** apply that block for the entire
  dispatch. Pre-stripping or a successful recovery rewrite updates the alias's
  working body; that stripped body is sticky for later failover targets.
- **No explicit alias block:** resolve policy independently after selecting each
  target. Start that target from `B`, not a prior target's working body. Its own
  pre-strip/recovery rewrite affects only its attempts. Share the one extra
  recovery allowance across all targets; do not restore the allowance when
  restoring `B` for the next target.

Recovery is considered **before** ordinary configured-status failover, including
when `400` is in `retry_status_codes`. A second mismatch cannot earn another
recovery; treat that result using ordinary alias rules. Recognition requires an
ordinary non-streaming `400`, opaque markers in the actual attempted body, a
bounded successful decode and matching error text, and a changed strip. An
unconditional policy never performs conditional recovery, even if it explicitly
also sets `on_caller_mismatch = "strip_and_retry"`.

For the sequences below, force normal selection order `p1/m`, then `p2/m`, with
distinct fixture credentials/endpoints/upstream names; `503` is retryable and
`400` is not unless stated. `M` is ordinary `400` with body
`{"error":{"message":"reasoning encrypted_content was not issued to this caller"}}`.

| Sequence                                             | Attempt 1              | Attempt 2                        | Attempt 3                                            | Outcome/bound                                                                                 |
| ---------------------------------------------------- | ---------------------- | -------------------------------- | ---------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Explicit alias R                                     | `p1/m`, B → M          | same `p1/m`, strip(B) → 503      | `p2/m`, strip(B) → 200                               | Success; exactly three calls, one recovery, sticky body.                                      |
| No alias block; p1 R, p2 R                           | `p1/m`, B → M          | same `p1/m`, strip(B) → 503      | `p2/m`, B → 200                                      | Success; exactly three calls, one recovery, target-local body.                                |
| No alias block; p1 R, p2 S                           | `p1/m`, B → M          | same `p1/m`, strip(B) → 503      | `p2/m`, strip(B) → 200                               | Exactly three calls; an exhausted recovery allowance does not suppress p2's pre-strip policy. |
| First two sequences; p2 returns M                    | Same as above          | Same as above                    | `p2/m`, strip(B) if explicit alias / B if absent → M | Return p2's 400; exactly three calls. Even p2 R has no recovery allowance left.               |
| Explicit alias S                                     | `p1/m`, strip(B) → 503 | `p2/m`, strip(B) → 200           | None                                                 | Exactly two calls; no reasoning recovery.                                                     |
| No alias block; p1 S, p2 D                           | `p1/m`, strip(B) → 503 | `p2/m`, B → M                    | None                                                 | Return 400; exactly two calls; p1 stripping did not defeat p2 reset.                          |
| No alias block; p1 S, p2 R                           | `p1/m`, strip(B) → 503 | `p2/m`, B → M                    | same `p2/m`, strip(B) → 200                          | Success; exactly three calls; p1 pre-strip did not spend the allowance.                       |
| No alias block; p1 D, p2 R; 400 configured retryable | `p1/m`, B → M          | `p2/m`, B → M                    | same `p2/m`, strip(B) → 200                          | Success; ordinary failover first, then one recovery.                                          |
| Direct p1 R                                          | `p1/m`, B → M          | same `p1/m`, strip(B) → 503 or M | None                                                 | Return the second result; never call p2, even if an alias contains it.                        |

If `400` is configured retryable, p2's M in the exhausted-allowance row may
continue normal failover to p3, but p3 still has no reasoning recovery allowance; maximum four
calls for three targets. After any retryable status/transport failure, normal
alias failover proceeds with the body rule above. Unrelated `400`s and
`401`/`403`/`5xx` never trigger reasoning recovery, but may still trigger
**ordinary alias failover** when configured (or through existing transport rules).
An unchanged strip earns no extra call, including a marker inside otherwise
ordinary text with no helper-selected removable shape.

Observe health, retry advice/cooldown, and per-attempt telemetry exactly as
currently required. An admitted same-target recovery does not reacquire/select
around newly recorded health/cooldown state; later target selection and future
requests still honor it. All-cooling synthetic `429` behavior is unchanged.
Direct recovery neither reads nor writes alias cooldowns. Peer-control outcomes
never trigger recovery or normal failover; only definite terminal inference
results may use these rules under existing execution budgets. No new inference
is authorized by polling/replay/finalization. Cancellation, exhausted execution
budgets, established streams, and decoder failures cannot authorize a fresh
reasoning attempt. If a result is returned without recovery, inspection must not
change its original response bytes/headers; ordinary failover may discard it
under the existing close/lease rules.

#### Derived Providers and Managed Overlay Representation

Copy declared provider/model policies and their pattern slices by value on every
candidate build and catalog copy. Base, child, sibling derivatives, `Models`,
`ModelByName`, and admitted snapshots must not share mutable policy declarations.
Local child declarations/overlays replace the copied block wholesale.

| Base provider block | Base model m block | Child provider block | Child model m overlay | Effective m policy                                                              |
| ------------------- | ------------------ | -------------------- | --------------------- | ------------------------------------------------------------------------------- |
| R                   | S                  | D                    | —                     | S, inherited model; child provider reset cannot override it.                    |
| R                   | S                  | S                    | D                     | D, local model reset; even child provider S loses.                              |
| R                   | S                  | D                    | Overlay removed       | S again, not child provider D.                                                  |
| R                   | —                  | S                    | —                     | S, local child provider.                                                        |
| R                   | —                  | —                    | —                     | R, copied base provider declaration.                                            |
| S                   | D                  | R                    | —                     | D, inherited model reset.                                                       |
| —                   | —                  | —                    | —                     | Current root, otherwise built-ins; do not copy root fallback into either layer. |

Static derivatives retain their existing restricted provider/credential/peer
surface. The only new allowances are a provider `encrypted_reasoning` block and
existing-name model reasoning overlays. A static model overlay may contain
`encrypted_reasoning`, the already supported `deferred_transport`, or both in
one model block, at most one of each and at least one total. Bare empty model
overlays, model attributes (`display_name`, `upstream_name`, `protocol`,
`capabilities`), pricing, unrelated blocks, duplicate names/blocks, and new
child model names remain rejected, including explicit empty/null redefinitions.
Existing peer eligibility/conflict/credential rules still apply; reasoning does
not cause base deferred opt-ins to become inherited.

Managed derivatives use the **existing nested `models` array**, not a new overlay
map, model endpoint, or flattened copy of the base inventory:

```json
{
  "extends": "base",
  "encrypted_reasoning": { "passthrough": false },
  "models": [{ "name": "m", "encrypted_reasoning": {} }]
}
```

This is a provider PUT fragment. Each derivative model entry accepts only
`name` and `encrypted_reasoning`, with an existing inherited name. A non-null
policy object is the sole new managed model overlay, including an empty reset.
No `deferred_transport`, model redefinition, new model, or extra overlay key is
accepted; detect **declared key presence**, not just nonzero decoded values.
For example, `upstream_name: ""`, `capabilities: []`, or `pricing: null` still
rejects the write. Reject duplicate overlay names and invalid policy objects
before commit; duplicate/case-variant JSON keys cannot hide forbidden declarations.
Policy-free name entries (omitted/null policy) mean no local overlay and are
canonicalized out after validating their existing name.

Storage/materialization contract:

1. Add nullable `encrypted_reasoning` JSONB to `DBProvider` and
   `DBProviderModel`. Store **local declarations only**: SQL NULL for absence,
   `{}` for reset, and sparse member objects preserving explicit false values.
   Treat a stored JSON `null` as absence; new writes canonicalize it to SQL NULL.
   Do not decode `{}` through a zero-value boolean that becomes false.
2. On concrete providers, model rows remain full model definitions plus their
   nullable local policy. On derivatives, non-null-policy rows are sparse
   existing-name overlays: name and policy only; other stored model fields must
   have their empty/default values. Parent `Extends` determines the row role;
   no additional public representation or overlay-kind column is needed.
3. `BuildProvider`, managed write validation, alias-target validation, and admin
   model/policy views must use consistent model materialization. Inherit the
   complete base inventory, apply only local overlays, and keep slice/index
   copies consistent. Never mistake a sparse overlay list for the served model
   inventory or persist inherited models/policies as local rows.
4. Additive migration must not delete/rewrite existing models. Current
   `BuildProvider` ignores _all_ derivative model rows; pre-feature rows whose
   new policy column is NULL remain **legacy inert rows** on derivatives, not
   new definitions/overlays. Preserve that loader compatibility and omit those
   inert definitions from the materialized view. New derivative model-array writes
   cannot create such full-definition rows. Any non-null malformed policy or invalid
   active overlay rejects the candidate; this exception is not a corruption
   fallback for new policies.
5. Preserve local provider policy across `extends` changes. When switching
   concrete ↔ derived with any stored model policy/overlay, require an explicit
   `models` array in the same update to choose destination definitions/overlays;
   never silently reinterpret or discard policy-bearing rows. Policy-free
   legacy transitions retain existing behavior. Changing one base name to
   another validates preserved overlays against the new base. A subsequent
   base-model removal that leaves an overlay dangling rejects reload with the
   old runtime intact; do not silently remove the overlay.

Bases remain enabled, concrete, same-type providers, with no self-reference or
inheritance chains (including managed-to-managed and managed-to-static-derived
paths). Credentials remain local and never inherited. Preserve current static
enabled-state restrictions and managed enabled-state behavior. `config.Provider`
currently lacks `Extends` provenance and the managed builders lack an explicit
chain guard: ER-02/ER-04 must retain enough relationship metadata to enforce
this approved single-level contract after flattening. This does not authorize
unrelated transport/provider overrides or managed inheritance from custom TLS /
deferred-opted bases.

#### Managed Update Intent and Read-Only Metadata

The block JSON below is the **administrative** representation; static HCL/JSON
removal remains omission of the block/key, not an HCL `encrypted_reasoning = null`
attribute. Nullable administrative policy values are intentional, not an
effective-policy value to persist.

| Input                         | Create               | Provider PUT / alias PUT                                               | Model entry in a supplied replacement `models` array                             |
| ----------------------------- | -------------------- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Policy omitted                | No local declaration | Preserve existing local block exactly                                  | No local model declaration; do not preserve the replaced row's old block.        |
| `"encrypted_reasoning": null` | No local declaration | Remove local block; resume lower-layer resolution                      | No local model declaration/overlay; restores base model policy for a derivative. |
| `"encrypted_reasoning": {}`   | Explicit safe reset  | Replace entire local block with reset                                  | Local model reset, not inherited defaults.                                       |
| Nonempty policy object        | Local override       | Replace entire block; omitted members use built-ins, not saved members | Local model override; same whole-block defaults.                                 |

The existing collection contract stays intact: on provider PUT, omitted **or
null `models`** preserves all rows; a supplied array replaces the complete local
definition/overlay set transactionally. `models: []` on a derivative clears
local overlays, not inherited models. On a concrete provider it remains subject
to the at-least-one-model validator. Within a replacement array, omitting/nulling
one policy is inheritance/removal, not a patch of that model. Clients must retain
other intended local overrides when submitting the complete array.
Omission/null preservation does not satisfy the explicit-array requirement for
a policy-bearing concrete/derived transition; null is not an array.

Example with base model m = R: derivative PUT `{"models":[{"name":"m",
"encrypted_reasoning":{}}]}` resets m to D; `{"models":null}` preserves that
reset; `{"models":[]}` or a replacement entry with policy null removes the
overlay and restores inherited R. Provider PUT `{"encrypted_reasoning":null}`
removes only the provider block and cannot bypass the model declaration.

Use the same policy-value meanings for aliases when sharing DTOs/codecs.
`updateDBAlias` currently conflates omitted and null blocks; differentiating an
explicit null removal is an intentional API clarification to document in ER-05.
An unchanged/omitted alias block stays explicit and sticky; null removes that
override and selects D1's target-policy/original-body mode. Do not change the
behavior of existing normalized non-null alias overrides or persisted full policy
objects; empty objects use the approved safe reset, not a zero-value false boolean.
Credential-only PUT preserves provider/model policy declarations and validates
the rebuilt provider; it is not a policy-edit endpoint and must reject supplied
reasoning-policy/model-policy edits rather than silently accepting them.

Pin these provider/model view fields (and matching web types):

| Field                              | Meaning                                                                                                                                                                                                                   |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `encrypted_reasoning`              | Nullable **owner-local declaration**, with optional members; return `{}` for a local reset and null for no local block, even when a base/root block is effective. Never default-fill it from effective metadata.          |
| `encrypted_reasoning_effective`    | Fully normalized resolved block; provider view resolves provider → root → built-ins, model view resolves the full direct model precedence. A provider value does not claim to override every model.                       |
| `encrypted_reasoning_source`       | Winning scope: `model`, `provider`, `root`, or `builtin` (provider views cannot report `model`).                                                                                                                          |
| `encrypted_reasoning_inherited`    | True only when the **winning declaration** is a by-value copy of a base provider/model block through `extends`; false for local winners, root fallback, and built-ins. Source identifies lower-scope fallback separately. |
| `models_inherited` (provider view) | Whether model **definitions** come from `extends`; independent of local model policy overlays.                                                                                                                            |

For a child with provider S and inherited model R, the model view is local null,
effective R, source `model`, inherited true. With a local model `{}` overlay it
is local `{}`, effective D, source `model`, inherited false, while
`models_inherited` stays true. With no base/child declarations and root R it is
local null, effective R, source `root`, inherited false.

Derivative GET `models` describes the complete inherited inventory with local
policy intent plus effective metadata for each model; definitions are read-only.
PUT sends only sparse local overlays, never those inherited definition fields
or policy metadata. Policy metadata fields are read-only and cannot be written
to bypass the declaration/overlay validators. Static provider/model visibility
uses the same local/inherited distinction under existing RBAC; root has no new
admin edit surface. An undeclared alias has no single target-independent
effective policy, so never expose the first pool member as its effective value.

Compute policy/model metadata with the same declaration-only rules, without
credential decryption/disclosure merely to render it. Managed reads describe
saved local intent and resolution against the relevant base/root snapshot, not
proof that a saved candidate activated; retain durable-save/activation errors
and instance-local reload semantics. UI unchanged saves omit untouched policy
and model collections; other model edits serialize retained **local** policy
intent. Reset emits `{}`, inheritance/removal emits policy null (or drops that
overlay from the replacement array), and type-of-inheritance transitions obey
the explicit-array rule above. Existing workspace/static-edit/credential
authorization boundaries must not widen.

## Task Ordering and Priorities

P1: core feature contract and serving correctness.
P2: configuration surfaces and release verification required before shipping.
There is no emergency defect priority in this plan.

Execute ER-01 first, then ER-02, ER-03, ER-04, ER-05, ER-06, and ER-07.
The user explicitly authorized sequential subagents on 2026-11-16. Use one
task-local subagent session at a time; do not start the next task until its
dependencies pass their task-specific checks and the coordinator reviews the
delivered changes. ER-07 uses a separate review session from the implementers.
No parallel implementation is authorized.
Shared hotspots include config normalization/cloning, admin policy DTOs, and
CLI/editor inputs. Complete the preceding task's changes before editing them.
Keep tests with each implementing task; ER-07 verifies integration rather than
owning all regression tests.

### Task ER-01: Finalize Inheritance and Retry Compatibility

Status: completed

Assigned owner: ER-01 contract subagent

Kind: investigation

Priority: P1 — unresolved semantics affect configuration, retry bounds, and compatibility.

Dependencies: none

Primary ownership: this task file; focused behavioral fixtures if needed.

Finding: scope precedence alone does not define cross-target body propagation,
request-wide retry allowance, `extends`, or managed support. Existing request-wide
alias behavior is material to all downstream tasks.

References: `internal/httpapi/dispatch.go` (`dispatchAlias`, `strippedAttempted`);
`internal/config/build.go` (`buildDerivedProvider`, `validateDerivedModelOverlays`);
`internal/dbmerge/dbmerge.go` (`BuildProvider`).

Requirements:

1. Resolve D1–D3 with the maintainer; record the confirmed choices here.
2. Pin examples for root fallback, empty-block opt-out, alias override, mixed
   target failover, and derived model/provider precedence.
3. State exact maximum inference attempts and body sent at each step of a
   mismatch → stripped retry → retryable failure → second-target sequence.
4. Adjust downstream tasks/dependencies if managed parity or overlays are deferred.

Resolution: all D1–D3 recommendations were approved before ER-01 started.
The finalized semantics above select full managed parity and sparse nested-model
overlays; no deferral or dependency change is needed.

Acceptance criteria:

- No unresolved decision affects downstream behavior; approved examples are
  sufficient to author deterministic tests without conversation history.
- Existing alias behavior is preserved or its approved change is explicitly recorded.
- Managed overlay storage/materialization, inherited-model-versus-local-provider
  precedence, local/effective metadata, and omission/null/replacement semantics
  are concrete enough for downstream API, DB, and mounted UI tests.

Verification: evidence/requirements review; use focused existing-package tests
only if an experiment is needed. No production implementation is required.

Completion evidence (2026-11-16; owner: ER-01 contract subagent):

- Changed only `docs/tasks/20261116-190846-scoped-encrypted-reasoning-policy.md`;
  retained the original analysis/baseline/history and coordinator authorization.
  No production code, temporary fixtures, subagent delegation, or commit.
- Reviewed current `internal/httpapi/dispatch.go` (`dispatchDirect`,
  `dispatchAlias`, shared `strippedAttempted`/working body), reasoning gates/
  stripping/peer helpers in `internal/httpapi/encrypted_reasoning.go`, and
  `internal/httpapi/encrypted_reasoning_test.go` (same-target recovery,
  unconditional strip, direct alias-isolation, text/tool preservation).
  These were **source/test evidence reviews, not newly executed runtime tests**.
- Reviewed `internal/config/{schema,types,build,dynamic,validate,catalog}.go`
  (default/presence, derived surface/model overlays, local credentials, copy
  boundaries), `internal/config/deferred_model_test.go` (existing transport-only
  overlay restrictions), and `internal/modelresolver/resolve.go` (snapshot
  catalog/target selection). The finalized precedence/reset/inheritance tables
  and exact request/attempt fixtures answer ER-01's behavioral acceptance criteria.
- Reviewed `internal/dbmerge/dbmerge.go` (`BuildProvider`'s early derived return,
  cloning/merge path), `internal/store/{models,provider_aggregate}.go` (nullable
  alias storage and transactional model-array replacement), and
  `internal/httpapi/{admin_providers,admin_aliases}.go` (upsert presence, views,
  `buildDBModels`, credential/device-flow rebuilds, `mergedAliasCatalog`, and
  alias null/omission conflation). Chose sparse existing-name reasoning overlays,
  explicit local/effective metadata, legacy inert NULL-row compatibility, and
  policy-bearing transition safeguards; full D3 parity remains required.
- Reviewed `web-ui/src/types.ts`, `web-ui/src/pages/providers-page.tsx`, and
  `website/docs/api-reference.md` (current nested-model replacement/inheritance
  workflow); recorded deterministic create/update/reset/remove/reopen semantics
  for ER-04/ER-06. No unrelated derivative/credential/RBAC relaxation is authorized.
- Verified `make docs-contract` from the repository root: passed
  (`documentation contract matrices match`). Read-only task-file checks passed
  for required fields/dependencies, local links, status isolation, whitespace/
  fenced-block structure, and absence of machine-specific paths/session IDs.
  `git status --short` showed only this already-untracked task document.
- Runtime/package tests, full/race suites, DB checks, web/docs builds, and binary
  integration were **not run by ER-01**: this is a contract/evidence-only task,
  no experiment was needed, and the planning baseline above is not new-feature
  verification. Those required implementation checks remain with ER-02–ER-07.
- Blockers: none. All ER-01 criteria are satisfied by recorded decisions and
  evidence review. ER-02–ER-07 remain pending; coordinator review/handoff is the
  next action, not additional work by this owner.

### Task ER-02: Add Scoped Configuration and Effective Policy Resolution

Status: completed

Assigned owner: ER-02 configuration recovery subagent

Kind: improvement

Priority: P1 — establishes one validated, immutable policy contract.

Dependencies: ER-01

Primary ownership: `internal/config/{schema,types,build,dynamic,validate,catalog}.go`,
associated syntax/inheritance validators, and config/modelresolver tests.

Resume note (2026-11-16): the initial ER-02 subagent stopped with an upstream
encrypted-content session error after saving partial configuration/helpers/tests.
No completion evidence was recorded and no work was discarded. Coordinator
verification `go test ./internal/config ./internal/modelresolver -count=1` passed
modelresolver but failed `TestEncryptedReasoningStaticInheritanceRestrictions`:
standalone/conversion missing-base and self-reference cases reported a deferred
opt-in error instead of the expected owner-specific `extends` error. A fresh
sequential recovery subagent must review all saved changes, fix remaining gaps,
and rerun the required checks before marking ER-02 completed.

Finding: root/provider/model lack policy fields; defaults, normalization,
validation, and cloning are alias-specific.

References: `rawFile`, `rawProvider`, `rawModel`, `rawEncryptedReasoning`;
`Runtime`, `Provider`, `Model`; `NormalizeAlias`, `ApplyProviderDefaults`;
`validateEncryptedReasoning`, `cloneProvider`, `cloneModel`, `cloneAlias`.

Requirements:

1. Implement the confirmed shared contract with shared policy defaulting,
   normalization, validation, deep-copying, and effective resolution.
2. Preserve nil versus explicit reset and false values; validate malformed/duplicate
   blocks and fields in both HCL and JSON with owner-specific errors.
3. Implement D2 without relaxing unrelated derivative, credential, or peer rules.
   Ensure `Models` and `ModelByName` carry consistent independently copied policies.
4. Keep root defaults available to static and database-backed targets without
   flattening inherited defaults into local declarations.
5. Retain relationship/local-declaration provenance needed for ER-01's inherited
   metadata and single-level base checks. Extend static overlay validators
   (including standalone peer/config conversion paths) only for the approved
   reasoning block or reasoning-plus-existing-transport overlay.

Acceptance criteria:

- Table-driven tests cover every precedence edge, absent/empty/false blocks,
  whole-block replacement, custom/default patterns, and disabled providers.
- Inheritance tests cover approved overlays, resets, rejected new models,
  duplicate declarations, and mutation isolation between base/child/catalog copies.
- HCL and JSON produce equivalent declared and effective policies.

Verification: `go test ./internal/config ./internal/modelresolver -count=1`;
shared final gates.

Completion evidence (2026-11-16; owner: ER-02 configuration recovery subagent):

- Recovered and reviewed the initial ER-02 subagent's saved implementation and
  tests; those changes were not discarded or represented as newly authored by
  this recovery owner. No subagents were spawned, no commit was made, and no
  unrelated work or other task statuses were changed.
- Retained scoped schema/build/default/validation/copy plumbing in
  `internal/config/{schema,types,build,dynamic,validate,catalog,load,deferred,convert}.go`,
  shared helpers in `internal/config/encrypted_reasoning.go`, syntax validation in
  `internal/config/encrypted_reasoning_syntax.go`, and root snapshot plumbing in
  `internal/modelresolver/resolve.go`.
- Shared API: `DefaultEncryptedReasoning`, `NewEncryptedReasoning`,
  `NormalizeEncryptedReasoning`, `DeclaredEncryptedReasoning`,
  `CloneEncryptedReasoning`, `CloneEncryptedReasoningDeclaration`,
  `ValidateEncryptedReasoning`, and `ResolveEncryptedReasoning`. Sparse
  `EncryptedReasoning.Declaration` preserves block/member absence, resets,
  explicit false, empty strings/lists, and unnormalized declared patterns.
  Normalized values default whole blocks; legacy boolean struct literals retain
  their existing semantics. Resolution returns a copied effective policy without
  declaration members plus source/inherited metadata.
- `Provider.Extends` and provider/model `EncryptedReasoningInherited` retain
  relationship and copied-versus-local provenance. Base/child/sibling, model
  slice/index, catalog accessors, and resolver results hold independent policy
  pointers/slices. `ApplyProviderDefaults` never flattens root policy into a
  provider/model declaration; `Resolver.EncryptedReasoning(provider, model,
alias)` uses the cloned root snapshot for static or dynamic selected targets.
- Reproduced the interrupted-work failure with
  `go test ./internal/config -run TestEncryptedReasoningStaticInheritanceRestrictions -count=1`.
  Fixed enforcement/error routing by sharing `derivedProviderBase` between
  static construction and standalone/conversion validation instead of treating
  reasoning-only overlays as deferred opt-ins. Missing/self/chained/disabled/
  wrong-type bases and forbidden provider/model declarations still reject;
  credential and peer eligibility/conflict rules remain intact.
- Strengthened duplicate-field tests to require owner context; the first
  combined inheritance/duplicate test run then exposed native HCL duplicate
  attribute diagnostics lacking their policy owner. Added narrowly scoped
  parse-diagnostic annotation and reran the required package checks successfully.
  The saved inheritance assertions were strengthened, not removed to pass.
- Expanded `internal/config/encrypted_reasoning_{scoped,inheritance,validation}_test.go`
  and added `internal/config/encrypted_reasoning_catalog_test.go` plus
  `internal/modelresolver/encrypted_reasoning_test.go`. Coverage includes all
  precedence edges, whole-block defaults, absent/reset/false declarations,
  byte-measured pattern bounds, disabled providers, HCL/JSON parity and sparse
  round trips, approved reasoning/transport/both overlays, transport-only
  reasoning inheritance, peer/credential restrictions, no-I/O standalone
  validation, mutation isolation, selected-target resolution, and retained
  old/new/removed-root resolver snapshots.
- Checks run from the repository root on the shared, uncommitted ER-02 checkout:
  - `go test ./internal/config ./internal/modelresolver -count=1`: passed both
    packages after recovery and final test additions.
  - `go test -race ./internal/config ./internal/modelresolver -count=1`: passed
    both packages; no race diagnostics.
  - `go vet ./internal/config ./internal/modelresolver`: passed.
  - `go test ./internal/httpapi ./internal/dbmerge -run 'EncryptedReasoning|DeferredModel' -count=1`:
    filtered consumer smoke passed, but was not full alias-behavior coverage.
  - `go test ./internal/httpapi -run 'EncryptedReasoning|Strip|CallerMismatch|BoundedCompressedInspection|DeferredModel' -count=1`:
    existing alias/strip/mismatch/compressed-inspection and managed-deferred
    regression smoke passed; not verification of ER-03's new runtime behavior.
  - `go test -v ./internal/dbmerge -run 'EncryptedReasoning|DeferredModel' -count=1`:
    hermetic base-rejection test and its 28 subcases passed; the one DB-backed
    merge test skipped because `AIPROXY_TEST_DATABASE_URL` is unset. This is not
    managed-policy or database acceptance evidence for ER-04.
  - `gofmt -l` over all ER-02 Go paths returned no files; `git diff --check`
    passed. Task-file status/link/whitespace/private-metadata checks passed.
- Blockers: none for ER-02. Runtime dispatch/reload admission behavior, managed
  policy codecs/storage/admin views, CLI/UI editing, and the full shared gates
  remain ER-03–ER-07; no live providers or database were exercised here. ER-04
  must use inherited provenance when rendering owner-local declarations, not
  serialize a copied base declaration or effective resolution as a local write.
  Coordinator review and the sequential ER-03 handoff are the next action.

### Task ER-03: Apply Policies to Direct and Alias Inference Attempts

Status: completed

Assigned owner: ER-03 routing verification subagent

Kind: improvement

Priority: P1 — new scopes must change serving behavior without unsafe replay.

Dependencies: ER-02

Primary ownership: `internal/httpapi/{dispatch,encrypted_reasoning}.go`,
runtime dependency/resolver plumbing as needed, `internal/app/app.go`, and focused
HTTP, reload, and peer-deferred tests.

Resume note (2026-11-16): the ER-03 subagent stopped at the model provider's token
rate limit after saving dispatch/helpers and focused runtime/reload/peer tests.
Coordinator reviewed the saved routing diff and test coverage, then verified
`go test ./internal/httpapi ./internal/app ./internal/modelresolver -count=1`
and `go test -race ./internal/httpapi ./internal/app ./internal/modelresolver -count=1`
with `AIPROXY_TEST_DATABASE_URL` set to an isolated disposable PostgreSQL instance:
both passed in all three packages; `git diff --check` also passed. A continuation
of the original subagent also hit the provider rate limit before recording
completion. A fresh, narrowly scoped sequential verification subagent reconciled
remaining criteria and recorded completion below; no saved work or task
dependency was discarded/bypassed.

Finding: only aliases apply policies; current reasoning retry state is global to
an alias dispatch. Root policy transport to the request snapshot is not present.

References: `dispatchDirect`, `dispatchAlias`, `encryptedReasoningEffective`,
`encryptedReasoningApplies`, `stripDispatchOpaqueBlocks`, `isDispatchCallerMismatch`;
`internal/app/app.go` (candidate resolver and dependency activation).

Requirements:

1. Use ER-02's resolver for direct requests and each selected alias target;
   implement the confirmed D1 body propagation and recovery budget.
2. Preserve same-target identity, request deadline/cancellation, inbound body
   cloning, capability/auth checks, normal alias failover, and cooldown isolation.
3. Close discarded results exactly once. Preserve lease release, stream
   finalizers, per-attempt upstream metrics/logs, and existing inbound usage/quota
   accounting semantics; do not charge an additional inbound request for recovery.
4. Reuse bounded compressed inspection and peer-aware budget helpers. No peer
   control error/replay/finalization path may authorize fresh inference.
5. Wire startup and reload snapshots consistently; verify invalid reload rollback
   and in-flight request/stream snapshot retention.
6. Encode ER-01's body/identity/attempt sequence table as deterministic fixtures,
   including pre-stripping that does not spend a later target's recovery allowance
   and configured `400` failover after the shared allowance is exhausted.

Acceptance criteria:

- Hermetic tests cover root/provider/model direct strip and recovery on chat and
  Responses; absent policies and alias-only declarations still leave direct calls alone.
- Mixed-policy alias tests assert target identity, each body, exact call count,
  request-wide recovery bound, alias overrides, and normal failover after recovery.
- Negative/boundary tests cover unrelated `400`, `401`/`403`/`5xx`, custom patterns,
  no opaque markers, unchanged rewrite, cancellation, immediate streaming errors,
  established SSE, and unsupported operations including native messages.
- Existing compressed-inspection and peer-control tests remain valid, with direct
  policy and deferred execution coverage added.
- Reload and close/lease/accounting assertions demonstrate no cross-snapshot
  policy changes, duplicate finalization, or extra inbound accounting.

Verification: `go test ./internal/httpapi ./internal/app ./internal/modelresolver -count=1`;
`go test -race ./internal/httpapi ./internal/app ./internal/modelresolver`;
shared final gates.

Completion evidence (2026-11-16):

- Delivered by the ER-03 routing implementation subagent:
  `internal/httpapi/{dispatch,encrypted_reasoning}.go`,
  `internal/httpapi/encrypted_reasoning_{boundaries,compressed,deferred,runtime,scoped}_test.go`,
  `internal/app/encrypted_reasoning_reload_test.go`, and focused fixture updates in
  `internal/httpapi/{deferred_caller,deferred_delivery,deferred_model,handler}_test.go`.
  Startup/reload use ER-02's snapshot-bound resolver through the existing
  `internal/app/app.go` dependency activation path; no new app runtime wiring was
  required.
- The ER-03 routing verification subagent reviewed the saved runtime diff and
  focused source/test assertions against the approved contract; no acceptance
  gap or additional runtime/test edit was found:
  - `TestScopedEncryptedReasoningDirect` and
    `TestScopedEncryptedReasoningAliasSequences` assert chat/Responses precedence,
    unchanged absent/alias-only direct behavior, same identity/context/deadline,
    cloned inbound bodies, maximum two direct / N + 1 alias attempts, explicit
    alias sticky bodies versus target-local admitted bodies, pre-strip without
    spending recovery, and configured `400` failover after allowance exhaustion.
  - Boundary/compressed/runtime tests assert custom-pattern and opaque-marker
    gates, actual changed rewrites, bounded successful decoding, untouched
    non-recovered bytes/headers, cancellation, immediate HTTP errors versus
    established SSE, native messages/other-operation exclusion, admission
    guards, and guardrail-admitted-body preservation.
  - Runtime/deferred tests assert once-only result/body close, selected lease
    retention and release, direct cooldown isolation, normal alias retry advice,
    per-attempt logs/metrics, final-result-only inbound usage/quota accounting,
    work/memory/shape budgets, and no fresh inference from peer controls,
    replay, deletion, or finalization errors.
  - `internal/app/encrypted_reasoning_reload_test.go` asserts startup and valid
    reload policy changes, invalid-candidate rollback, old identity/policy/body
    retention through in-flight recovery/failover and established SSE, and
    resource-lease release after completion.
- Coordinator-executed verification on the saved runtime/test code, from the
  repository root with `AIPROXY_TEST_DATABASE_URL` set to isolated disposable
  PostgreSQL (not production):
  - `go test ./internal/httpapi ./internal/app ./internal/modelresolver -count=1`:
    PASS all three packages (42.625s / 23.010s / 0.017s, respectively).
  - `go test -race ./internal/httpapi ./internal/app ./internal/modelresolver -count=1`:
    PASS all three packages (147.174s / 30.122s / 1.060s, respectively).
  - `git diff --check`: PASS.
    These commands were run by the coordinator, not rerun or claimed as executed
    by the verification subagent; the reviewed runtime/test files were not changed
    during finalization.
- Verification-subagent checks, from the repository root:
  `go vet ./internal/httpapi ./internal/app ./internal/modelresolver` passed;
  `gofmt -l` over the delivered ER-03 Go paths returned no files.
  Final task-status/private-metadata/whitespace checks and `git diff --check`
  passed after the completion record was added.
- Blockers: none for ER-03. This is focused runtime finalization, not the final
  integrated review or live-provider compatibility evidence. ER-04–ER-07 remain
  pending; managed parity, CLI/public docs, UI work, and shared final gates are
  not completed by these checks. The next sequential task is ER-04.

### Task ER-04: Preserve Policy Declarations in Managed Catalogs

Status: completed

Assigned owner: ER-04 admin/API subagent (second sequential work unit)

Kind: improvement

Priority: P2 — prevents managed configuration from silently diverging from serving policy.

Dependencies: ER-03

Primary ownership: `internal/store/` provider/model persistence and migrations,
`internal/dbmerge/dbmerge.go`, `internal/httpapi/{admin_providers,admin_aliases}.go`,
shared admin reasoning DTOs/validators and declaration-only materialization,
and associated store/admin/merge tests.

Sequential work units: first implement/test nullable storage, migrations, shared
policy codecs, and database catalog materialization; after coordinator review,
a fresh admin/API subagent implements read/write intent, metadata, authorization,
and transition safeguards. This bounded split reduces repeated large session
context after upstream rate-limit interruptions. ER-04 remains in progress until
both units and all task-specific checks pass; ER-05 cannot start early.

Coordinator first-unit review: inspected strict codecs, nullable migration and
storage normalization, credential-free materialization, inheritance/overlay
enforcement, and recorded test evidence. Independently reran
`go test ./internal/store ./internal/dbmerge -count=1` with the isolated disposable
PostgreSQL configuration; both packages passed. The second admin/API unit may
now proceed; the first unit's delivered work is preserved.

Finding: provider/model rows and admin DTOs omit policy declarations, while aliases
already persist the feature. Managed derivatives have a separate inheritance path.

References: `DBProvider`, `DBProviderModel`, `DBAlias`; `internal/store/store.go`
(`migrations`); `BuildProvider`; `adminProviderView`, `adminModelView`.

Requirements:

1. Implement mandatory D3 parity: add nullable local policy storage and
   idempotent migrations for both fresh and upgraded schemas; retain null versus
   empty reset objects and false values.
2. Apply shared normalization/validation on provider/model creation, updates,
   credential-only updates that rebuild providers, and stored-row activation.
   Corrupt stored policies reject candidates with the old runtime intact.
3. Implement the confirmed managed inheritance/overlay behavior and separate local
   declarations from effective read-only policy metadata.
4. Preserve existing static edit restrictions, workspace authorization, credential
   secrecy, durable-save/activation semantics, and instance-local reload behavior.
5. Use ER-01's sparse nested-model representation, legacy NULL-row compatibility,
   collection/transition rules, and named metadata fields. Differentiate policy
   omission from explicit null removal, including shared alias codec/update paths.
   Reject derivative redefinitions by presence, even empty/null; credential-only
   edits cannot mutate policies. Alias validation must see inherited served models,
   not just the stored overlay rows.

Acceptance criteria:

- Existing rows inherit defaults without being rewritten as explicit policies.
- Create/read/update/reset/remove/reopen tests preserve exact local intent at both
  provider and model scopes; root updates reach undeclared managed targets.
- Approved roles can edit their authorized targets only; unauthorized and
  unsupported writes fail before commit. Credential updates retain policy intent.
- Fresh/legacy/repeated migrations and invalid stored-row activation are tested.
- Managed/static-base derivatives cover provider and model inheritance/reset,
  sparse overlay reopen/remove, rejected new/redefined models and chains, legacy
  inert rows, explicit-array transitions, and consistent read/serve model inventory.

Verification: `go test ./internal/store ./internal/dbmerge ./internal/httpapi -count=1`.
DB-backed assertions require `AIPROXY_TEST_DATABASE_URL` pointing at a disposable
PostgreSQL database with the repository fixtures' schema-creation permissions;
inspect verbose output and do not treat skipped tests as completion. Shared final gates.

First-unit evidence and coordinator handoff (2026-11-16):

- ER-04 remains `in_progress`. This unit implemented and verified storage/codecs/
  materialization only; no independent coordinator review is claimed. ER-05–ER-07
  remain pending, and the next admin/API unit must run sequentially after review.
- Storage: `internal/store/{models.go,schema.sql,store.go,encrypted_reasoning.sql,
encrypted_reasoning.go,provider_aggregate.go,crud.go}` add nullable provider/model
  JSONB declarations and canonicalize new whole-policy JSON null writes to SQL NULL,
  including aliases. `{}`, explicit false, sparse optional members, and declared
  empty lists remain distinct. Aggregate credential edits retain model rows and
  provider declarations; replacement arrays retain the supplied local intent.
  Migration tests verify fresh/legacy/repeated application, unchanged legacy row
  bodies and PostgreSQL tuple versions, and no legacy model deletion/rewrite.
- Shared codecs in `internal/config/encrypted_reasoning_json.go`:
  `DecodeEncryptedReasoningJSON(raw []byte, owner string)` returns a normalized
  `*EncryptedReasoning` with independent sparse `Declaration`, or nil for omitted/
  whole-value null input. It rejects wrong types, null members/list entries,
  unknown fields, duplicate/case-variant keys, trailing values, and existing
  policy/pattern bound violations with owner context. Existing empty mismatch
  string and empty-list defaults remain unchanged.
  `EncodeEncryptedReasoningJSON(policy *EncryptedReasoning, owner string)`
  validates and returns declaration-only `json.RawMessage`; nil means SQL NULL,
  not a default-filled block. Encoding never substitutes effective members for
  declared ones. The API caller must still distinguish omitted raw fields from
  explicit null before invoking the codec; outer request-key presence/duplicate
  guards belong to the next unit.
- `dbmerge.MaterializeProvider(row, models, bases)` is the shared credential-free
  declaration/inventory builder. It returns complete concrete/inherited model
  definitions, normalized declarations, `Extends`, and provider/model inherited
  flags with independently cloned slice/index policies. It does not attach,
  decrypt, or disclose API/Copilot credentials, resolve credential files, apply
  root reasoning fallback, or prove full credential/runtime validity.
  `BuildProvider` delegates to it, then attaches only the row's local credential.
  Admin views and alias validation should reuse this materializer rather than
  reinterpret stored sparse rows; apply runtime provider defaults as appropriate,
  then use `ResolveEncryptedReasoning` with the relevant root snapshot.
- `internal/dbmerge/{dbmerge.go,encrypted_reasoning.go}` preserves enabled,
  concrete, same-type, single-level bases, including static-derived/managed
  chains and a managed row shadowed by an older static concrete entry.
  Custom TLS/deferred inheritance rejection and local credential isolation remain
  intact. Non-null model policies activate existing-name sparse overlays only;
  null-policy legacy definitions remain inert. Local blocks replace inherited
  blocks wholesale. Stored aliases use the same strict codec; existing full
  non-null alias semantics remain explicit, and `{}` is a safe reset.
- New tests: `internal/config/encrypted_reasoning_json_test.go`,
  `internal/store/encrypted_reasoning_test.go`, and
  `internal/dbmerge/{encrypted_reasoning_test.go,encrypted_reasoning_database_test.go}`.
  Coverage includes NULL/reset/false/optional-member persistence, aggregate
  rollback, root/dynamic fallback and admitted resolver snapshots, managed/static
  inherited layers and overlay removal, legacy inert rows, invalid active
  overlays/chains/disabled bases, copy isolation, corrupt stored policies, and
  alias validation against complete served inventory. Rejected database merges
  leave the supplied old catalog/root and admitted resolver snapshot unchanged.
- Supporting test-only adjustments: the exact migration lists in
  `internal/store/{spend_ledger_test.go,copilot_credential_test.go}` include the
  additive migration. Store concurrency/Copilot rollback assertions in
  `internal/store/{provider_aggregate_test.go,copilot_device_flow_test.go,
copilot_device_flow_lock_test.go}` compare timestamp instants independently of
  `Local`/`UTC` location representation. The active `newRT` fixture in
  `internal/httpapi/handler_test.go` explicitly sets its base provider enabled;
  no HTTP implementation, admin DTO, CLI, UI, or public documentation changed.
- Verification, working directory repo root: `go test ./internal/config -count=1`,
  `go test ./internal/store ./internal/dbmerge -count=1`,
  `go test ./internal/httpapi -count=1`,
  `go vet ./internal/config ./internal/store ./internal/dbmerge`,
  `go test -race ./internal/config ./internal/store ./internal/dbmerge -count=1`,
  `make vet test`, and `git diff --check` passed. Database commands used the
  configured disposable PostgreSQL URL, never production. Verbose focused
  `TestEncryptedReasoning(Migration|Store)` store tests and
  `TestMergeCatalogEncryptedReasoning|TestMaterialize` merge tests passed with
  no skips; every new database fixture owns an isolated schema.
- Resolved check failures: the first legacy migration assertion incorrectly
  scanned scalar SQL NULL into a Bun raw-message slice; it now checks SQL
  `IS NULL` directly. The full store run exposed location-only timestamp
  comparison failures, and the first full repository run exposed the missing
  enabled flag in the active base fixture; fixes above were followed by passing
  package/full/race checks. No unresolved first-unit blocker remains.
- Remaining ER-04 work: administrative omission/null/removal intent and strict
  outer/nested request-key presence guards; provider/model/alias create/update/
  credential validation before commit; explicit-array concrete/derived
  transition safeguards and preserved-overlay validation against changed bases;
  complete saved-intent model views with the approved local/effective/source/
  inherited metadata under existing RBAC and credential secrecy; consistent
  declaration-only base lookup/alias target validation; durable-save/activation
  regressions and actual managed invalid-policy reload integration coverage.
  This unit's merge/snapshot tests are not a substitute for those API/reload
  tests. Conversion/CLI/public docs and web forms remain ER-05/ER-06 work.

Second-unit completion evidence and coordinator handoff (2026-11-16):

- ER-04's remaining admin/API acceptance criteria and required database checks
  passed. The first-unit work and all earlier implementation changes were
  preserved. This second unit did not spawn another subagent or commit changes;
  coordinator review of this unit is still required before starting ER-05.
- HTTP implementation: `internal/httpapi/{admin_providers.go,admin_aliases.go,
admin_encrypted_reasoning.go,admin_provider_inventory.go}` now uses the shared
  strict policy codecs and credential-free `dbmerge.MaterializeProvider` for
  saved provider/base inventories, views, and alias validation. Provider, model,
  and alias declarations remain sparse; effective metadata is never persisted.
  Provider/alias PUT omission preserves the exact stored local declaration,
  null removes it, `{}` resets it, and nonempty objects replace it wholesale.
  Omitted/null model collections preserve rows; arrays replace the complete
  local set. Concrete entries carry full definitions plus nullable policies.
  Derivative entries accept only existing `name` and `encrypted_reasoning`;
  validated name-only/null entries are canonicalized away, and empty arrays
  remove overlays without removing inherited definitions.
- Presence guards reject forbidden derivative fields even when empty/null,
  duplicate names/keys, policy/model case variants, unknown overlay keys, and
  read-only metadata. Duplicate outer collections cannot erase forbidden
  declarations, including deferred declarations. Credential PUT rejects policy
  and model-edit keys, preserves exact provider/model intent, and validates the
  rebuilt candidate before commit. Provider durability headers,
  saved-but-activation-failed outcomes, and receiving-instance-only activation
  behavior are unchanged. Existing TLS selector name-folding and unrelated
  ignored credential-field compatibility remain intact; no existing
  assertion was removed or weakened.
- Reads expose the pinned nullable owner-local `encrypted_reasoning`, normalized
  `encrypted_reasoning_effective`, winning `encrypted_reasoning_source`, winning
  base-copy-only `encrypted_reasoning_inherited`, and definition-only
  `models_inherited`. Derivative `models` is the complete inherited inventory
  with local overlays and effective metadata. Static views use the same
  local/inherited distinction. Alias declarations are nullable and sparse;
  `encrypted_reasoning_effective` is null for undeclared aliases, not a selected
  target's policy. Workspace visibility/edit guards and credential secrecy are
  retained, including avoiding a second config-source view of database resources.
- Minimal supporting changes in `internal/config/types.go` and
  `internal/dbmerge/dbmerge.go` retain internal `DatabaseManaged` provenance on
  materialized providers and merged aliases (`json:"-"`). This prevents an old
  admitted, subsequently deleted database base from masquerading as static
  saved intent or passing alias validation after a rejected reload; recreating
  that base remains possible. `Extends` chain/disabled/type/custom-TLS/deferred
  restrictions are still enforced. Explicit-array policy-bearing transitions,
  provider-local intent across base changes, and preserved overlays against a
  new base are validated before commit.
- New tests: `internal/httpapi/{admin_reasoning_helpers_test.go,
admin_encrypted_reasoning_test.go,admin_reasoning_inheritance_test.go,
admin_reasoning_validation_test.go,admin_reasoning_authorization_test.go}` and
  `internal/app/encrypted_reasoning_managed_reload_test.go`. Coverage includes
  create/reopen/reset/remove/false/custom declarations, exact omission/null
  collection preservation, static/managed inheritance and legacy inert rows,
  credential-only preservation/rejection/corruption, authorized and unauthorized
  roles/workspaces/static targets, strict key presence/duplicates, transitions,
  saved-base alias inventory, and saved-versus-activated metadata. Real
  application tests verify upstream bodies and local credentials, root fallback,
  alias removal, corrupt provider/model/alias reload rollback, dangling overlays,
  durable failed activation, base deletion, and explicit repair/recreation.
  Every new database fixture owns an isolated schema.
- Verification, working directory repo root, with the configured disposable
  PostgreSQL database: `go test ./internal/store ./internal/dbmerge
./internal/httpapi -count=1`, `go test ./internal/app -count=1`, and
  `go vet ./internal/config ./internal/store ./internal/dbmerge ./internal/httpapi
./internal/app` passed. Verbose `go test ./internal/store ./internal/dbmerge
./internal/httpapi -run 'EncryptedReasoning|TestMaterialize' -count=1 -v` and
  `go test ./internal/app -run TestEncryptedReasoningManaged -count=1 -v` passed
  with no skips. `go test -race ./internal/httpapi ./internal/app -run
'TestAdminEncryptedReasoning|TestEncryptedReasoningManaged' -count=1 -v` passed
  with no skips or race reports. A final required-package rerun with
  `GOFLAGS='-p=1'` also passed.
- Resolved check failures: the initial HTTP run exposed established TLS
  selector/credential unknown-field compatibility; the narrowed guards above
  restored it. The first new inheritance fixture used invalid single-line HCL
  nested blocks and was corrected. Default parallel `make vet test` encountered
  shared-schema interference between older HTTP/merge fixtures: merge saw
  unrelated HTTP aliases/credentials and an HTTP presentation encountered a
  concurrently removed deferred fixture base. `GOFLAGS='-p=1' make vet test`
  passed in an additional run-owned disposable schema, which was cleaned up.
  Only the exact orphan row created by that failed setup was removed from the
  shared disposable database; no unrelated rows or database were dropped.
- Blockers: none for ER-04. CLI/conversion/public docs, web types/forms, and the
  complete final gate set remain ER-05–ER-07 work. This is implementation and
  focused verification evidence, not an independent integrated review or
  live-provider compatibility claim. ER-05 is ready for the next sequential
  handoff after coordinator acceptance.

### Task ER-05: Support Conversion, CLI Editing, and Public Documentation

Status: completed

Assigned owner: ER-05 documentation subagent (third sequential work unit)

Kind: improvement

Priority: P2 — policy intent must survive configuration workflows and be documented accurately.

Dependencies: ER-04

Primary ownership: `internal/config/convert.go`, `internal/configedit/`,
`cmd/aiproxy/configure.go`, related tests, `examples/`, `README.md`, `AGENTS.md`,
`docs/design.md`, and affected `website/docs/` pages.

Coordinator ER-04 review: inspected admin declaration/effective metadata, strict
key/policy validation, sparse model materialization and transition guards, and
saved-versus-active origin handling. Independently verified
`GOFLAGS='-p=1' go test ./internal/store ./internal/dbmerge ./internal/httpapi -count=1`
with the disposable PostgreSQL configuration; all passed, as did
`git diff --check`. ER-04's delivered work is preserved.

Sequential ER-05 units: conversion/shared editor support first, CLI integration
second, then public documentation/examples/release notes after flag syntax is
settled. Use fresh bounded contexts and coordinator review between units.
ER-05 stays in progress until the combined criteria and verification pass.

Finding: CLI/editor reasoning controls are alias-specific; the JSON-to-HCL block
registry omits even the existing alias block. Public routing docs currently state
that direct requests are never stripped.

References: `jsonBlockLabels`, `writeHCLBody`; `AliasEncryptedReasoningInput`,
alias rendering; `applyAliasEncryptedReasoning` and alias flags;
`website/docs/providers-and-routing.md` (Alias Encrypted Reasoning).

Requirements:

1. Register the block for conversion and test HCL → JSON → HCL and JSON → HCL →
   JSON for all supported scopes, including existing alias policies and resets.
2. Add root/provider/model editing using established CLI conventions. Decide exact
   model-scoped input syntax within current provider-model editing workflows;
   document/help-test it without introducing an unrelated command redesign.
3. Omitted edit flags preserve local intent; removing a block means inheritance,
   not disablement. Preserve unrelated blocks, comments where supported, and
   `env()` spelling in in-place edits; conversion retains its existing env resolution.
4. Update contracts/examples for precedence, whole-block defaults, reset versus
   inherit, D1 retry bounds, D2 inheritance, D3 managed support, endpoints, streams,
   compressed inspection, and safe defaults. Add a release-note entry according
   to repository conventions, especially for the revised direct-request contract.
5. Document ER-01's nullable policy versus nullable `models` distinction, sparse
   derivative writes/read-only inherited views, and the intentional alias-null
   removal clarification. Do not promise compatibility for previously ignored
   derived model redefinitions or unsupported inheritance chains.

Acceptance criteria:

- Conversion accepts aliases with policies and round-trips every new scope
  without losing false/reset/local-versus-inherited distinctions.
- CLI create/edit/no-op/reset/remove and unrelated-provider/root edits retain
  existing and new policies; invalid input does not publish a changed config.
- Documentation examples load in hermetic config tests and make no claim of
  blanket error recovery or universal plaintext-reasoning preservation.

Verification: `go test ./internal/config ./internal/configedit ./cmd/aiproxy -count=1`;
`make docs-contract`; `pnpm docs:build`; shared final gates.

First sequential unit evidence (2026-11-16; conversion/shared editor only):

- Changed: `internal/config/encrypted_reasoning_convert_test.go`,
  `internal/configedit/configedit.go`, `internal/configedit/encrypted_reasoning.go`,
  and focused `encrypted_reasoning_scoped_test.go`,
  `encrypted_reasoning_preservation_test.go`, and
  `encrypted_reasoning_derived_test.go` under `internal/configedit/`.
- Retained ER-02's zero-label `jsonBlockLabels` registration without rewriting
  conversion. Tests prove both round-trip directions, compact/noncompact output,
  root/provider/model/explicit-target and shorthand aliases, absent versus reset
  versus false, explicit empty strings/lists, static reasoning/deferred/combined
  overlays, and no flattening of inherited declarations/model definitions.
  Static JSON null/invalid policies fail conversion rather than disappearing.
  Conversion still resolves environment calls; editor source retention does not
  change `env()` expansion into quoted strings. Boolean expressions such as
  `env("ER_PASS") == "true"` and list entries `[env("ER_PATTERN")]` work; a bare
  boolean `env()` value remains invalid under the existing strict policy parser.
- Shared API: `EncryptedReasoningInput` retains the original three alias fields,
  adds `HasOnCallerMismatch`/`HasMatchMessages` for explicit empty members, and
  retains authoritative `Block`/`Expressions` source. The existing
  `AliasEncryptedReasoningInput` is a compatible type alias; provider/model
  inputs now also have `EncryptedReasoning`. Use
  `EncryptedReasoningInputFromDeclaration` and `CloneEncryptedReasoningInput`
  for deep-copied local declarations, never inherited effective values.
- `ParseEncryptedReasoningBlock`/`ReadEncryptedReasoning` retain unevaluable
  expressions, presence, and policy comments without coercing values to defaults.
  `RenderEncryptedReasoningBlock` works at root and is reused by provider/model/
  alias renderers. Source-backed inputs must be changed through
  `ApplyEncryptedReasoningEdit`, not by mutating their legacy literal fields:
  edit member pointers distinguish omitted from false/empty; `Reset` declares an
  empty block; `Remove` inherits; `Expressions` accepts raw expression changes
  (nil removes a member, absent map entries preserve it).
- `PatchEncryptedReasoningBlock` replaces only changed expression ranges and
  preserves untouched expressions/comments; `UpdateEncryptedReasoning` patches
  a document using zero-value root or named provider/model/alias
  `EncryptedReasoningScope`. It handles inline blocks and removes the enclosing
  reasoning-only derived model overlay when necessary; combined deferred blocks
  survive. It requires an existing local owner/model block and deliberately
  rejects last-overlay cleanup with an unevaluable `extends` rather than
  guessing. Creating an inherited-model overlay must use a sparse
  `ProviderModelInput` and existing candidate validation.
- Existing write preflight already reaches ER-02's shared policy validation.
  Tests load successful output under synthetic environments and prove invalid
  policy/env/derived-overlay edits leave config and staged credentials unchanged.
  Existing credential/disabled/deferred/TLS/editor and CLI package tests pass.
- Verified at repo root: `GOFLAGS='-p=1' go test ./internal/config ./internal/configedit ./cmd/aiproxy -count=1`;
  focused `GOFLAGS='-p=1' go test ./internal/config ./internal/configedit -run 'Test(ConvertEncryptedReasoning|EncryptedReasoning|RenderProviderAndAliasSharedEncryptedReasoningInputs)' -count=1`;
  scoped `go vet` with `GOFLAGS='-p=1'`, `GOFLAGS='-p=1' make vet`, and
  `git diff --check` all passed. Full `make test` was not run: no disposable test
  database is configured and the available Docker endpoint is inaccessible;
  database tests were not skipped as a replacement for that gate. Docs checks
  remain with the later documentation unit.
- CLI handoff: wire `existingProviderInput` (including nested models) and
  `existingAliasInput` to the shared raw parser/read helpers; replace
  `applyAliasEncryptedReasoning`'s literal copying with the shared edit helper.
  Preserve the new fields through `promptProviderInput`'s reconstructed literals,
  `promptProviderModelInteractive`, model-option replacement, and
  `preserveProviderModelDeferredTransports`/
  `preservedProviderDeferredModelOverlays` in `cmd/aiproxy/configure_deferred.go`.
  Root controls can use the shared renderer plus established upsert/removal
  paths. Add CLI flags/help/create/edit/reset/remove/no-op tests and handle sparse
  derived overlay creation without flattening runtime inventory. No command
  workflows were modified, so shared editor tests are not end-to-end CLI evidence.
  ER-05 stays `in_progress`; CLI integration and public docs/release notes remain.
  ER-06/ER-07 stay pending; no independent review or live-provider claim is made.

Coordinator editor-unit review: inspected shared source-backed inputs,
expression/member edits, overlay rendering, and scoped updates. Independently
reran `GOFLAGS='-p=1' go test ./internal/config ./internal/configedit ./cmd/aiproxy -count=1`;
all passed. The coordinator's disposable database is available for later full
checks despite the first unit's unset subprocess database variable. Root policy
flags belong on the existing `configure upstream` workflow, not a new root command.
The CLI unit must wire raw-block helpers, preserve untouched expressions on
member edits, and handle missing/last derived overlays without guessing `extends`.

Second sequential unit evidence and docs handoff (2026-11-17; CLI integration):

- Changed: `cmd/aiproxy/configure.go`, `cmd/aiproxy/configure_deferred.go`,
  new `cmd/aiproxy/configure_encrypted_reasoning.go`, and focused
  `configure_encrypted_reasoning_test.go`,
  `configure_encrypted_reasoning_preservation_test.go`, and
  `configure_encrypted_reasoning_derived_test.go` under `cmd/aiproxy/`.
  One existing deferred text-prompt fixture now explicitly selects preserve/no.
  The only additional shared-editor change is rendering `extends` through
  `RenderStringOrExpression`, so ordinary environment-backed base references
  survive provider reconstruction instead of becoming quoted expressions.
  Prior ER-01–ER-04/editor-unit work is preserved; no subagents or commits were made.
- CLI contract: the same existing alias flag names now work on
  `configure upstream` (root), `configure provider` (provider), and
  `configure alias` (alias). No new root/model command was introduced:

  | Flag                                                             | Local edit                                                                                                                                 |
  | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
  | `--encrypted-reasoning-passthrough[=true\|false]`                | Explicit boolean member; use `=false` to strip. Omission preserves.                                                                        |
  | `--encrypted-reasoning-on-caller-mismatch fail\|strip_and_retry` | Explicit mismatch member; `--encrypted-reasoning-on-caller-mismatch=` writes the empty string selecting `fail`.                            |
  | `--encrypted-reasoning-match-message TEXT`                       | Repeatable literal substring; replaces this scope's list. A single `--encrypted-reasoning-match-message=` writes `[]` selecting built-ins. |
  | `--no-encrypted-reasoning`                                       | Remove the local block and inherit; legacy alias compatibility retained.                                                                   |
  | `--reset-encrypted-reasoning`                                    | Declare a present empty `{}` block, not inheritance.                                                                                       |

  Provider model controls follow the existing `name=value` convention:

  | Flag                                   | Value syntax                                                                                                                   |
  | -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
  | `--model-reasoning-passthrough`        | `name=true` or `name=false`; repeat for different models.                                                                      |
  | `--model-reasoning-on-caller-mismatch` | `name=fail`, `name=strip_and_retry`, or `name=` for explicit empty/default.                                                    |
  | `--model-reasoning-match-message`      | `name=substring`; repeat per model to replace that model's list. A single `name=` writes `[]`. Quote values containing spaces. |
  | `--no-model-reasoning`                 | A model name; repeat to remove local blocks/inherit.                                                                           |
  | `--reset-model-reasoning`              | A model name; repeat to declare local `{}` blocks.                                                                             |

  Duplicate per-model scalar/control edits fail instead of silently winning.
  Remove/reset/member edits conflict only at the same scope/model. A single empty
  pattern value is the explicit default-list control; an empty entry combined
  with other entries is invalid. Existing normalization and 16-entry/4–256-byte
  bounds remain enforced before publication. Boolean remove/reset controls passed
  as `=false` are no-ops. Members omitted from an existing local block are
  preserved; a newly declared block never copies lower-layer effective fields.

- `configure upstream` no longer requires a timeout to change only policy and
  does not rewrite an untouched timeout expression. Existing policy-only provider/
  alias edits use targeted source updates, including no-op byte preservation.
  `existingProviderInput`/`existingAliasInput` use raw shared read/parse helpers;
  alias member edits use `ApplyEncryptedReasoningEdit` rather than mutating
  source-backed literal fields. Provider prompt reconstructions, TUI/text model
  edits, model-option replacement, and both deferred model-preservation helpers
  retain local reasoning source, explicit empties, and combined deferred siblings.
- Model controls alone never create a model definition. Concrete provider model
  flags keep their existing replacement semantics; reasoning edits reconcile
  against the resulting retained/explicitly created model names. Derivatives
  still reject inherited-field flags, new/redefined inherited models, chains, and
  credential/deferred bypasses. Missing child overlays use the validated base
  inventory and sparse model blocks only. Removing a reasoning-only child overlay
  removes its enclosing bare model block; concrete definitions and deferred
  sibling blocks remain. Environment-backed `extends` cleanup uses a validated
  config snapshot with the lexical filename, not raw-expression guessing.
- Interactive root/provider/alias and selected-model policy menus distinguish
  preserve/inherit/reset/override. Override member choices preserve untouched raw
  expressions; mismatch/default-list choices write explicit empty members.
  Unchanged inputs, including a selected inherited model with no local overlay,
  never materialize inherited/default policies. The optional line-oriented policy
  menus also preserve when an older input stream ends before the new menus.
- Focused Cobra/file tests cover create/update/no-op/reset/remove, whole-block
  precedence, legacy shorthand/explicit-target aliases, explicit false/empty
  members, normalized byte bounds, slash-segment model names, disabled providers,
  concrete model replacement, sparse/environment-backed derivatives, combined
  reasoning/deferred overlays, text/TUI reconstruction, and invalid/conflicting
  flags or environment-backed policies leaving config/staged secrets unchanged.
  New reasoning command examples are help-checked and executed against hermetic
  fixtures; every successful test output is loaded as config. No live upstream
  calls or production credentials are used.
- Verified at repo root: focused
  `GOFLAGS='-p=1' go test ./cmd/aiproxy -run '^TestConfigureEncryptedReasoning' -count=1`,
  final `GOFLAGS='-p=1' go test ./internal/config ./internal/configedit ./cmd/aiproxy -count=1`,
  scoped `GOFLAGS='-p=1' go vet ./internal/config ./internal/configedit ./cmd/aiproxy`,
  and `git diff --check` passed. Full `GOFLAGS='-p=1' make vet test` passed with
  the approved disposable `AIPROXY_TEST_DATABASE_URL` supplied to that shell
  (normal make test caching retained). No database was stopped or dropped.
- Remaining: the next sequential docs unit must update public CLI examples/help
  references, configuration/routing/API/operations contracts, design/AGENTS,
  and release notes for the revised direct-request behavior. Document whole-block
  precedence/defaults, reset versus inherit, D1 body/recovery bounds, D2 sparse
  overlays, D3 managed nullable-policy versus nullable-models semantics and
  read-only inherited views, and alias-null removal. Retain ER-01's endpoint,
  stream, compressed-inspection, and peer constraints without broad recovery or
  plaintext-preservation promises. `make docs-contract` and `pnpm docs:build`
  remain with that unit. CLI editing stays HCL-only; inherited overlay editing
  requires a valid, loadable base inventory, not a partial/invalid base config.
  No broad public docs, UI/admin/runtime, or release-note work was performed here.
  ER-05 remains `in_progress`; ER-06/ER-07 remain pending. Coordinator review of
  this unit and combined ER-05 acceptance remain required.

Coordinator CLI-unit review (2026-11-17): inspected generic/model flag handling,
presence and conflict validation, source-backed edits, prompt reconstruction,
inventory-based sparse overlays, and raw `extends` preservation. Independently
reran `GOFLAGS='-p=1' go test ./internal/config ./internal/configedit ./cmd/aiproxy -count=1`;
all passed. `git diff --check` passed. The documentation unit may proceed.

Third sequential unit evidence (2026-11-17; public documentation/examples):

- Changed: `README.md`, `AGENTS.md`, `docs/design.md`, `CHANGELOG.md`,
  `website/docs/configuration.md`, `website/docs/providers-and-routing.md`,
  `website/docs/api-reference.md`, `website/docs/operations.md`,
  `examples/alias-encrypted-reasoning.hcl`, new
  `examples/scoped-encrypted-reasoning.hcl` and
  `internal/config/encrypted_reasoning_docs_test.go`, plus this execution record.
  Preserved all earlier implementation/editor/CLI changes; no runtime/admin/CLI/UI
  implementation changes, subagents or commits in this unit.
- Public contracts now cover whole-block precedence/defaults, absent/reset/false,
  normalized byte/pattern bounds, direct max-two and alias shared-extra-attempt
  limits, sticky explicit-alias versus original-body-per-target behavior,
  pre-stripping, ordinary immediate 400 versus established stream handling,
  bounded compressed inspection, cancellation and fail-closed peer controls/
  budgets. Removed blanket direct-never-strips and plaintext-never-strips claims;
  documented unsigned typed reasoning/chat-field removal, continuity loss and no
  output decryption. Broadened static derivative overlays to reasoning/deferred/
  both without allowing definition/credential/chain relaxation.
- Managed docs distinguish nullable local policy from nullable model collections,
  sparse replacement overlays from full inherited GET inventory, provider-level
  effective metadata from per-model resolution, read-only source/inherited fields
  and null effective policy on undeclared aliases. Explicit alias-null removal
  is called out as an intentional API clarification. Migration/legacy inert rows,
  invalid/dangling overlay rollback, credential-only rejection, existing RBAC,
  no root admin edit, durable-save headers and instance-local activation are
  documented. Dedicated provider/model web controls are explicitly pending ER-06,
  not claimed delivered.
- CLI docs match the inspected current flag definitions and the preceding unit's
  handoff: root uses `configure upstream` without requiring a timeout; generic
  and `name=value` model controls cover preserve/inherit/reset/member edits,
  explicit empty members and conflicts. HCL-only in-place editing retains
  `env()` spelling; conversion resolves it. Sparse inherited edits require a
  valid loadable base. The previous routing anchor
  `#alias-encrypted-reasoning` remains as an explicit compatibility anchor.
- `TestDocumentedEncryptedReasoningScopedConfiguration` loads the exact full
  configuration fence and checks byte equality with the shipped example;
  `TestDocumentedEncryptedReasoningRoutingAlias` appends the exact documented
  explicit-target snippet; `TestDocumentedEncryptedReasoningAliasExample`
  verifies the corrected legacy example. All assert sparse declaration presence,
  resets/false/default lists, effective precedence/source/inherited outcomes,
  local credentials and complete inventory, plus compact/noncompact HCL/JSON
  conversion. Fixtures use synthetic credentials and loopback stub URLs, require
  no database and perform no provider calls. Initial test-only nil/empty-list and
  catalog-enumeration-order assertions were corrected; no implementation mismatch
  was found or production behavior changed.
- Verified at repo root: final
  `GOFLAGS='-p=1' go test ./internal/config ./internal/configedit ./cmd/aiproxy -count=1`,
  `make docs-contract`, `pnpm docs:build`, `gofmt` and `git diff --check` passed.
  `GOFLAGS='-p=1' make vet test` also passed with the approved disposable
  `AIPROXY_TEST_DATABASE_URL` supplied to that shell (normal make caching
  retained); no service/database was stopped or dropped. Docs contract matrices
  are unchanged/aligned; MDX and website link validation succeeded, and both
  new/compatibility reasoning anchors were checked in rendered HTML.
- Formatting: Prettier checks passed for README, AGENTS, design, routing, API and
  operations pages, and isolated new configuration/Unreleased sections. An
  initial whole-file check flagged new tables, which were corrected, plus
  historical changelog list style and an untouched configuration JSON snippet.
  Separate `HEAD` checks reproduced those latter warnings; historical content
  was not broadly reformatted.

Combined ER-05 acceptance review (documentation unit; not independent ER-07 review):

- Reconciled the editor/CLI coordinator reviews above with the current published
  flags/contracts and reran all three package suites uncached. Conversion and
  editing acceptance remains covered by the preceding units' round-trip,
  no-op/reset/remove, expression-preservation and rejected-publication tests;
  this unit adds exact public-fixture semantic checks and passing docs gates.
- ER-05 requirements/acceptance and task-specific verification are satisfied;
  status is completed. Coordinator acceptance of this final handoff still
  precedes starting the next sequential subagent. ER-06/ER-07 remain pending.
  ER-06 should update the explicit pending-web-control notes only when its
  provider/model controls are delivered and verified. Full final build/race/
  integration/artifact review gates remain ER-07 work; no live-provider or
  independent integrated-review claim is made here.

### Task ER-06: Expose Inherit and Override Behavior in Web Forms

Status: completed

Assigned owner: ER-06 alias/completion subagent (second sequential work unit)

Kind: improvement

Priority: P2 — managed users must not accidentally persist inherited defaults or lose opt-outs.

Dependencies: ER-05

Primary ownership: `web-ui/src/types.ts`, `web-ui/src/pages/providers-page.tsx`,
`web-ui/src/pages/aliases-page.tsx`, related service/input types, and mounted tests.

Coordinator ER-05 review (2026-11-17): reviewed exact documentation fixtures,
conversion/state assertions, scope/reset/recovery and API/CLI contracts.
Independently verified `go test ./internal/config -run DocumentedEncryptedReasoning -count=1`
and `make docs-contract`; both passed. The documented pending-web notes will be
replaced only after ER-06's matching controls and checks pass.

Sequential ER-06 units: shared declaration/effective web types, policy controls,
and provider/model forms with mounted tests first; aliases and cross-page intent/
metadata checks second. Review each unit before the next starts. ER-06 remains
in progress until all page regression suites, typechecking, and web build pass.

Coordinator pre-ER-06 web baseline: `pnpm --filter @aiproxy/web-ui test` passed
218 tests in 11 files; `pnpm web:typecheck` passed. This is pre-feature UI evidence,
not verification of the new forms.

Finding: provider/model UI types omit policy; aliases use an optional block with
existing round-trip tests.

References: `adminProviderSchema`, `adminModelSchema`, `adminAliasSchema`;
`web-ui/src/pages/aliases-page.test.tsx` (existing policy serialization assertions).

Requirements:

1. Implement D3 parity: add provider/model override controls and ER-01's read-only
   effective/source/inheritance context; expose static configuration read-only.
2. Clearly distinguish inherit, explicit safe reset, and configured override.
   Do not emit an inherited policy during unchanged saves or conflate false with absence.
3. Retain alias precedence and existing round trips; follow confirmed derivative
   restrictions and existing UI authorization boundaries.
4. Render inherited model definitions while submitting only existing-name reasoning
   overlays on derivatives. Preserve local declarations in complete model-array
   replacements, distinguish policy null removal from `models` null preservation,
   and require destination arrays for policy-bearing inheritance-kind transitions.

Acceptance criteria:

- Mounted tests assert actual HTTP bodies for create, unchanged save, reset,
  removal/inherit, false values, custom messages, and reopen at supported scopes.
- Inherited effective metadata never becomes a local write by accident; rejected
  edits show field errors and produce zero mutation requests.
- Existing alias/provider page regression suites remain passing.

Verification: `pnpm --filter @aiproxy/web-ui test`; `pnpm web:typecheck`;
`pnpm web:build`; shared final gates.

First sequential unit evidence and handoff (2026-11-17; ER-06 remains in progress):

- Changed `web-ui/src/types.ts`, `web-ui/src/lib/encrypted-reasoning-form.ts`
  and its tests, `web-ui/src/components/encrypted-reasoning-control.tsx`,
  `web-ui/src/pages/providers-page.tsx`, and its mounted tests. No alias-page,
  backend, CLI, credential/device-flow, or public pending-web documentation
  implementation was changed in this unit.
- Local policy schemas now preserve absent/null/`{}` and optional false/string/
  list members without defaults or catch-filling. A separate effective schema
  describes normalized read-only values. Provider/model source, winning-base
  inheritance, and model-definition inheritance metadata decode separately.
  Alias decoding shares the sparse nullable declaration/effective schemas,
  but its existing form serialization is intentionally unchanged for unit two.
- Provider and concrete-model editors expose inherit/reset/custom plus optional
  member controls, including explicit empty mismatch/list members. Unchanged
  saves omit provider policy and models; reset writes `{}`, provider removal
  writes null, and concrete replacement arrays retain each retained model's
  exact local declaration (including raw pattern spelling), never effective
  policy metadata. Read-only server context is labeled saved, not a preview
  of unsaved policy resolution. Static rows expose local/effective context
  without adding edit actions; root remains static and provider-vs-model
  precedence is explained.
- Derivative editors show authorized inherited definitions read-only and emit
  only sparse name+local-policy overlays. Inherit drops the local overlay;
  clearing all overlays emits `models: []`, not inventory deletion. New/changed
  bases require authorized inventory, unmatched local overlays remain visible
  until explicitly removed, and policy-bearing concrete/derived transitions
  require destination-array confirmation. Returning to an original base can
  reuse its authorized provider GET inventory without guessing definitions or
  copying base policy declarations.
- Shared API for the alias unit:
  `reasoningDraft(local)` preserves local presence/member intent;
  `reasoningDraftSchema` validates custom declarations;
  `reasoningDeclaration(draft)` returns null/`{}`/sparse object.
  `EncryptedReasoningControl` is controlled by `value`/`onChange`, with `label`,
  optional `view` (read-only metadata), `error`, and `disabled` props. Use a
  form Controller and compare local drafts before deciding whether to emit a
  policy key. `ReasoningPolicyContext` and `ReasoningPolicyReadOnly` expose
  saved metadata without using it to create local state. Alias inheritance
  still needs its target-dependent explanation and must not invent a single
  effective policy or source for an undeclared alias.
- Validation uses Go-compatible Unicode whitespace trimming/simple per-rune
  lowercase normalization (including dotted I/contextual sigma distinctions),
  TextEncoder UTF-8 byte lengths, at most 16 entries, and normalized duplicate
  rejection. Empty lists are intentional; blank list entries/interior lines
  are never filtered into valid patterns. Mounted provider/concrete/inherited
  model cases reject duplicates, blank lines, count and byte-bound failures
  before any HTTP mutation.
- Verified from repo root:
  `pnpm --filter @aiproxy/web-ui test` — **291 tests, 12 files passed**;
  `pnpm web:typecheck` — passed; `pnpm web:build` — passed (serialized).
  Focused helper/provider/alias suites — **144 tests, 3 files passed**,
  including existing Copilot provider and alias regressions.
  `env -u AIPROXY_TEST_DATABASE_URL GOFLAGS=-p=1 make vet test` — passed;
  database-backed tests were intentionally not enabled for this web-only
  unit, so this is not ER-07's DB-backed final-gate evidence.
  `git diff --check -- web-ui` — passed.
- Initial focused checks exposed two inheritance fixtures lacking authorized
  base rows and one new fixture's TypeScript shape; these were corrected and
  the checks rerun successfully, without default-filling production intent.
- Remaining: independent first-unit review, then fresh sequential alias form/
  mounted intent and metadata tests; only after that review update pending-web
  documentation and assess full ER-06 acceptance. ER-07 remains pending;
  no DB/race/integration final gates or independent review are claimed here.

Coordinator provider-unit review: inspected sparse nullable declaration/effective
schemas, controlled inherit/reset/custom inputs, Go-byte-bound validation,
provider/model serialization, and inherited inventory/transition safeguards.
Independently reran the full web suite (291 tests in 12 files), web typecheck,
and `git diff --check`; all passed. The second alias/completion unit may proceed.

Second sequential unit completion evidence (2026-11-17):

- Changed `web-ui/src/pages/aliases-page.tsx` and its mounted tests, with minimal
  alias-aware saved-context support and cross-scope tests in
  `web-ui/src/components/encrypted-reasoning-control.tsx` and its new test file.
  Reused the reviewed sparse draft/validation helpers without duplicating policy
  schemas or normalization. No backend, CLI or provider-form implementation changed.
- Alias forms distinguish unchanged preservation, inherit, safe reset and custom.
  Untouched policies are omitted even during routing edits; removal sends null,
  reset sends `{}`, and custom changes retain other exact local members, including
  false, explicit empty members and raw/multiline pattern spelling. Effective
  metadata is never written. Existing target/slash-segment, shorthand, algorithm,
  retry and affinity round-trip assertions remain covered.
- Mounted tests assert actual POST/PUT bodies against separate saved-view fixtures
  for create, noop, unrelated edits, reset, removal, custom edits and reopen.
  Invalid mismatch, blank/duplicate/count/UTF-8-byte patterns show field errors
  before any mutation; deliberate empty lists and byte/count boundaries pass.
  Static rows remain read-only; a forbidden list read reveals no policy context
  and adds no catalog lookup.
- Undeclared aliases show target-dependent model → provider → root → built-ins
  resolution, not a first-target/global effective value; explicit aliases show
  normalized saved alias context without invented source/inherited metadata.
  Two-model targets, saved-vs-unsaved context and activation-error retention are
  covered, along with help text for alias precedence/sticky stripping and the
  request-wide once-only recovery allowance.
  Shared component tests retain provider/model source/inheritance context through
  unsaved edits; the complete existing provider/model suite still passes.
- After both pages passed their combined UI checks, replaced pending-web notes in
  `README.md`, `AGENTS.md`, `docs/design.md`, `website/docs/configuration.md`,
  `website/docs/api-reference.md` and `website/docs/operations.md`. Documented
  available local-intent controls, read-only definitions/static rows, root remaining
  static configuration and saved context that is not activation proof. Existing
  example fences, fixture test names and anchors are unchanged.
- Verified from repo root, serialized:
  `pnpm --filter @aiproxy/web-ui test` — **333 tests, 13 files passed**;
  `pnpm web:typecheck` and `pnpm web:build` — passed.
  `make docs-contract` and `pnpm docs:build` — passed.
  Prettier checks on the four owned UI files and `git diff --check` — passed.
  With `AIPROXY_TEST_DATABASE_URL` set to the isolated ready test database and
  `GOFLAGS=-p=1`, `make vet test` — passed with DB-backed tests enabled.
- Initial focused test failures were exact-text whitespace matching and an
  expected existing identity lookup in new fixtures; corrected those assertions/
  fixtures and reran successfully. The first Go gate hit the command timeout;
  its longer-timeout rerun passed. No DB prerequisite was disabled.
- ER-06 acceptance is complete; ER-07 remains **pending** for a separate
  independent reviewer and its integrated acceptance, race and binary-level
  integration gates. Frontend request/view tests are not DB/runtime activation
  verification or live-provider evidence. Leave the isolated database ready and
  retain per-shell DB configuration plus serialized Go package execution.

### Task ER-07: Verify Integrated Scoped Policy Behavior

Status: completed

Assigned owner: ER-07 independent integration-review subagent

Kind: improvement

Priority: P2 — release gate for a cross-cutting configuration and replay change.

Dependencies: ER-02, ER-03, ER-04, ER-05, ER-06

Primary ownership: integration fixtures/tests, this task file, and minimal fixes
required by verified acceptance failures.

Coordinator ER-06 review: inspected alias controlled drafts, sparse writes,
unchanged-policy omission, target-dependent context and shared validation.
Independently reran the full web suite (333 tests in 13 files) and
`git diff --check`; both passed. All preceding task-specific gates are satisfied.
This final subagent is a fresh session, separate from feature implementers,
and must independently review behavior and add binary verification.

Finding: package coverage alone cannot establish binary configuration/reload,
managed persistence, UI intent, and runtime behavior agreement.

References: `Makefile` (`vet`, `test`, `test-race`, `integration`, `docs-contract`);
task acceptance criteria above.

Requirements:

1. Add/execute hermetic binary-level scenarios for root defaults, direct recovery,
   scoped opt-out, alias override, mixed-target behavior, and valid/invalid reload.
2. Review every acceptance criterion against evidence; reconcile implementation,
   static/managed types, docs, and examples. Review unsupported operations, RBAC,
   response ownership, attempt limits, stream retention, and peer boundaries.
3. Run the final gates below; record commands, results, and skipped prerequisites.
   Reopen affected tasks for failures rather than declaring code-written completion.
4. An independent review is desirable if separately authorized; do not claim one
   occurred when the implementer performs this review.

Acceptance criteria:

- All approved tasks and contracts have passing evidence, with no required DB
  assertions silently skipped and no accidental behavior change to existing aliases.
- Final verification and any explicitly deferred surface/decision are documented,
  including rationale, revised dependencies, and remaining limitations.

Verification: shared final gates and acceptance-evidence review.

Independent completion evidence (2026-11-17):

- ER-07 ran in a fresh review session, separate from ER-01–ER-06 implementers,
  after the coordinator's sequential handoff. No additional agents or commits
  were created. Existing implementation and unrelated work were preserved.
- Changed only `internal/integration/encrypted_reasoning_binary_test.go`,
  `internal/integration/encrypted_reasoning_fixtures_test.go`, and this task file.
  The confirmed gap was missing binary-level scoped-policy coverage, not a
  reproduced implementation defect; no production-code correction was needed.
- Added an operation-aware HTTP upstream fixture without modifying the existing
  chat-only stub. The new tests use isolated child configuration/home state,
  remove ambient inline-config/proxy settings, resolve the executable absolutely
  at runtime, and use synthetic inbound/upstream credentials only.
- All **36 leaf binary scenarios** passed: 16 direct-scope, 18 alias, and two
  valid/invalid SIGHUP scenarios across chat **and Responses**. Assertions compare
  complete upstream JSON independently of implementation stripping helpers,
  including retained text/tool history and removed signed/unsigned typed items;
  they pin operation paths, upstream model names, credentials, exact call counts,
  response bytes/fixture headers, and zero calls to unselected/rejected endpoints.
  Cases cover built-in no-recovery, root fallback/recovery, provider/model empty
  opt-outs, whole-block member defaults, explicit alias reset/strip overrides,
  per-target original versus explicit sticky bodies, pre-strip allowance,
  configured-400 failover, direct max-two and alias single-extra bounds.

Acceptance reconciliation:

| Task / boundary      | Independently inspected evidence and result                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ER-01 contract       | Shared contract and finalized D1–D3 tables reconcile with parsing, dispatch, saved-intent APIs and public docs; no unresolved decision or parity deferral. Explicit alias compatibility and intentional alias-null removal are documented.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| ER-02 configuration  | Reviewed `encrypted_reasoning.go`, syntax/JSON codecs, build/deferred/catalog/dynamic paths and resolver root cloning. Scoped/inheritance/validation/catalog/format tests and resolver snapshot tests pass: nil/reset/false/member presence, whole-block precedence, inherited model over child provider, independent declaration/slice copies, disabled validation and no flattened root defaults. Existing credential, chain, TLS and deferred restrictions remain enforced.                                                                                                                                                                                                                                                                                        |
| ER-03 inference      | Reviewed direct/alias loops, recovery gates, result ownership and execution-budget checks. New binaries plus scoped runtime/boundary/compressed/deferred tests pass: same target/context, direct cooldown isolation, once-only shared allowance, guardrail-admitted originals, bounded decoding, unchanged-rewrite rejection, immediate streaming errors, established SSE exclusion, native/unsupported operation boundaries, cancellation/peer fail-closed behavior, leases and once-only accounting. App tests retain admitted recovery/failover/stream snapshots through valid and rejected reloads.                                                                                                                                                               |
| ER-04 managed parity | Reviewed additive migration/store writes, `MaterializeProvider`, admin inventory, strict input decoding, provider/alias saves and credential/RBAC paths. DB-backed migration/round-trip/reopen/rollback, inheritance/overlay/transition and authorization assertions actually executed. Omitted policy preserves, null removes, reset remains sparse; omitted/null collections preserve and arrays replace. Forbidden zero/null keys, duplicate/case-variant keys and read-only metadata cannot bypass validators; credential-only writes retain policies and reject edits. Saved-but-failed activation and corrupt/dangling-row rollback tests pass. Reads materialize full inventory/local metadata without decrypting credentials merely to render policy context. |
| ER-05 CLI/docs       | Reviewed shared raw-block editing, CLI presence-aware flags and derived inventory handling, conversion block registration and public policy/API/routing docs. Full tests include raw expression/comment preservation, no-op/reset/remove, unrelated edits, invalid-write preflight, HCL/JSON sparse round trips and exact documentation fixtures. Docs describe typed unsigned stripping, bounded conditional recovery and no output decryption/universal plaintext preservation.                                                                                                                                                                                                                                                                                     |
| ER-06 mounted UI     | Reviewed declaration/effective schemas, shared draft/control, provider sparse model serialization and alias omission handling. Mounted provider/alias tests assert actual mutation bodies, reset/removal/reopen, explicit false/empty members, inherited overlays, transition confirmation, field-error zero-write behavior and static/RBAC restrictions. Saved context remains read-only and target-dependent for undeclared aliases; mounted mocks do not establish server activation. All 333 web tests pass.                                                                                                                                                                                                                                                      |
| ER-07 integration    | New binary scenarios and every required final gate below pass. No scoped implementation defect remained reproducible; no required database assertion was skipped.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |

Verification run from repo root on the final implementation/UI state, with
`AIPROXY_TEST_DATABASE_URL` set to the coordinator-owned isolated disposable
PostgreSQL 16.15 database for Go gates. Builds and package/DB fixtures were run
serially; `GOFLAGS='-p=1'` changes scheduling only, not test selection.
`$PWD` below means the repo checkout, used because the binary helper requires an
absolute executable path.

| Command                                                                                                                                                  | Result                                                                                                                    |
| -------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `GOFLAGS='-p=1' make build`                                                                                                                              | Passed; rebuilt the embedded UI and host binary before Go final gates.                                                    |
| `GOFLAGS='-p=1' AIPROXY_BINARY="$PWD/dist/aiproxy" go test -tags=integration ./internal/integration -run '^TestBinaryEncryptedReasoning' -count=1 -v`    | Passed: three top-level tests, 36 leaf scenarios, no skips.                                                               |
| `GOFLAGS='-p=1' AIPROXY_BINARY="$PWD/dist/aiproxy" go test -race -tags=integration ./internal/integration -run '^TestBinaryEncryptedReasoning' -count=1` | Passed: new binary tests and fixture synchronization under the race detector.                                             |
| `GOFLAGS='-p=1' go test ./internal/store ./internal/dbmerge ./internal/httpapi ./internal/app -run EncryptedReasoning -count=1 -v`                       | Passed: 56 top-level tests, 608 passing nodes including parents/subtests, zero skips.                                     |
| `GOFLAGS='-p=1' make vet test`                                                                                                                           | Passed; all packages, required DB tests enabled.                                                                          |
| `GOFLAGS='-p=1' make test-race`                                                                                                                          | Passed; complete race suite, required DB tests enabled; no tests/assertions removed.                                      |
| `GOFLAGS='-p=1' make integration`                                                                                                                        | Passed; complete hermetic binary suite; rebuilt embedded UI through `pnpm web:build` and `make build`.                    |
| `GOFLAGS='-p=1' go test ./... -count=1 -json`                                                                                                            | Passed uncached: 30 tested packages, 2,017 top-level tests / 9,187 passing nodes including parents/subtests; no DB skips. |
| `make docs-contract`                                                                                                                                     | Passed.                                                                                                                   |
| `pnpm --filter @aiproxy/web-ui test`                                                                                                                     | Passed: 333 tests / 13 files.                                                                                             |
| `pnpm web:typecheck`                                                                                                                                     | Passed.                                                                                                                   |
| `pnpm docs:build`                                                                                                                                        | Passed; static documentation generated. Non-blocking Docusaurus update and Node localStorage notices only.                |
| `make check-toolchain`                                                                                                                                   | Passed.                                                                                                                   |
| `git diff --check`                                                                                                                                       | Passed.                                                                                                                   |

Limitations and readiness:

- The uncached full Go run reports three unrelated expected skips:
  `TestNotBuiltWithoutIndex` applies only before an embedded UI exists;
  `TestRegularReadPTYChild` is a fresh-session subprocess helper;
  `TestInstalledBunCaptureCompareAndVerifiedTLS` requires the optional
  `TLS_CLIENTHELLO_TEST_BUN` real-runtime opt-in. No required ER test or database
  assertion skipped; these do not disable a final-gate prerequisite.
- Binary evidence is Linux/local-fixture evidence, not live-provider,
  multi-instance broadcast, or cross-platform compatibility certification.
  Existing provider capability/translation limits, static root editing,
  instance-local activation and process-local peer state remain intentional.
- All seven tasks meet their acceptance criteria and required verification.
  No open D1–D3 decision, required-check blocker or additional parity deferral
  remains. Live-provider compatibility was not tested or claimed.

Coordinator final acceptance (2026-11-17): reviewed the independent binary
fixtures/scenarios and completion record, independently reran
`GOFLAGS='-p=1' AIPROXY_BINARY="$PWD/dist/aiproxy" go test -tags=integration ./internal/integration -run '^TestBinaryEncryptedReasoning' -count=1`
(passed), and verified all seven statuses, required fields, local links and
private-metadata hygiene. The coordinator-owned disposable PostgreSQL instance
was stopped after all checks; no production database or unrelated process was
used. Changes remain uncommitted.

## Shared Verification and Definition of Done

All commands run from the repository root with its declared Go, Node, and pnpm
toolchains and installed workspace dependencies. Tests use synthetic credentials
and local mock upstreams, never real provider calls or production databases.

Final gates:

```sh
GOFLAGS='-p=1' make vet test
GOFLAGS='-p=1' make test-race
GOFLAGS='-p=1' make integration
make docs-contract
pnpm --filter @aiproxy/web-ui test
pnpm web:typecheck
pnpm docs:build
```

Set `AIPROXY_TEST_DATABASE_URL` to the isolated disposable database for Go gates.
`GOFLAGS='-p=1'` serializes package execution because ER-04 observed cross-package
shared-schema fixture interference in a default parallel run. It changes
scheduling only: retain every test and assertion, including all database tests
and the full race suite. Do not misreport that failed parallel run as passing.

`make integration` builds the binary and embedded web UI through `make build`.
Serialize it with `pnpm web:build` and other builds writing
`internal/webui/dist`; documentation builds are not embedded-UI builds.
Run ER-04's DB-backed checks with the required disposable database and verify
they actually execute. ER-07's completion evidence above records the complete
final gate run with database assertions enabled. The planning-only
`make docs-contract` result was superseded by that final rerun.

Done means ER-01 decisions are recorded, each required task meets its acceptance
criteria and verification, documented examples match runtime precedence/retry
bounds, old alias-only configurations remain compatible, direct policies work,
and no required check is unavailable or skipped. Keep task statuses and concise
completion evidence current; this planning file alone completes no implementation task.
