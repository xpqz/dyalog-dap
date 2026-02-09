# Dyalog DAP Support Triage Workflow

This document defines the first-pass support flow for debugging incidents reported from user machines.

## 1. Collect the diagnostic bundle

From VS Code, run:

- `Dyalog DAP: Generate Diagnostic Bundle`

This writes a single JSON artifact under:

- `.dyalog-dap/support/diagnostic-bundle-<timestamp>.json`

The diagnostic bundle includes:

- recent extension diagnostics (from the `Dyalog DAP` output channel mirror)
- launch/config snapshots relevant to `dyalog-dap`
- transcript pointers (for example `rideTranscriptsDir`, `DYALOG_RIDE_TRANSCRIPTS_DIR`, and default artifacts location)

## 2. Confirm redaction before sharing

Bundle generation applies redact rules automatically for sensitive keys and likely endpoint/path values.

Before attaching the bundle to an issue, do a quick manual scan for:

- credentials in unexpected fields
- private hostnames or filesystem paths that should be removed
- command-line flags containing secrets

If needed, scrub those fields and keep a note that manual redaction was applied.

## 3. First-pass analysis checklist

Use the bundle artifacts for quick classification:

- connection/setup failures (invalid `rideAddr`, missing adapter path, unreachable endpoint)
- handshake/protocol failures (connect succeeds, then transport/protocol errors)
- runtime debug failures (breakpoint mapping, stack/scopes/variables/source/evaluate errors)
- environment-specific failures (OS/path differences, launch command incompatibilities)

Expected outputs from first-pass:

- issue class and suspected subsystem (`extension`, `dap-adapter`, `ride transport`, `runtime`)
- whether reproduction appears deterministic
- required follow-up artifacts (full transcript files, minimal repro workspace, exact Dyalog version/build)

Use the bundle summarizer for consistent first-pass extraction:

```bash
go run ./cmd/diagnostic-summary <bundle.json>
```

or machine-readable output:

```bash
go run ./cmd/diagnostic-summary --json <bundle.json>
```

The summary highlights probable incident class and missing artifacts.

## 4. Escalation package

When escalating beyond first-pass, include:

- the diagnostic bundle JSON
- referenced transcript artifacts
- user launch configuration (with redacted secrets)
- Dyalog version/platform details

This keeps triage consistent and minimizes back-and-forth with the reporter.

## 5. Intake template

For new incidents, use issue template:

- `.github/ISSUE_TEMPLATE/diagnostic-support.yml`

This ensures reports include bundle attachment, diagnostic-summary output, reproduction steps, and transcript artifacts.
