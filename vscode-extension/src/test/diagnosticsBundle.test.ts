import test from "node:test";
import assert from "node:assert/strict";
import { buildDiagnosticBundle, redactBundleValue } from "../diagnosticsBundle";

test("redactBundleValue redacts sensitive keys recursively", () => {
  const input = {
    token: "abc123",
    nested: {
      password: "secret",
      keep: "value",
      credential: "credential-value"
    },
    list: [{ apiKey: "key-1" }, { ghPat: "ghp_123" }, "ok"]
  };
  const redacted = redactBundleValue(input) as Record<string, unknown>;
  assert.equal(redacted.token, "<redacted>");
  assert.equal((redacted.nested as Record<string, unknown>).password, "<redacted>");
  assert.equal((redacted.nested as Record<string, unknown>).credential, "<redacted>");
  assert.equal((redacted.nested as Record<string, unknown>).keep, "value");
  const list = redacted.list as Array<Record<string, unknown> | string>;
  assert.equal((list[0] as Record<string, unknown>).apiKey, "<redacted>");
  assert.equal((list[1] as Record<string, unknown>).ghPat, "<redacted>");
  assert.equal(list[2], "ok");
});

test("buildDiagnosticBundle includes redacted env, config snapshot, and transcript pointers", () => {
  const bundle = buildDiagnosticBundle({
    extensionVersion: "0.0.1",
    workspaceName: "sample",
    diagnostics: ["line-a", "line-b"],
    env: {
      DYALOG_RIDE_ADDR: "10.0.0.1:4502",
      DYALOG_DAP_ADAPTER_PATH: "/Users/alice/bin/dap-adapter",
      MY_TOKEN: "abcd"
    },
    configSnapshot: {
      rideAddr: "10.0.0.1:4502",
      adapterEnv: {
        PASSWORD: "pw"
      }
    },
    transcriptPointers: ["/tmp/transcript-1.jsonl"]
  });

  assert.equal(bundle.extension.version, "0.0.1");
  assert.equal(bundle.workspace.name, "sample");
  assert.deepEqual(bundle.diagnostics.recent, ["line-a", "line-b"]);
  assert.equal(bundle.environment.DYALOG_RIDE_ADDR, "<redacted-endpoint>");
  assert.equal(bundle.environment.DYALOG_DAP_ADAPTER_PATH, "<redacted-path>");
  assert.equal(bundle.environment.MY_TOKEN, "<redacted>");
  assert.equal((bundle.configSnapshot as any).adapterEnv.PASSWORD, "<redacted>");
  assert.deepEqual(bundle.transcripts.pointers, ["/tmp/transcript-1.jsonl"]);
});
