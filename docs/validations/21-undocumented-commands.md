# Validation: Undocumented RIDE Commands (Issue #21)

Issue: #21  
Context plan: /Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md (sections 7 and 12)

## Scope

Commands targeted by this issue:

- `GetWindowLayout`
- `SetSIStack`
- `ExitMultilineInput`
- `SetSessionLineGroup`

## Evidence From Reference Implementations

Reference evidence captured from repository sources:

- `GetWindowLayout`
  - /Users/stefan/work/lsp-dap/reference/ride/src/cn.js
  - /Users/stefan/work/lsp-dap/reference/gritt/tui.go
- `SetSIStack`
  - /Users/stefan/work/lsp-dap/reference/ride/src/dbg.js
- `ExitMultilineInput`
  - /Users/stefan/work/lsp-dap/reference/ride/src/se.js
- `SetSessionLineGroup`
  - /Users/stefan/work/lsp-dap/reference/ride/src/ide.js

## In-Tree Validation Added

Protocol codec coverage added in this repository:

- command recognition in known command set
- typed decoders for:
  - `SetSIStackArgs`
  - `ExitMultilineInputArgs`
  - `SetSessionLineGroupArgs`
- decode tests in:
  - /Users/stefan/work/lsp-dap/internal/ride/protocol/codec_test.go

Validation test:

- `TestDecodePayload_UndocumentedCommandSubset_DecodesTypedArgs`

## Result

- these commands are now explicitly recognized and decoded by the protocol layer
- unknown-command fallback remains in place for future protocol drift

## Remaining Gap

Cross-version live interpreter verification is not available in this task context. Live matrix validation across target Dyalog versions remains to be completed by integration harness work under testing track issues.
