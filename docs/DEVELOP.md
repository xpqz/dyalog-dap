# Development Guide

This document is for developers working on `dyalog-dap`.
For end users, see `/Users/stefan/work/lsp-dap/README.md`.

## Repository layout

- `cmd/dap-adapter`: DAP server entrypoint
- `cmd/diagnostic-summary`: support bundle summarizer CLI
- `internal/ride/*`: RIDE transport/protocol/session state
- `internal/dap/adapter`: DAP request handling and runtime wiring
- `internal/integration/harness`: fake/live integration harness
- `internal/support/diagbundle`: support bundle summary model
- `vscode-extension/`: VS Code extension source and packaging

## Local prerequisites

- Go 1.22+
- Node.js 20+
- npm
- VS Code (for extension host and manual UI tests)

## Core local validation

```bash
go test ./... -count=1
go vet ./...
```

## Extension validation

```bash
cd vscode-extension
npm ci
npm run lint
npm test -- --runInBand
npm run test:exthost
npm run build
```

## Live Dyalog validation

Start Dyalog in RIDE SERVE mode, then run live suites:

```bash
export DYALOG_RIDE_ADDR=127.0.0.1:4502

go test ./internal/integration/harness -run '^TestLiveDyalog_' -count=1 -v
go test ./cmd/dap-adapter -run '^TestLiveDAPAdapter_' -count=1 -v
```

Interactive E2E:

```bash
export DYALOG_E2E_REQUIRE=1
go test ./cmd/dap-adapter -run '^TestLiveDAPAdapter_InteractiveWorkflow$' -count=1 -v
```

## Extension packaging

```bash
cd vscode-extension
npm run package:vsix
```

## Release process

Use these references when preparing releases:

- `/Users/stefan/work/lsp-dap/docs/releases/release-checklist.md`
- `/Users/stefan/work/lsp-dap/docs/releases/release-notes-template.md`

Release workflows:

- `/Users/stefan/work/lsp-dap/.github/workflows/release.yml`
- `/Users/stefan/work/lsp-dap/.github/workflows/extension-release.yml`

## Supportability workflows

- support triage: `/Users/stefan/work/lsp-dap/docs/support/triage.md`
- issue template: `/Users/stefan/work/lsp-dap/.github/ISSUE_TEMPLATE/diagnostic-support.yml`
- bundle summary CLI:

```bash
go run ./cmd/diagnostic-summary --json <bundle.json>
```

## Design and planning references

- protocol/design analysis: `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md`
- beta readiness policy: `/Users/stefan/work/lsp-dap/docs/validations/beta-readiness.md`
- issue PR/review records: `/Users/stefan/work/lsp-dap/docs/prs/`, `/Users/stefan/work/lsp-dap/docs/reviews/`
