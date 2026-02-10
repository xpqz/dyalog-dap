# Refactor Plan: Organisation, Duplication, and File Size

## Purpose

This document captures maintainability findings from a structure-focused review of the current codebase and proposes clear, incremental refactorings.

The goal is to improve:
- clarity of ownership between modules
- separation of concerns
- test readability and maintainability
- confidence in future changes (especially in debugger/runtime behavior)

## Scope of Review

Primary files reviewed:
- `internal/dap/adapter/server.go`
- `internal/dap/adapter/server_test.go`
- `cmd/dap-adapter/main.go`
- `cmd/dap-adapter/main_test.go`
- `scaffold_test.go`
- `vscode-extension/src/extension.ts`
- `vscode-extension/src/adapterInstaller.ts`
- `vscode-extension/src/test/extensionBundleCommand.integration.test.ts`
- `internal/ride/sessionstate/dispatcher.go`
- `internal/ride/protocol/codec.go`

Largest files currently:
- `internal/dap/adapter/server_test.go` (~2524 lines)
- `internal/dap/adapter/server.go` (~2482 lines)
- `scaffold_test.go` (~904 lines)
- `cmd/dap-adapter/main_test.go` (~734 lines)
- `internal/ride/sessionstate/dispatcher_test.go` (~684 lines)
- `cmd/dap-adapter/main.go` (~464 lines)
- `vscode-extension/src/extension.ts` (~442 lines)

## Findings (Prioritized)

### 1. Adapter server is a "god object" and too large

Files:
- `internal/dap/adapter/server.go`

Symptoms:
- one file handles DAP routing, lifecycle state, source mapping, breakpoints, thread/stack modeling, locals/evaluate, RIDE event handling, payload extraction, formatting utilities
- one `Server` struct owns many maps/counters/flags, increasing coupling and cognitive load

Risks:
- high regression risk when changing unrelated behavior
- lock and state invariants are hard to reason about
- harder to onboard contributors

Recommendation:
- split by concern into focused files/components:
  - `server_requests.go` (DAP request dispatch + lifecycle checks)
  - `server_ride_events.go` (RIDE inbound event handling)
  - `server_breakpoints.go`
  - `server_threads_stack.go`
  - `server_scopes_variables.go`
  - `server_evaluate.go`
  - `server_sources.go`
  - `server_decode.go` (temporary, then reduce via shared decoder)

### 2. Locking and I/O boundaries are mixed

Files:
- `internal/dap/adapter/server.go`

Symptoms:
- state lock (`mu`) used across broad handlers
- outbound RIDE sends are intertwined with state logic

Risks:
- deadlock and latency hazards become harder to detect
- difficult to reason about progress guarantees

Recommendation:
- enforce a pattern:
  - mutate/read state under lock
  - construct command intents under lock
  - perform outbound I/O after releasing lock
- add small helper types for outbound intents to make this explicit

### 3. Duplicate decode helpers exist across layers

Files:
- `internal/dap/adapter/server.go`
- `internal/ride/protocol/codec.go`
- `cmd/dap-adapter/main.go` (separate `getString/getInt`)

Symptoms:
- repeated conversion helpers (`int/string/bool/slice from any`)
- repeated parsing logic for request arguments

Risks:
- subtle behavioral drift across components
- duplicated test burden

Recommendation:
- centralize conversion/parsing utilities:
  - either route more through `internal/ride/protocol/codec.go`
  - or create shared internal typed decode helper package (e.g. `internal/support/decode`)
- make adapter consume typed event/argument structures as early as possible

### 4. VS Code extension entrypoint has too many concerns

Files:
- `vscode-extension/src/extension.ts`

Symptoms:
- activation wiring, command registration, debug descriptor factory, diagnostics logging/history, bundle generation, install orchestration all in one file
- repeated command structure (`log -> validate -> show message`)

Risks:
- slower iteration when adding/changing commands
- brittle tests for unrelated command changes

