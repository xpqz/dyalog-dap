# Dyalog DAP for VS Code

Debug Dyalog APL in VS Code with breakpoints, stepping, stack inspection, variables, and evaluate.

This README is for APL users who want to use the debugger.
Developer workflows are in [docs/DEVELOP.md](docs/DEVELOP.md).

## What you need

- VS Code
- Dyalog APL with RIDE enabled
- Dyalog DAP VS Code extension

Release page:

- https://github.com/xpqz/dyalog-dap/releases

## Install in VS Code

1. Download the extension file `dyalog-dap-<version>.vsix` from Releases.
2. Install the VSIX:

```bash
code --install-extension dyalog-dap-0.1.0-beta.8.vsix --force
```

3. In VS Code, open Command Palette and run:

- `Dyalog DAP: Install/Update Adapter`

This downloads the matching adapter binary for your platform and verifies checksums automatically.

Release assets named `dyalog-dap_<version>_<os>_<arch>.tar.gz` (or `.zip`) are adapter binaries.

Alternative manual adapter configuration:

1. Download `dyalog-dap_<version>_<os>_<arch>.tar.gz` (or `.zip`) from Releases.
2. Unpack it and locate `dap-adapter` (`dap-adapter.exe` on Windows).
3. Set `adapterPath` in your debug config to that binary path.

## Start your Dyalog session

Start Dyalog with a RIDE listener, for example on port `4502`:

```bash
RIDE_INIT=SERVE:*:4502 dyalog +s -q
```

## Configure debugging

Open your APL workspace folder in VS Code.

Run:

- `Dyalog DAP: Setup Launch Configuration`

This adds a starter `.vscode/launch.json` entry.

Minimal launch config:

```json
{
  "name": "Dyalog: Launch (RIDE)",
  "type": "dyalog-dap",
  "request": "launch",
  "rideAddr": "127.0.0.1:4502",
  "launchExpression": "#.MyNs.MyFn 42",
  "rideTranscriptsDir": "${workspaceFolder}/.dyalog-dap/transcripts"
}
```

Attach config (if Dyalog is already running):

```json
{
  "name": "Dyalog: Attach (RIDE)",
  "type": "dyalog-dap",
  "request": "attach",
  "rideAddr": "127.0.0.1:4502"
}
```

Then press `F5` or start the config from Run and Debug.

The adapter applies breakpoints first, then evaluates `launchExpression` after `configurationDone`.
Use this to run a function with arguments as the debug entry point.

You can leave `launchExpression` empty if you only want to attach and drive execution manually from Dyalog.
If omitted, transcript logging defaults to a writable path under your workspace (`.dyalog-dap/transcripts`).

## APL debug console workflow

During a debug session, use the VS Code Debug Console to run APL expressions.

- entered expressions are sent using RIDE `Execute`
- interpreter output appears in the Debug Console output stream
- input echo lines from RIDE are suppressed to avoid duplicate console noise

This gives you a simple session-style console inside VS Code while debugging.

## Commands you will use

- `Dyalog DAP: Setup Launch Configuration`
- `Dyalog DAP: Validate Adapter Path`
- `Dyalog DAP: Validate RIDE Address`
- `Dyalog DAP: Install/Update Adapter`
- `Dyalog DAP: Generate Diagnostic Bundle`
- `Dyalog DAP: Toggle Verbose Diagnostics`

## Troubleshooting

If debug start fails:

- confirm Dyalog is running with `RIDE_INIT=SERVE:*:<port>`
- confirm `rideAddr` matches that port
- run `Dyalog DAP: Validate RIDE Address`
- run `Dyalog DAP: Install/Update Adapter` again
- if `adapterPath` in `launch.json` points to a missing file, either fix it or remove `adapterPath` to use the installer-managed adapter path

If you need support:

1. Run `Dyalog DAP: Generate Diagnostic Bundle`
2. Open an issue using the diagnostics-first support template
3. Include the bundle file and requested environment details

Support triage reference:

- [docs/support/triage.md](docs/support/triage.md)

## Beta readiness and support matrix

This project is currently beta.

Beta readiness policy:

- [docs/validations/beta-readiness.md](docs/validations/beta-readiness.md)

Current support matrix target:

- Dyalog 19.0 baseline
- macOS: arm64, amd64
- Linux: amd64, arm64
- Windows: amd64, arm64
- VS Code stable major supported by extension manifest
