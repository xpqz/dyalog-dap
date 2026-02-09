import test from "node:test";
import assert from "node:assert/strict";
import { buildAdapterLaunchContract, resolveDebugConfigurationContract } from "../contracts";

test("resolveDebugConfigurationContract returns default launch config for empty input", () => {
  const resolved = resolveDebugConfigurationContract({});
  assert.equal(resolved.type, "dyalog-dap");
  assert.equal(resolved.request, "launch");
  assert.equal(resolved.name, "Dyalog: Launch (RIDE)");
  assert.equal(resolved.rideAddr, "127.0.0.1:4502");
  assert.equal(resolved.adapterPath, "${workspaceFolder}/dap-adapter");
});

test("resolveDebugConfigurationContract fills missing fields without clobbering values", () => {
  const resolved = resolveDebugConfigurationContract({
    request: "attach",
    rideAddr: "localhost:4510"
  });
  assert.equal(resolved.type, "dyalog-dap");
  assert.equal(resolved.request, "attach");
  assert.equal(resolved.rideAddr, "localhost:4510");
  assert.equal(resolved.name, "Dyalog: Debug");
});

test("buildAdapterLaunchContract maps args and env with launch rideAddr", () => {
  const contract = buildAdapterLaunchContract(
    {
      request: "launch",
      rideAddr: "127.0.0.1:4502",
      adapterArgs: ["--verbose"],
      adapterEnv: { FOO: 42 }
    },
    "/tmp/ws",
    { PATH: "/usr/bin" },
    (candidate: string) => candidate === "/tmp/ws/dap-adapter",
    "linux"
  );
  assert.equal(contract.error, undefined);
  assert.equal(contract.adapterPath, "/tmp/ws/dap-adapter");
  assert.deepEqual(contract.args, ["--verbose"]);
  assert.equal(contract.env.DYALOG_RIDE_ADDR, "127.0.0.1:4502");
  assert.equal(contract.env.FOO, "42");
});

test("buildAdapterLaunchContract does not overwrite existing DYALOG_RIDE_ADDR", () => {
  const contract = buildAdapterLaunchContract(
    {
      request: "launch",
      rideAddr: "127.0.0.1:4502"
    },
    "/tmp/ws",
    { DYALOG_RIDE_ADDR: "10.0.0.1:4502" },
    (candidate: string) => candidate === "/tmp/ws/dap-adapter",
    "linux"
  );
  assert.equal(contract.error, undefined);
  assert.equal(contract.env.DYALOG_RIDE_ADDR, "10.0.0.1:4502");
});

test("buildAdapterLaunchContract returns actionable error when adapter is missing", () => {
  const contract = buildAdapterLaunchContract({}, "/tmp/ws", {}, () => false, "linux");
  assert.equal(typeof contract.error, "string");
  assert.match(contract.error ?? "", /validate adapter path/i);
  assert.equal(contract.adapterPath, "");
});
