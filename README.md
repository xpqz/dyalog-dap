# lsp-dap

Go-based RIDE protocol runtime and DAP adapter foundations for debugging Dyalog APL from VS Code.

## For APL Users (VS Code Debugging)

If you are an APL user and just want debugging to work in VS Code, start here.

Current status (as of February 9, 2026):

- The `dap-adapter` binary is now a long-running stdio DAP server.
- Core phase-1 debug commands are wired: initialize, launch/attach, threads, stackTrace, scopes, variables, source, evaluate, continue/step/pause, setBreakpoints.
- Live Dyalog smoke coverage is in CI and local tests.
- A minimal VS Code debugger extension is included under `vscode-extension/` (debug type: `dyalog-dap`).
- Automated VSIX packaging is available (local `npm run package:vsix` and tag-triggered GitHub Actions artifact build).

Quick start right now:

1. Start Dyalog with RIDE enabled:

```bash
RIDE_INIT=SERVE:*:4502 dyalog +s -q
```

2. Build and run the adapter:

```bash
go build ./cmd/dap-adapter
DYALOG_RIDE_ADDR=127.0.0.1:4502 ./dap-adapter
```

3. Point your DAP client to that adapter process and send `initialize` then `launch` (or `attach`).

4. For VS Code, use the bundled extension source in `vscode-extension/` (Extension Development Host) and a `launch.json` config using `"type": "dyalog-dap"`.

Supported `launch`/`attach` arguments:

- shared:
  - `rideAddr` (example `127.0.0.1:4502`)
  - `rideConnectTimeout` (optional, duration string, example `10s`)
  - `rideConnectTimeoutMs` (optional integer milliseconds)
  - `rideTranscriptsDir` (optional transcript output directory)
- `launch` only (adapter-owned interpreter lifecycle):
  - `rideLaunchCommand` (optional, adapter launches Dyalog)
  - `dyalogBin` (optional executable path; adapter derives `RIDE_INIT=SERVE:*:<port> ... +s -q`)
- `attach` policy:
  - connect-only; launch settings are not accepted
  - any `rideLaunchCommand`/`rideLaunch`/`dyalogBin` in `attach` will fail fast

## Getting Started in 5 Minutes

Run this as-is from a clean machine:

```bash
git clone <your-fork-or-origin-url>
cd lsp-dap

go version
go test ./... -count=1
go vet ./...
go build ./cmd/dap-adapter
```

If all commands pass, your environment is ready.

## Packaging and Installation

### Download prebuilt binaries (recommended)

Packaging is set up for GitHub Releases. For each tagged release, downloadable archives are produced for:

- macOS (`amd64`, `arm64`)
- Linux (`amd64`, `arm64`)
- Windows (`amd64`, `arm64`)

Each release includes:

- `dap-adapter` binary (or `dap-adapter.exe` on Windows)
- `checksums.txt` for integrity verification

Release page:

- `https://github.com/xpqz/dyalog-dap/releases`

### Local packaging command (maintainers)

```bash
goreleaser release --snapshot --clean
```

This writes local archives under `dist/` without creating a GitHub release.

## Quick Command Cheat Sheet

```bash
# full local gate
go test ./... -count=1 && go vet ./...

# build adapter binary
go build ./cmd/dap-adapter

# run only critical packages quickly
go test ./internal/ride/transport ./internal/ride/protocol ./internal/ride/sessionstate ./internal/dap/adapter ./internal/integration/harness -count=1

# integration harness only
go test ./internal/integration/harness -count=1

# run adapter against an already-running Dyalog RIDE endpoint
DYALOG_RIDE_ADDR=127.0.0.1:4502 go run ./cmd/dap-adapter
```

## Prerequisites

- Go `1.22+` (CI runs `1.22.x` and `1.23.x`)
- Git
- Optional for live integration: reachable Dyalog interpreter with RIDE enabled
- Optional for editor workflow: VS Code + Go extension

## What Is Implemented Right Now

- RIDE transport framing + protocol v2 handshake
- Typed RIDE protocol codec for phase-1 command surface
- Prompt-aware session dispatcher with queue/allow-list semantics
- DAP adapter core lifecycle, run control, thread/stack, breakpoint mapping
- DAP source retrieval and evaluate (watch/repl via ValueTip path)
- Long-running stdio DAP server entrypoint (`cmd/dap-adapter`)
- Integration harness with protocol transcript artifacts
- CI gating + VS Code smoke workflow scaffolding

