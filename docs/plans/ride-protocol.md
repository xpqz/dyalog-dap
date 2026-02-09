# RIDE Protocol Analysis for a DAP Adapter

## Goal

This document captures a deep analysis of the two reference implementations:

- `reference/ride` (official JS/Electron RIDE client, full-featured)
- `reference/gritt` (modern partial Go client)

The purpose is to identify exactly what is needed to implement a DAP server for VS Code, and what can be ignored.

## Sources Reviewed

Primary protocol and behavior sources:

- `reference/ride/docs/protocol.md`
- `reference/ride/src/cn.js` (transport, framing, connect/send gating)
- `reference/ride/src/ide.js` (message handlers, runtime behavior)
- `reference/ride/src/se.js` (session/prompt handling)
- `reference/ride/src/ed.js` and `reference/ride/src/dbg.js` (debugger/editor commands)
- `reference/gritt/ride/protocol.go` and `reference/gritt/ride/client.go` (wire implementation)
- `reference/gritt/tui.go`, `reference/gritt/editor.go` (practical protocol usage)
- `reference/gritt/adnotata/*.md` (known protocol quirks and failures)
- `reference/gritt/tui_test.go` (live behavior expectations via tmux integration tests)

## 1. Wire Protocol Fundamentals

RIDE transport framing is stable and straightforward:

- 4-byte big-endian length
- 4-byte ASCII magic `"RIDE"`
- UTF-8 payload
- length includes `"RIDE"` + payload (not the length field itself)

Payload forms:

- Handshake strings (non-JSON):
  - `SupportedProtocols=2`
  - `UsingProtocol=2`
- Normal command payload:
  - `["CommandName",{...args...}]`

Important practical details:

- Messages are asynchronous and can arrive out of request order.
- Some requests may not receive replies.
- Receiver should tolerate JSON booleans and numeric `0/1` booleans.
- Real implementations treat many flags as numeric (`debugger`, `readOnly`, etc.).

## 2. Handshake and Session Startup (Observed Behavior)

The spec text and real behavior differ slightly.

Observed startup sequence in the references:

1. TCP connect to interpreter (typically `RIDE_INIT=SERVE` endpoint).
2. Handshake exchange for protocol version 2.
3. Client sends:
   - `Identify` with `{"apiVersion":1,"identity":1}`
   - `Connect` with `{"remoteId":2}`
   - `GetWindowLayout` (undocumented in protocol.md, but used in practice)

Notes:

- `reference/gritt/ride/client.go` expects Dyalog to send `SupportedProtocols=2` first in SERVE mode.
- `reference/ride/src/cn.js` sends its startup messages immediately after socket connect.
- Both approaches work in practice.
- `GetWindowLayout` is essential for restoring open editors/tracers after reconnect, despite not being in `protocol.md`.

## 3. Execution and Prompt-State Model

Prompt state is central for correctness.

The interpreter sends `SetPromptType` to describe input readiness/mode:

- `0`: not ready / busy
- `1`: normal input prompt
- `2`: quad input (`⎕`)
- `3`: multiline editor mode
- `4`: quote-quad input (`⍞`)
- `5`: catch-all unknown mode

Execution behavior:

- Client sends `Execute {"text":"...\\n","trace":N}`.
- Typical flow while running:
  - echo as `AppendSessionOutput` (often type 14)
  - `SetPromptType {"type":0}` while busy
  - output messages
  - `SetPromptType {"type":1}` when ready

Important practical behavior:

- `reference/ride/src/cn.js` blocks most outgoing commands while prompt type is `0` (busy), except a small allow-list (interrupts, replies, Save/Close, etc.).
- `reference/ride/src/ide.js` queues multi-line execute requests and clears that queue on `HadError`.
- `reference/gritt` similarly treats `SetPromptType > 0` as completion boundary for command execution.

Implication for DAP:

- Implement a prompt-aware command scheduler.
- Do not assume immediate request-response.
- Track interpreter readiness independently of DAP request lifecycle.

