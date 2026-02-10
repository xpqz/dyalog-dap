import path from "path";
import * as vscode from "vscode";
import { installAdapterFromLatestRelease } from "../adapterInstaller";
import { expandWorkspace } from "../adapterPath";
import { buildDiagnosticBundle } from "../diagnosticsBundle";
import {
  isVerboseDiagnosticsEnabled,
  logDiagnostic,
  type DiagnosticHistory
} from "../diagnostics/logger";

export async function runGenerateDiagnosticBundle(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory,
  extensionVersion: string
): Promise<void> {
  const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
  if (!workspaceFolder) {
    logDiagnostic(output, diagnostics, "warn", "support.bundle.noWorkspace", {});
    void vscode.window.showErrorMessage(
      "No workspace folder is open. Open a folder before generating a diagnostic bundle."
    );
    return;
  }

  const launchConfigs = readLaunchConfigurations(workspaceFolder);
  const workspacePath = workspaceFolder.uri.fsPath;
  const bundle = buildDiagnosticBundle({
    extensionVersion,
    workspaceName: workspaceFolder.name,
    diagnostics: diagnostics.lines,
    env: filterEnvironmentForBundle(process.env),
    configSnapshot: {
      launchConfigurations: launchConfigs,
      extensionConfiguration: {
        diagnosticsVerbose: isVerboseDiagnosticsEnabled()
      },
      activeSession: toSerializable(vscode.debug.activeDebugSession?.configuration ?? null)
    },
    transcriptPointers: collectTranscriptPointers(workspacePath, launchConfigs, process.env)
  });

  const bundleDirectory = vscode.Uri.joinPath(workspaceFolder.uri, ".dyalog-dap", "support");
  const bundleFile = vscode.Uri.joinPath(
    bundleDirectory,
    `diagnostic-bundle-${new Date().toISOString().replace(/[:.]/g, "-")}.json`
  );
  await vscode.workspace.fs.createDirectory(bundleDirectory);
  await vscode.workspace.fs.writeFile(bundleFile, Buffer.from(`${JSON.stringify(bundle, null, 2)}\n`, "utf8"));

  logDiagnostic(output, diagnostics, "info", "support.bundle.generated", {
    path: bundleFile.fsPath,
    diagnostics: bundle.diagnostics.recent.length,
    transcripts: bundle.transcripts.pointers.length
  });
  output.show(true);
  void vscode.window.showInformationMessage(`Dyalog diagnostic bundle written to ${bundleFile.fsPath}`);
}

export async function runInstallAdapter(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory,
  context: vscode.ExtensionContext
): Promise<void> {
  try {
    const installRoot = path.join(context.globalStorageUri.fsPath, "adapter");
    const result = await installAdapterFromLatestRelease({
      installRoot,
      platform: process.platform,
      arch: process.arch
    });
    await vscode.workspace
      .getConfiguration("dyalogDap")
      .update("adapter.installedPath", result.adapterPath, vscode.ConfigurationTarget.Global);
    logDiagnostic(output, diagnostics, "info", "adapter.install.success", {
      path: result.adapterPath,
      release: result.versionTag,
      asset: result.assetName
    });
    output.show(true);
    void vscode.window.showInformationMessage(
      `Installed adapter ${result.versionTag} (${result.assetName}) to ${result.adapterPath}.`
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    logDiagnostic(output, diagnostics, "error", "adapter.install.failed", { error: message });
    output.show(true);
    void vscode.window.showErrorMessage(`Failed to install adapter: ${message}`);
  }
}

function readLaunchConfigurations(workspaceFolder: vscode.WorkspaceFolder): Array<Record<string, unknown>> {
  const launchConfigs = vscode.workspace
    .getConfiguration("launch", workspaceFolder)
    .get<unknown[]>("configurations", []);
  if (!Array.isArray(launchConfigs)) {
    return [];
  }
  return launchConfigs.filter(isRecord).map((item) => toSerializable(item) as Record<string, unknown>);
}

function collectTranscriptPointers(
  workspacePath: string,
  launchConfigs: Array<Record<string, unknown>>,
  env: NodeJS.ProcessEnv
): string[] {
  const pointers = new Set<string>();

  const envPointer = env.DYALOG_RIDE_TRANSCRIPTS_DIR;
  if (typeof envPointer === "string" && envPointer.trim() !== "") {
    pointers.add(envPointer.trim());
  }

  for (const config of launchConfigs) {
    if (asNonEmptyString(config.type) !== "dyalog-dap") {
      continue;
    }
    const direct = asNonEmptyString(config.rideTranscriptsDir);
    if (direct !== "") {
      pointers.add(expandWorkspace(direct, workspacePath));
    }
  }

  pointers.add(path.join(workspacePath, "artifacts", "integration"));
  return Array.from(pointers);
}

function toSerializable(value: unknown): unknown {
  try {
    return JSON.parse(JSON.stringify(value)) as unknown;
  } catch {
    return String(value);
  }
}

function filterEnvironmentForBundle(env: NodeJS.ProcessEnv): Record<string, string | undefined> {
  const snapshot: Record<string, string | undefined> = {};
  const keepKeyPatterns = [/^DYALOG_/i, /^RIDE_/i, /^(PATH|HOME|USERPROFILE|SHELL|OS)$/i];
  for (const [key, value] of Object.entries(env)) {
    if (keepKeyPatterns.some((pattern) => pattern.test(key))) {
      snapshot[key] = value;
    }
  }
  return snapshot;
}

function asNonEmptyString(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
