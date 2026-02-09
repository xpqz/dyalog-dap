# Live E2E Flake Classification Policy (Issue #54)

Context reference: `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md`

## Goal

Provide deterministic triage labels for failures in live interactive adapter E2E tests.

## Classifications

- `infrastructure-flake`
  - Symptoms: timeout, broken pipe, connection reset, stream closed
  - Action: retry once, inspect network/process startup and transcript artifacts

- `scenario-precondition`
  - Symptoms: missing stopped/thread/frame/scope prerequisites
  - Action: verify live Dyalog launch scenario/environment configuration (`DYALOG_E2E_REQUIRE`, startup command)

- `product-defect`
  - Symptoms: deterministic protocol/behavior mismatch not explained by infra/precondition classes
  - Action: treat as regression and file bug with artifact links

## Artifact Requirements

Live E2E tests must emit artifact paths in failure messages and write JSONL DAP traces under:

- `artifacts/integration/live-e2e/`

CI uploads `artifacts/integration/` on both pass/fail in live jobs for diagnosis.