## 4. Debugger Model in RIDE

The debugger model is window/token based, not frame-object RPC based.

Core concepts:

- `OpenWindow` / `UpdateWindow` represent editor or tracer windows.
- `debugger` flag distinguishes tracer windows (`1`) from plain editors (`0`).
- `token` is the stable identifier for a window; later commands use `win`/`token`.
- Breakpoints/trace/monitor are line arrays on that window (`stop`, `trace`, `monitor`), 0-based line indices.

Stepping/continue commands (all target a tracer window `win`):

- `StepInto`
- `RunCurrentLine` (step over)
- `ContinueTrace` (step out)
- `Continue` (continue thread)
- `TraceBackward` / `TraceForward`
- `RestartThreads` (resume all threads)

Highlight/current execution:

- Interpreter pushes `SetHighlightLine` with 0-based coordinates.
- RIDE converts to editor coordinates (1-based for Monaco).

Breakpoint updates:

- Immediate in tracer via `SetLineAttributes`.
- Persisted on save via `SaveChanges`.

Critical ordering rule (validated in gritt notes/tests):

- Do not send `CloseWindow` before `ReplySaveChanges` if a save is pending.
- Correct sequence: `SaveChanges -> ReplySaveChanges -> CloseWindow`.

## 5. Threads and Stack

Thread data:

- `GetThreads` / `ReplyGetThreads`
- `SetThread` / `ReplySetThread`
- Optional attribute APIs: `GetThreadAttributes`, `SetThreadAttributes`, `PauseAllThreads`

Stack data:

- `GetSIStack` / `ReplyGetSIStack` provide textual stack descriptions.
- Official RIDE also sends `SetSIStack` from UI when selecting a stack entry (`reference/ride/src/dbg.js`), but this command is not documented in `protocol.md`.

Practical debugger-frame model:

- On stop/error, interpreter often emits one or more tracer `OpenWindow` messages (one per frame in nested cases).
- `reference/gritt` builds its stack pane from tracer window tokens, not purely from `ReplyGetSIStack`.

Implication for DAP:

- Use tracer window tokens as the primary stack-frame identifiers in phase 1.
- Treat `GetSIStack` as supplemental metadata.
- Keep `SetSIStack` as a compatibility candidate; verify against live Dyalog before relying on it.

## 6. Asynchrony and Correlation Patterns

Protocol includes both correlated and uncorrelated flows:

- Correlated by token:
  - `GetAutocomplete` / `ReplyGetAutocomplete` (`token`)
  - `GetValueTip` / `ValueTip` (`token`)
- Partially correlated:
  - `SaveChanges` / `ReplySaveChanges` (`win`)
- Pure event stream:
  - `OpenWindow`, `UpdateWindow`, `SetPromptType`, `SetHighlightLine`, `AppendSessionOutput`

Design requirement:

- Single reader loop, event dispatcher, and explicit in-memory state machine.
- Never block receive loop while waiting for a specific reply.

## 7. Undocumented or Incomplete Areas

The references expose several gaps in `protocol.md`:

- `GetWindowLayout` (used by both RIDE and gritt)
- `SetSIStack` (used by RIDE debug UI)
- `ExitMultilineInput` (used in session multiline mode)
- `SetSessionLineGroup` (handled by RIDE)
- Some prompt-mode semantics are flagged as TODO in docs.

Also:

- `protocol.md` has internal red-note markers and known ambiguities.
- Some wording is legacy (process manager concepts not central to direct interpreter attach).
- Real clients rely on implementation behavior, not docs alone.

## 8. DAP-Relevant Command Subset (Phase 1)

Recommended minimal subset for first usable DAP adapter:

- Connection/session:
  - handshake (`SupportedProtocols=2`, `UsingProtocol=2`)
  - `Identify`, `Connect`, `GetWindowLayout`
- Runtime output/state:
  - `AppendSessionOutput`, `SetPromptType`, `HadError`, `SysError`, `Disconnect`
