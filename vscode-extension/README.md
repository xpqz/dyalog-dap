# Dyalog DAP VS Code Extension

This extension contributes the `dyalog-dap` debug type and launches the `dap-adapter` executable.

## Compatibility

- The extension is tested against the adapter from this repository.
- If `dap-adapter` is not on a default path, set `DYALOG_DAP_ADAPTER_PATH` (or `adapterPath` in `launch.json`) to the exact binary location.
- Keep extension and adapter versions aligned when possible, especially around launch/attach argument changes.

## Guided Setup and Diagnostics

Available commands from the VS Code command palette:

- `Dyalog DAP: Setup Launch Configuration` adds a starter `.vscode/launch.json` entry.
- `Dyalog DAP: Validate Adapter Path` validates the `dap-adapter` binary path and shows concrete fix guidance.
- `Dyalog DAP: Validate RIDE Address` validates `rideAddr` formatting (`host:port`).
- `Dyalog DAP: Toggle Verbose Diagnostics` toggles extra diagnostic output for support sessions.

Runtime diagnostics are written to the `Dyalog DAP` output channel.
Use verbose diagnostics when you need deeper startup/runtime traces without telemetry.

## Local Development

```bash
cd vscode-extension
npm ci
npm run lint
npm run test:contracts
npm run test
npm run build
```

## Package VSIX

```bash
cd vscode-extension
npm run package:vsix
```

The generated `.vsix` can be installed with:

```bash
cd vscode-extension
code --install-extension ./*.vsix --force
```
