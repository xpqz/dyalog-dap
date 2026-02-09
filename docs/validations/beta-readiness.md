# Beta Readiness Policy

This document defines beta readiness criteria and release gate expectations for `dyalog-dap`.

Plan context: `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md`.

## Beta Readiness Goals

- APL users can install and run interactive debugging with low friction.
- Maintainers can validate changes with repeatable automated checks.
- Support can triage user issues quickly using standard artifacts.

## Release Gate Policy

Before declaring a beta release candidate, these gates must pass:

1. Core quality gates:
   - `go test ./... -count=1`
   - `go vet ./...`
   - extension TypeScript lint/build/test
2. Live integration signal:
   - live Dyalog matrix and E2E workflows must be green within policy freshness window
3. Supportability gates:
   - diagnostic bundle generation path remains functional
   - support triage docs stay current with shipped workflow
4. Distribution gates:
   - release artifacts include checksums
   - user-facing install/upgrade instructions are present and verified

## Public Support Matrix

This support matrix states current beta targets:

- Dyalog: 19.0 baseline (newer versions are best-effort until explicitly added)
- OS:
  - macOS (arm64, amd64)
  - Linux (amd64, arm64)
  - Windows (amd64, arm64)
- VS Code:
  - current stable major series supported by extension manifest

## Required Runtime Coverage

Beta readiness requires ongoing coverage for:

- adapter protocol and runtime tests
- live Dyalog harness tests
- interactive debugger flow including breakpoints/step/resume
- VS Code extension workflows, including command coverage and vscode extension host integration

## Evidence Artifacts

Each beta release candidate should retain:

- CI run URLs for core and live gates
- release artifact checksums
- summary of known limitations/regressions
- support validation notes (including diagnostic bundle workflow status)
