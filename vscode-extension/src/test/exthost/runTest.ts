import path from "node:path";
import os from "node:os";
import fs from "node:fs/promises";
import { runTests } from "@vscode/test-electron";

async function main(): Promise<void> {
  const extensionDevelopmentPath = path.resolve(__dirname, "../../../");
  const extensionTestsPath = path.resolve(__dirname, "./suite/index");
  const workspacePath = await fs.mkdtemp(path.join(os.tmpdir(), "dyalog-dap-exthost-"));
  try {
    await runTests({
      extensionDevelopmentPath,
      extensionTestsPath,
      launchArgs: [workspacePath, "--disable-extensions"]
    });
  } finally {
    await fs.rm(workspacePath, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error("Failed to run extension host tests");
  console.error(error);
  process.exit(1);
});
