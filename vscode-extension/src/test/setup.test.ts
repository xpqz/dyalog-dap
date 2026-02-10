import test from "node:test";
import assert from "node:assert/strict";
import {
  ensureLaunchConfigText,
  starterLaunchConfiguration,
  validateAdapterPathCandidate,
  validateRideAddress
} from "../setup";

test("starterLaunchConfiguration returns working dyalog-dap launch config", () => {
  const cfg = starterLaunchConfiguration("${workspaceFolder}/dap-adapter");
  assert.equal(cfg.type, "dyalog-dap");
  assert.equal(cfg.request, "launch");
  assert.equal(cfg.rideAddr, "127.0.0.1:4502");
  assert.equal(cfg.launchExpression, "");
  assert.equal(cfg.rideTranscriptsDir, "${workspaceFolder}/.dyalog-dap/transcripts");
  assert.equal(cfg.adapterPath, "${workspaceFolder}/dap-adapter");
});

test("validateRideAddress accepts host:port and rejects malformed values", () => {
  assert.equal(validateRideAddress("127.0.0.1:4502").ok, true);
  assert.equal(validateRideAddress("localhost:4502").ok, true);
  assert.equal(validateRideAddress("").ok, false);
  assert.equal(validateRideAddress("4502").ok, false);
  assert.equal(validateRideAddress("127.0.0.1:0").ok, false);
  assert.equal(validateRideAddress("127.0.0.1:70000").ok, false);
});

test("validateAdapterPathCandidate reports missing binary with fix guidance", () => {
  const missing = validateAdapterPathCandidate("/tmp/dap-adapter", () => false);
  assert.equal(missing.ok, false);
  assert.match(missing.message, /build/i);
  assert.match(missing.message, /DYALOG_DAP_ADAPTER_PATH/i);

  const found = validateAdapterPathCandidate("/tmp/dap-adapter", () => true);
  assert.equal(found.ok, true);
});

test("ensureLaunchConfigText creates and appends dyalog-dap config idempotently", () => {
  const starter = starterLaunchConfiguration("${workspaceFolder}/dap-adapter");

  const created = ensureLaunchConfigText("", starter);
  assert.equal(created.changed, true);
  const createdDoc = JSON.parse(created.text);
  assert.equal(createdDoc.version, "0.2.0");
  assert.equal(createdDoc.configurations.length, 1);

  const appended = ensureLaunchConfigText(
    JSON.stringify(
      {
        version: "0.2.0",
        configurations: [{ name: "Node", type: "node", request: "launch", program: "app.js" }]
      },
      null,
      2
    ),
    starter
  );
  assert.equal(appended.changed, true);
  const appendedDoc = JSON.parse(appended.text);
  assert.equal(appendedDoc.configurations.length, 2);

  const unchanged = ensureLaunchConfigText(appended.text, starter);
  assert.equal(unchanged.changed, false);
  assert.equal(unchanged.error, undefined);
});

test("ensureLaunchConfigText accepts launch.json JSONC comments and trailing commas", () => {
  const starter = starterLaunchConfiguration("${workspaceFolder}/dap-adapter");
  const jsonc = `{
    // Existing launch config
    "version": "0.2.0",
    "configurations": [
      {
        "name": "Node",
        "type": "node",
        "request": "launch",
      },
    ],
  }`;

  const result = ensureLaunchConfigText(jsonc, starter);
  assert.equal(result.changed, true);
  assert.equal(result.error, undefined);
  const doc = JSON.parse(result.text);
  assert.equal(doc.configurations.length, 2);
});

test("ensureLaunchConfigText avoids destructive overwrite on invalid syntax", () => {
  const starter = starterLaunchConfiguration("${workspaceFolder}/dap-adapter");
  const bad = "{ invalid";
  const result = ensureLaunchConfigText(bad, starter);
  assert.equal(result.changed, false);
  assert.equal(typeof result.error, "string");
  assert.equal(result.text, bad);
});
