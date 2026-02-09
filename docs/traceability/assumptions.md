# Implementation Assumptions and Plan Traceability

This document maps implemented behavior to assumptions made during delivery, with direct traceability back to `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md` and verification evidence in code/tests.

## Plan References

Primary plan anchors used for implementation and validation:

- Section 1: wire protocol framing and payload structure
- Section 2: startup handshake and `GetWindowLayout`
- Section 3: prompt-state command gating model
- Section 4: debugger model, line attributes, and save/close ordering
- Section 5: threads and stack modeling
- Section 7: undocumented/incomplete command handling
- Section 8: phase-1 command subset
- Section 9: DAP mapping strategy
- Section 10: multi-layer testing strategy
- Sections 11/12: Go architecture and project conclusions

## Assumptions

- RIDE command and argument shapes are treated as source-of-truth from observed behavior in reference implementations where protocol docs are incomplete.
- Prompt type `0` is the only strict busy state for outbound gating; non-zero prompt types are treated as send-allowed.
- Source references are path-stable within a session and can be reused across window close/reopen cycles.
- Fatal runtime signals (`Disconnect`, `SysError`, `InternalError`) should terminate adapter session state unless explicit reconnect flow is invoked.
- Live Dyalog endpoint availability is environment-dependent; fake-server integration tests are required for deterministic CI.

## Feature Traceability Matrix

| Feature | Plan Section(s) | Key Assumption(s) | Implementation Evidence | Verification Evidence |
|---|---|---|---|---|
| RIDE framing + startup handshake | 1, 2 | Protocol v2 framing + startup command order is stable | `/Users/stefan/work/lsp-dap/internal/ride/transport/client.go` | `/Users/stefan/work/lsp-dap/internal/ride/transport/client_test.go`, `/Users/stefan/work/lsp-dap/internal/integration/harness/harness_test.go` |
| Prompt-aware outbound queue + allow-list/cancel | 3 | Only prompt type `0` blocks non-allow-listed sends | `/Users/stefan/work/lsp-dap/internal/ride/sessionstate/dispatcher.go` | `/Users/stefan/work/lsp-dap/internal/ride/sessionstate/dispatcher_test.go` |
| DAP control mapping + pause fallback chain | 9 | Weak interrupt may fail and should cascade to strong/fallback | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server.go` | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server_test.go` |
| Threads + stack frame mapping | 5, 9 | Tracer windows are primary stack-frame source in phase 1 | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server.go` | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server_test.go`, `/Users/stefan/work/lsp-dap/internal/dap/adapter/protocol_flow_test.go` |
| Breakpoints (mapped + deferred) | 4, 9 | Breakpoints may be set before source windows exist; apply on later mapping | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server.go` | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server_test.go`, `/Users/stefan/work/lsp-dap/internal/integration/harness/harness_test.go` |
| Save/Reply/Close ordering enforcement | 4 | `CloseWindow` must be held until matching `ReplySaveChanges` | `/Users/stefan/work/lsp-dap/internal/ride/sessionstate/dispatcher.go` | `/Users/stefan/work/lsp-dap/internal/ride/sessionstate/dispatcher_test.go`, `/Users/stefan/work/lsp-dap/internal/integration/harness/harness_test.go` |
| Runtime error translation + reconnect hook | 2, 7 | Fatal RIDE diagnostics terminate session; reconnect is explicit lifecycle event | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server.go` | `/Users/stefan/work/lsp-dap/internal/dap/adapter/server_test.go` |
| Undocumented command subset decoding | 7 | `GetWindowLayout`, `SetSIStack`, `ExitMultilineInput`, `SetSessionLineGroup` are real-world protocol surface | `/Users/stefan/work/lsp-dap/internal/ride/protocol/codec.go` | `/Users/stefan/work/lsp-dap/internal/ride/protocol/codec_test.go`, `/Users/stefan/work/lsp-dap/docs/validations/21-undocumented-commands.md` |
| Integration harness + transcript artifacts | 10 | Deterministic protocol verification requires reusable harness + logs | `/Users/stefan/work/lsp-dap/internal/integration/harness/harness.go` | `/Users/stefan/work/lsp-dap/internal/integration/harness/harness_test.go` |
| CI gating and optional live path | 10 | Live endpoint may be unavailable in CI and must be optional | `/Users/stefan/work/lsp-dap/.github/workflows/ci.yml` | `/Users/stefan/work/lsp-dap/scaffold_test.go` |

## Validated Deviations

- Language/runtime deviation: original early direction referenced TypeScript for DAP, but implementation is Go by explicit project decision. Validation: complete Go package structure and tests under `internal/`.
- Live matrix deviation: many integration assertions currently use fake RIDE server flows for deterministic coverage; live endpoint execution is optional (`live-dyalog` CI job conditioned on `DYALOG_RIDE_ADDR`).
- Reconnect orchestration deviation: reconnect support is currently hook-based in adapter (`HandleRideReconnect`) rather than a fully automated transport supervisor loop.
- Prompt-mode ambiguity deviation: prompt type edge behavior for some modes remains explicitly marked as known limitation/skip pending live multi-version verification.
