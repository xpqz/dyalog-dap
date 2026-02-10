import test from "node:test";
import assert from "node:assert/strict";
import { applyInstalledAdapterFallback } from "../installedAdapterFallback";

test("keeps configured adapterPath when it exists", () => {
  const config = {
    adapterPath: "${workspaceFolder}/dap-adapter"
  };
  const resolved = applyInstalledAdapterFallback(
    config,
    "/tmp/ws",
    "/Users/me/.config/dyalog-dap/dap-adapter",
    (candidate: string) => candidate === "/tmp/ws/dap-adapter"
  );
  assert.equal(resolved.adapterPath, "${workspaceFolder}/dap-adapter");
});

test("falls back to installed adapter when configured adapterPath is missing", () => {
  const config = {
    adapterPath: "${workspaceFolder}/dap-adapter"
  };
  const resolved = applyInstalledAdapterFallback(
    config,
    "/tmp/ws",
    "/Users/me/.config/dyalog-dap/dap-adapter",
    () => false
  );
  assert.equal(resolved.adapterPath, "/Users/me/.config/dyalog-dap/dap-adapter");
});

test("falls back to installed adapter when adapterPath is empty", () => {
  const config = {};
  const resolved = applyInstalledAdapterFallback(
    config,
    "/tmp/ws",
    "/Users/me/.config/dyalog-dap/dap-adapter",
    () => false
  );
  assert.equal(resolved.adapterPath, "/Users/me/.config/dyalog-dap/dap-adapter");
});

test("does not change config when no installed adapter is available", () => {
  const config = {
    adapterPath: "${workspaceFolder}/dap-adapter"
  };
  const resolved = applyInstalledAdapterFallback(config, "/tmp/ws", "", () => false);
  assert.equal(resolved.adapterPath, "${workspaceFolder}/dap-adapter");
});