Current limitation: VSIX packaging is automated, but Marketplace publication still requires manual publisher credentials/approval.

## Day-to-Day Development

### Standard Validation Loop

```bash
go test ./... -count=1
go vet ./...
```

### Focused Package Runs

```bash
go test ./internal/ride/transport -count=1
go test ./internal/ride/protocol -count=1
go test ./internal/ride/sessionstate -count=1
go test ./internal/dap/adapter -count=1
go test ./internal/integration/harness -count=1
```

## VS Code Workflow

Repository-provided config:

- `.vscode/launch.json`
- `.vscode/tasks.json`

Launch profiles:

- `DAP Adapter Smoke (Go)`
- `Harness Integration Smoke`

Tasks:

- `build dap-adapter`
- `test smoke`

Smoke flow in this repo:

1. Open folder in VS Code.
2. Run task `build dap-adapter`.
3. Start launch config `Harness Integration Smoke`.

For real interactive debugging UI, use a VS Code DAP client/extension setup that launches `dap-adapter` and sends `launch`/`attach` with `rideAddr`.

### Bundled extension (minimal)

The repository includes a minimal VS Code extension at `vscode-extension/`:

- debug type: `dyalog-dap`
- contribution schema for `launch`/`attach`
- adapter executable resolution (launch.json `adapterPath`, `DYALOG_DAP_ADAPTER_PATH`, workspace fallbacks)

Example debug configuration:

```json
{
  "name": "Dyalog: Launch (RIDE)",
  "type": "dyalog-dap",
  "request": "launch",
  "rideAddr": "127.0.0.1:4502",
  "adapterPath": "${workspaceFolder}/dap-adapter"
}
```

To try it quickly:

1. Build adapter binary at repo root: `go build ./cmd/dap-adapter`
2. Build and test extension package:

```bash
cd vscode-extension
npm install
npm run lint
npm run test
npm run build
```

3. Open `vscode-extension/` in VS Code
4. Press `F5` to launch an Extension Development Host
5. In the Extension Host, use the above `launch.json` configuration

## Live Dyalog Integration Harness

The integration harness can connect to a live RIDE endpoint and write protocol transcripts as JSON Lines.
When `DYALOG_RIDE_LAUNCH`/`DYALOG_BIN` is used, harness shutdown now uses process-group-aware termination on Unix (terminate, then kill fallback) to avoid orphaned Dyalog children.

Environment variables:

- `DYALOG_RIDE_ADDR` required, example `127.0.0.1:4502`
- `DYALOG_RIDE_LAUNCH` optional launch command for the harness to execute
- `DYALOG_BIN` optional Dyalog executable path used by live smoke when `DYALOG_RIDE_LAUNCH` is not set
- `DYALOG_RIDE_CONNECT_TIMEOUT` optional, defaults to `10s`
- `DYALOG_RIDE_TRANSCRIPTS_DIR` optional, defaults to `artifacts/integration`

Live smoke test command (real interpreter, not fake server):

```bash
export DYALOG_RIDE_ADDR=127.0.0.1:4502
export DYALOG_RIDE_CONNECT_TIMEOUT=10s
# Optional: auto-start Dyalog in SERVE mode (pattern used by gritt/RIDE flows)
# export DYALOG_RIDE_LAUNCH='RIDE_INIT=SERVE:*:4502 dyalog +s -q'
# or: export DYALOG_BIN='dyalog'   # live test will derive launch command from DYALOG_RIDE_ADDR port

go test ./internal/integration/harness -run '^TestLiveDyalog_' -count=1 -v
```

Live stdio adapter smoke (real DAP process boundary):

```bash
export DYALOG_RIDE_ADDR=127.0.0.1:4502
go test ./cmd/dap-adapter -run '^TestLiveDAPAdapter_' -count=1 -v
```

Connect-only mode (if Dyalog is already running):

```bash
export DYALOG_RIDE_ADDR=127.0.0.1:4502
unset DYALOG_RIDE_LAUNCH
unset DYALOG_BIN

go test ./internal/integration/harness -run '^TestLiveDyalog_' -count=1 -v
```

Transcripts are written under `artifacts/integration/` by default.

## Troubleshooting

### `go: command not found`

Install Go `1.22+`, reopen shell, then run `go version`.

### Tests fail due to cache or stale state

```bash
go clean -testcache
go test ./... -count=1
```

### Live harness times out

