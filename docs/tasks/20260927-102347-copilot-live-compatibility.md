# Deferred GitHub Copilot Live Compatibility Verification

Created: 2026-09-27 10:23:47 local time

## Objective And Decision

The user approved completing Copilot implementation verification with mocks rather
than supplying a real OAuth client/account. The original implementation plan,
`20260907-022659-github-copilot-device-flow.md`, now closes hermetic behavior.
This separate task retains the real-provider questions that simulated services
cannot answer. Deferral is explicit, not evidence of upstream compatibility.

No live GitHub calls have been performed for this decision. Existing hermetic tests
are evidence of implementation behavior only. No credentials or production account
details are required to leave this task deferred. No CHANGELOG edits or commits.

### Task COPILOT-LIVE-01: Establish Real Client And Account Compatibility

Status: deferred

Kind: investigation

Priority: P2 — required before claiming verified live Copilot compatibility; not a
blocker to the separately accepted hermetic implementation deliverable.

Suggested agent: maintainer-authorized live integration reviewer

Owner: maintainer supplies authorization/application/account; reviewer records results.

Dependencies: original plan COPILOT-05 and COPILOT-06 for a tested implementation.

Primary ownership: this record; relevant Copilot docs and narrowly scoped adapter
corrections only if an authorized experiment establishes a contract mismatch.

Finding: local fixtures cannot establish whether a particular OAuth application
and entitled account can send a device-flow result directly to the current Copilot
inference endpoint. Current token exchange requirements, required headers, available
chat models and token lifetime may differ from the simulated contract.

References: original plan COPILOT-01 historical investigation/unknowns and
COPILOT-05 historical live gate; `internal/copilotlogin`,
`internal/provider/githubcopilot.go` (`doGitHubCopilot`), CLI login/model listing commands,
`examples/github-copilot.hcl`.

Deferral rationale: user selected mock-only verification with no real public client
ID/account. Keep deferred until an authorized owner explicitly elects live validation.
Residual risk: hermetic tests can pass while GitHub rejects direct authentication,
changes mandatory headers/models, or requires an exchange/refresh not implemented.

Requirements to resume:

1. Obtain explicit permission for a small set of live calls, a public OAuth client ID
   controlled by the maintainer with device flow enabled, and an account with suitable
   Copilot entitlement/policy permissions. Successful device login alone is not proof
   of direct inference access. Never reuse a foreign application's identity.
2. Have the owner complete browser authorization locally. Keep resulting tokens and
   device codes out of task evidence. Use isolated config/credential names and a
   loopback proxy listener; no production credential replacement.
3. Exercise device login, upstream `models --upstream`, an actually available chat
   model in JSON/SSE, and deliberate test-credential authentication failure followed
   by re-login plus explicit reload/restart. Record path/header/usage/lifetime facts
   actually observed, not inferred from fixture success.
4. If a required exchange/header/entitlement mismatch appears, document the result
   and scope an implementation change or continued deferral. Do not relabel failed
   or unavailable live testing as passed. Enterprise/other operations remain separate.

Acceptance criteria:

- Sanitized evidence identifies date, client/account eligibility conditions, selected
  model, actual successful login/listing/JSON/SSE and recovery outcomes, plus remaining
  unknowns. No tokens/device codes/account-private data appear in evidence.
- Any required implementation correction has local regression coverage and relevant
  repository gates. Docs describe exactly the verified scope.
- Until these criteria pass, this task stays deferred or becomes in_progress/blocked
  with explicit owner/prerequisite; it must not be marked completed by mock results.

Verification when resumed: authorized live smoke procedure above plus original
plan's local gates for any code change. This task has **not run**; there is no live
completion evidence. Current disposition: deliberately deferred by user decision.
