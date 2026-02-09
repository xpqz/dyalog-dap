# Dyalog DAP VS Code Extension

This extension contributes the `dyalog-dap` debug type and launches the `dap-adapter` executable.

## Compatibility

- The extension is tested against the adapter from this repository.
- If `dap-adapter` is not on a default path, set `DYALOG_DAP_ADAPTER_PATH` (or `adapterPath` in `launch.json`) to the exact binary location.
- Keep extension and adapter versions aligned when possible, especially around launch/attach argument changes.

## Local Development

```bash
cd vscode-extension
npm ci
npm run lint
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
