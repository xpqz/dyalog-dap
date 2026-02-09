# Extension VSIX Release Checklist

Issue context: `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md` and SKIS issue `#47`.

## Preflight

- Verify adapter and extension tests are green:
  - `go test ./... -count=1`
  - `go vet ./...`
  - `cd vscode-extension && npm ci && npm run lint && npm run test`
- Confirm the extension version in `vscode-extension/package.json` is incremented for this release.
- Confirm `vscode-extension/README.md` compatibility notes still match current adapter behavior.

## Build VSIX

- Run `cd vscode-extension && npm run package:vsix`.
- Confirm a `.vsix` artifact exists in `vscode-extension/`.
- Smoke-install locally:
  - from repo root: `code --install-extension ./vscode-extension/*.vsix --force`
  - or from `vscode-extension/`: `code --install-extension ./*.vsix --force`

## Publish Artifacts

- GitHub:
  - Create/push a version tag `vX.Y.Z`.
  - Confirm `.github/workflows/extension-release.yml` succeeds and uploads the VSIX artifact.
  - Attach VSIX to GitHub release notes for easy download.
- Marketplace:
  - Validate publisher credentials and permissions.
  - Publish the same VSIX through the VS Code Marketplace workflow.

## Adapter Compatibility Gate

- Confirm the release notes state which `dap-adapter` version/commit was validated.
- Confirm `DYALOG_DAP_ADAPTER_PATH` override behavior is documented for users with non-default install paths.