Recommendation:
- split into:
  - `src/activation.ts` (wire-up only)
  - `src/commands/*.ts` (one module per command or command group)
  - `src/debug/descriptorFactory.ts`
  - `src/diagnostics/logger.ts`
- introduce a minimal command wrapper utility for consistent error/log/message handling

### 5. Scaffold tests are large and repetitive

Files:
- `scaffold_test.go`

Symptoms:
- many tests repeat: read file, parse JSON, assert snippets
- broad coverage mixed into one file

Risks:
- hard to maintain
- noisy diffs for small changes

Recommendation:
- split by area:
  - `scaffold_ci_test.go`
  - `scaffold_extension_test.go`
  - `scaffold_release_test.go`
  - `scaffold_support_test.go`
- add helpers:
  - `mustRead(t, path)`
  - `requireSnippets(t, text, snippets...)`
  - `mustUnmarshalJSON[T](t, path)`

### 6. Runtime entrypoint combines transport, config parsing, and lifecycle

Files:
- `cmd/dap-adapter/main.go`
- `cmd/dap-adapter/main_test.go`

Symptoms:
- DAP framing, runtime boot/shutdown, launch/attach config parsing, and orchestration all in `main.go`
- tests include extensive protocol helpers that could be shared

Recommendation:
- extract:
  - `internal/dap/transport` (frame read/write)
  - `internal/runtime` (rideRuntime lifecycle)
  - `internal/runtime/config` (`runtimeConfigFrom`)
- keep `cmd/dap-adapter/main.go` as composition root only

### 7. Integration test stubbing is heavy in one TS test file

Files:
- `vscode-extension/src/test/extensionBundleCommand.integration.test.ts`

Symptoms:
- large inline VS Code stub and module loader override logic

Recommendation:
- extract common test harness/stubs:
  - `vscode-extension/src/test/helpers/vscodeStub.ts`
  - `vscode-extension/src/test/helpers/moduleLoadPatch.ts`

## Clear Refactoring Plan (Execution Order)

### Phase 1: Safe structural split (no behavioral change intended)

1. Split `internal/dap/adapter/server.go` by concern into multiple files in same package.
2. Split `scaffold_test.go` into themed test files.
3. Split `vscode-extension/src/extension.ts` into activation + command modules.

Success criteria:
- tests remain green
- no user-visible behavior changes
- reduced average file size and clearer ownership

### Phase 2: Reduce duplication and tighten boundaries

1. Consolidate decode helpers to one shared location.
2. Standardize adapter request/event argument parsing on typed forms.
3. Consolidate repeated test helpers in Go and TS.

Success criteria:
- fewer repeated helper implementations
- simpler parser tests with shared fixtures

### Phase 3: Concurrency and runtime clarity

1. Introduce explicit outbound command intent pattern in adapter server.
2. Ensure no external I/O is done while holding adapter state lock.
3. Extract runtime/config/transport from `cmd/dap-adapter/main.go`.

Success criteria:
- clearer lock invariants
- easier reasoning about lifecycle and shutdown behavior

## Recommended First Issues

1. Split `internal/dap/adapter/server.go` into request/event/breakpoint/source/locals modules without changing behavior.
2. Extract scaffold test helpers and split `scaffold_test.go` by domain.
3. Refactor `vscode-extension/src/extension.ts` into activation + commands + diagnostics modules.
4. Create shared decode utility and migrate adapter decode helpers to it.
5. Extract `runtimeConfigFrom` and DAP framing from `cmd/dap-adapter/main.go`.

## Guardrails During Refactor

- preserve existing behavior first, then clean up
- keep each PR narrow and traceable
- maintain current test coverage; add tests before moving concurrency-sensitive code
- avoid changing protocol semantics in pure-structure PRs

## Done Definition for This Refactor Track

- no single production source file exceeds ~600 lines (except justified protocol tables/tests)
- adapter request handling and RIDE event handling are in separate modules
- extension activation file only wires components
- scaffold tests use shared helpers and are domain-split
- duplicate decode utilities are removed or intentionally centralized