- Debug lifecycle:
  - `OpenWindow`, `UpdateWindow`, `CloseWindow`, `WindowTypeChanged`, `SetHighlightLine`
- Breakpoints:
  - `SetLineAttributes`, `SaveChanges`, `ReplySaveChanges`
- Control flow:
  - `StepInto`, `RunCurrentLine`, `ContinueTrace`, `Continue`, `TraceBackward`, `TraceForward`
- Threads/stack:
  - `GetThreads`, `ReplyGetThreads`, `SetThread`
  - `GetSIStack`, `ReplyGetSIStack` (plus optional `SetSIStack` after validation)
- Interrupts:
  - `WeakInterrupt`, `StrongInterrupt`

Everything else can be deferred.

## 9. Mapping to DAP (Initial Strategy)

High-level mapping:

- DAP `continue` -> `Continue` on active tracer window/thread
- DAP `next` -> `RunCurrentLine`
- DAP `stepIn` -> `StepInto`
- DAP `stepOut` -> `ContinueTrace`
- DAP `pause` -> `WeakInterrupt` (with `StrongInterrupt` fallback path)
- DAP `threads` -> `GetThreads`/cached `ReplyGetThreads`
- DAP `stackTrace` -> tracer window tokens (+ optional SI descriptions)
- DAP `setBreakpoints` -> translate file->window token and send `SetLineAttributes` (and persist through `SaveChanges` path when needed)

Risk:

- Breakpoints are window-token-centric, while DAP breakpoints are file-centric.
- We need a source↔window registry and on-demand open/layout synchronization.

## 10. Testing Strategy (Must-Have)

Both references confirm the same lesson: protocol correctness is discovered against live Dyalog, not docs.

Test layers for this project:

1. Pure unit tests
   - frame encode/decode
   - command parser/serializer
   - state-machine transitions
2. Scripted integration tests with live Dyalog
   - handshake/startup
   - execute lifecycle (`SetPromptType` transitions)
   - tracer open/step/close
   - save-close ordering
3. DAP integration tests
   - VS Code debug adapter test harness
   - end-to-end attach, break, step, continue, inspect

Also required:

- protocol log capture for each integration run (incoming/outgoing with timestamps)
- deterministic assertions on ordering-sensitive flows

## 11. Go Implementation Requirements

For this repository, the adapter should be implemented in Go with strict package boundaries:

- Go 1.22+ (or current stable toolchain for the project)
- Clean separation:
  - `ride/transport` (framing + socket I/O)
  - `ride/protocol` (typed command schemas)
  - `ride/session-state` (event-driven model)
  - `dap/adapter` (DAP request/event handling)
- Typed message models for known commands and tolerant handling for unknown commands.
- A single receive loop and non-blocking dispatcher architecture.

Recommended tooling baseline:

- Go modules (`go.mod`)
- `go test` for unit/integration tests
- `golangci-lint` (or equivalent static analysis/linting)
- `gofmt`/`goimports` for formatting and imports
- A Go DAP adapter approach (existing library or direct DAP protocol implementation)

## 12. Key Conclusions

1. The official `protocol.md` is necessary but not sufficient; implementation behavior is the real source of truth.
2. Prompt-state handling (`SetPromptType`) is foundational to correctness.
3. Debugging is window/token/event driven; DAP mapping must be stateful, not RPC-style.
4. Some required commands are undocumented (`GetWindowLayout`, likely `SetSIStack`), so live verification is mandatory.
5. A Go adapter is feasible now, but only with strong integration tests against real Dyalog from the beginning.

## Open Questions to Validate Next

- Exact semantics and reliability of `SetSIStack` across Dyalog versions.
- Best file-centric breakpoint strategy when corresponding editor window is not yet open.
- Robust handling of quote-quad (`⍞`) style input modes if DAP evaluate/repl support is expanded.
- Whether `WeakInterrupt` alone is sufficient for DAP pause semantics in all run states.