- Verify `DYALOG_RIDE_ADDR` points to an active RIDE listener.
- Increase timeout: `export DYALOG_RIDE_CONNECT_TIMEOUT=30s`.
- If startup is external, ensure interpreter launched with `RIDE_INIT=SERVE:*:<port>` and that `<port>` matches `DYALOG_RIDE_ADDR`.
- If auto-launching, verify the executable path via `DYALOG_BIN` or explicit `DYALOG_RIDE_LAUNCH`.

### Suspected orphaned Dyalog process after tests

- Current harness cleanup terminates the launched process group on Unix to avoid lingering child interpreters.
- If you still suspect leftovers from older runs, inspect and terminate manually:

```bash
pgrep -fl dyalog
pkill -f dyalog
```

### DAP `launch` or `attach` fails with missing ride address

- Provide `rideAddr` in launch/attach arguments, or set `DYALOG_RIDE_ADDR` before starting `dap-adapter`.

### DAP `attach` fails when launch settings are provided

- `attach` is now explicit connect-only mode and does not allow adapter-owned process launch.
- Remove `rideLaunchCommand`/`rideLaunch`/`dyalogBin` from `attach`, or switch request type to `launch`.

### Live smoke test is skipped

- `TestLiveDyalog_*` skips when `DYALOG_RIDE_ADDR` is not set.
- To force failure instead of skip (useful in CI), set `DYALOG_LIVE_REQUIRE=1`.

### No transcript artifacts appear

- Confirm harness test actually executed.
- Set explicit output directory:

```bash
export DYALOG_RIDE_TRANSCRIPTS_DIR=artifacts/integration
go test ./internal/integration/harness -count=1
```

## CI Strategy

Workflow file: `.github/workflows/ci.yml`

- `critical-gate`: Go matrix on core packages
- `full-suite`: `go test ./...` and `go vet ./...` after critical gate
- `live-dyalog`: optional, runs only when `DYALOG_RIDE_ADDR` is configured in CI; executes `TestLiveDyalog_*` and `TestLiveDAPAdapter_*`

## Architecture Overview

### `internal/ride/transport`

- TCP framing (`RIDE` magic + length-prefixed payload)
- Protocol v2 startup handshake
- Thread-safe full-duplex read/write
- Structured traffic logging (`JSONLTrafficLogger`)

### `internal/ride/protocol`

- Typed encode/decode for known RIDE commands
- Tolerant unknown-command handling
- Coverage for documented + selected undocumented commands

### `internal/ride/sessionstate`

- Single-reader dispatcher
- Prompt-type busy gating with allow-list
- Deferred send queueing/cancel semantics
- Save/Reply/Close ordering enforcement

### `internal/dap/adapter`

- DAP requests: `initialize`, `launch/attach`, `configurationDone`, `disconnect/terminate`
- DAP requests: `threads`, `stackTrace`, `continue`, `next`, `stepIn`, `stepOut`, `pause`, `setBreakpoints`
- RIDE event ingestion with tracer lifecycle and stopped synthesis
- Error/disconnect translation and reconnect rebuild trigger (`GetWindowLayout`)

### `internal/integration/harness`

- Reusable connect/launch harness for integration tests
- Protocol transcript artifact generation
- Fake-server and live-endpoint test support

## Supported Phase-1 DAP to RIDE Mappings

- `continue` -> `Continue`
- `next` -> `RunCurrentLine`
- `stepIn` -> `StepInto`
- `stepOut` -> `ContinueTrace`
- `pause` -> `WeakInterrupt` then `StrongInterrupt` fallback
- `threads` -> `GetThreads` with cached reply model
- `stackTrace` -> tracer-window-based frame modeling
- `setBreakpoints` -> `SetLineAttributes` with deferred apply for unopened sources

## Known Limitations

- Marketplace publication is not automated yet; release checklist is documented in `docs/releases/extension-vsix.md`.
- Some integration scenarios are fake-server deterministic flows
- Live interpreter matrix coverage depends on environment availability
- Prompt-mode semantics vary across interpreter/version combinations and are tracked as explicit open items

## Documentation Index

- Protocol analysis plan: `docs/plans/ride-protocol.md`
- Assumptions + traceability: `docs/traceability/assumptions.md`
- Undocumented command validation: `docs/validations/21-undocumented-commands.md`
- Per-issue PR guides: `docs/prs/`
- Per-issue review notes: `docs/reviews/`

## License

No license file is currently included in this repository.
