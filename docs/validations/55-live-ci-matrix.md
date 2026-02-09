# Live CI Matrix Policy

Context reference: `/Users/stefan/work/lsp-dap/docs/plans/ride-protocol.md`

## Objective

Define a repeatable live compatibility matrix across supported OS and Dyalog variants, separated from fast gates, with explicit promotion/release-readiness rules.

## Matrix Dimensions

Current matrix profiles (live/slow):

- Linux x64 + Dyalog 19.0
- macOS arm64 + Dyalog 19.0
- Windows x64 + Dyalog 19.0

Profiles run on self-hosted runners with Dyalog preinstalled and RIDE endpoints reachable.

## Fast vs Slow Gates

Fast gates (required on PR/push):

- protocol/adapter unit and integration tests against deterministic harness/fake paths
- extension lint/test/build/contract checks

Slow gates (optional-by-env, required for release readiness when enabled):

- live interpreter smoke
- live adapter interactive workflow
- artifact capture and reliability metrics

## Promotion Policy

- `main` merge: fast gates must pass.
- release readiness: when `LIVE_MATRIX_REQUIRED=1`, latest successful `live-matrix.yml` run on `main` must be within policy freshness window (`LIVE_MATRIX_MAX_AGE_DAYS`).
- if live matrix freshness check fails, release workflow must stop before publishing artifacts.

## Environment and Secrets Strategy

Hosted runners:

- run fast gates only (no Dyalog secrets/binaries required)

self-hosted runners (live matrix):

- provide RIDE endpoint vars per profile (repo vars), for example:
  - `DYALOG_RIDE_ADDR_LINUX_190`
  - `DYALOG_RIDE_ADDR_MACOS_190`
  - `DYALOG_RIDE_ADDR_WINDOWS_190`
- optional launch/bin vars per profile for controlled startup
- no plaintext credentials in repository; use Actions vars/secrets only

## Reliability Metrics

Each live matrix job writes metrics artifacts under `artifacts/integration/metrics/` with:

- profile
- outcome
- timestamp
- run metadata

These artifacts are uploaded on every run (`if: always()`) for trend/flake analysis.
