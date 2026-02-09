import assert from "node:assert/strict";
import path from "node:path";
import fs from "node:fs/promises";
import * as vscode from "vscode";

type Bundle = {
  diagnostics: { recent: string[] };
  environment: Record<string, unknown>;
  configSnapshot: unknown;
  transcripts: { pointers: string[] };
};

export async function runDiagnosticBundleExtensionHostTest(): Promise<void> {
  const extension = vscode.extensions.getExtension("xpqz.dyalog-dap");
  assert.ok(extension, "expected xpqz.dyalog-dap extension to be available");
  await extension.activate();

  const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
  assert.ok(workspaceFolder, "expected extension host workspace folder");

  const previous = process.env.DYALOG_RIDE_TRANSCRIPTS_DIR;
  const transcriptDir = path.join(workspaceFolder.uri.fsPath, "host-transcripts");
  process.env.DYALOG_RIDE_TRANSCRIPTS_DIR = transcriptDir;

  try {
    await vscode.commands.executeCommand("dyalogDap.generateDiagnosticBundle");
    const supportDir = path.join(workspaceFolder.uri.fsPath, ".dyalog-dap", "support");
    const names = (await fs.readdir(supportDir)).filter((name) =>
      name.startsWith("diagnostic-bundle-")
    );
    assert.ok(names.length > 0, "expected diagnostic bundle artifact");

    const latest = names.sort().at(-1);
    assert.ok(latest, "expected latest bundle filename");
    const bundlePath = path.join(supportDir, latest);
    const payload = JSON.parse(await fs.readFile(bundlePath, "utf8")) as Bundle;

    assert.ok(
      payload.diagnostics.recent.some((line) => line.includes("extension.activate")),
      "expected extension activation marker in diagnostics"
    );
    assert.equal(
      payload.environment.DYALOG_RIDE_TRANSCRIPTS_DIR,
      "<redacted-path>",
      "expected transcript env value to be redacted"
    );
    assert.ok(payload.configSnapshot !== undefined, "expected config snapshot");
    assert.ok(Array.isArray(payload.transcripts.pointers), "expected transcript pointers");
    assert.ok(
      payload.transcripts.pointers.includes(path.join(workspaceFolder.uri.fsPath, "artifacts", "integration")),
      "expected default transcript pointer"
    );
  } finally {
    if (previous === undefined) {
      delete process.env.DYALOG_RIDE_TRANSCRIPTS_DIR;
    } else {
      process.env.DYALOG_RIDE_TRANSCRIPTS_DIR = previous;
    }
  }
}
