import test from "node:test";
import assert from "node:assert/strict";
import { expandWorkspace, isObject, resolveAdapterPath } from "../adapterPath";

test("expandWorkspace replaces workspaceFolder token", () => {
  const actual = expandWorkspace("${workspaceFolder}/dap-adapter", "/tmp/ws");
  assert.equal(actual, "/tmp/ws/dap-adapter");
});

test("isObject accepts plain objects only", () => {
  assert.equal(isObject({ a: 1 }), true);
  assert.equal(isObject([]), false);
  assert.equal(isObject(null), false);
  assert.equal(isObject("text"), false);
});

test("resolveAdapterPath uses explicit adapterPath first", () => {
  const seen: string[] = [];
  const actual = resolveAdapterPath(
    { adapterPath: "${workspaceFolder}/bin/dap-adapter" },
    { uri: { fsPath: "/tmp/ws" } } as any,
    {},
    (candidate) => {
      seen.push(candidate);
      return candidate === "/tmp/ws/bin/dap-adapter";
    }
  );
  assert.equal(actual, "/tmp/ws/bin/dap-adapter");
  assert.deepEqual(seen, ["/tmp/ws/bin/dap-adapter"]);
});

test("resolveAdapterPath falls back to env then workspace candidates", () => {
  const actual = resolveAdapterPath(
    {},
    { uri: { fsPath: "/tmp/ws" } } as any,
    { DYALOG_DAP_ADAPTER_PATH: "/opt/dap-adapter" },
    (candidate) => candidate === "/opt/dap-adapter"
  );
  assert.equal(actual, "/opt/dap-adapter");
});
