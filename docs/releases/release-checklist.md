# Release Checklist (Adapter + Extension)

Use this checklist before shipping a tagged release.

Plan context: `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md`.

## Adapter Artifacts

- confirm all expected adapter artifacts are published for supported OS/arch
- verify `dist/checksums.txt` exists and includes all shipped archives
- verify at least one smoke install path from release archive to runnable `dap-adapter`

## Extension Artifacts

- confirm VSIX artifact exists and installs cleanly
- verify extension command surface includes current setup/support commands
- verify installer flow can resolve adapter binary from release assets

## Release Notes

- populate `docs/releases/release-notes-template.md`
- include installation and upgrade guidance for APL users
- include checksums verification guidance and support links

## Supportability

- confirm diagnostic bundle workflow and support triage docs are still accurate
- link known limitations and regression notes clearly
