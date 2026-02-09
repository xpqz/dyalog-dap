import path from "path";
import * as vscode from "vscode";
import { expandWorkspace, resolveAdapterPath } from "./adapterPath";
import { buildAdapterLaunchContract, resolveDebugConfigurationContract } from "./contracts";
import { buildDiagnosticBundle } from "./diagnosticsBundle";
import {
  ensureLaunchConfigText,
  starterLaunchConfiguration,
  validateAdapterPathCandidate,
  validateRideAddress
} from "./setup";

type DebugConfig = vscode.DebugConfiguration;
type DiagnosticFields = Record<string, unknown>;
type DiagnosticHistory = {
  readonly limit: number;
  lines: string[];
};

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Dyalog DAP");
  const diagnostics = createDiagnosticHistory(2000);
  context.subscriptions.push(output);
  logDiagnostic(
    output,
    diagnostics,
    "info",
    "extension.activate",
    {
      extension: "dyalog-dap",
      version: context.extension.packageJSON.version ?? "unknown"
    }
  );

  const setupLaunchCommand = vscode.commands.registerCommand("dyalogDap.setupLaunchConfig", async () => {
    await runSetupLaunchConfig(output, diagnostics);
  });
  const validateAdapterPathCommand = vscode.commands.registerCommand(
    "dyalogDap.validateAdapterPath",
    async () => {
      await runValidateAdapterPath(output, diagnostics);
    }
  );
  const validateRideAddrCommand = vscode.commands.registerCommand("dyalogDap.validateRideAddr", async () => {
    await runValidateRideAddr(output, diagnostics);
  });
  const toggleDiagnosticsVerboseCommand = vscode.commands.registerCommand(
    "dyalogDap.toggleDiagnosticsVerbose",
    async () => {
      await runToggleDiagnosticsVerbose(output, diagnostics);
    }
  );
  const generateDiagnosticBundleCommand = vscode.commands.registerCommand(
    "dyalogDap.generateDiagnosticBundle",
    async () => {
      await runGenerateDiagnosticBundle(output, diagnostics, context.extension.packageJSON.version ?? "unknown");
    }
  );

  const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
    resolveDebugConfiguration(_folder, config: vscode.DebugConfiguration) {
      return resolveDebugConfigurationContract(config as Record<string, unknown>) as vscode.DebugConfiguration;
    }
  });

  const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory("dyalog-dap", {
    createDebugAdapterDescriptor(session) {
      const config = (session.configuration ?? {}) as DebugConfig;
      const contract = buildAdapterLaunchContract(
        config as Record<string, unknown>,
        session.workspaceFolder?.uri?.fsPath ?? "",
        process.env
      );
      if (typeof contract.error === "string" && contract.error !== "") {
        logDiagnostic(
          output,
          diagnostics,
          "error",
          "adapter.resolve.failed",
          {
            rideAddr: asNonEmptyString(config.rideAddr),
            workspace: session.workspaceFolder?.uri?.fsPath ?? ""
          }
        );
        throw new Error(contract.error);
      }

      const adapterPath = contract.adapterPath;
      const args = contract.args;
      const env = contract.env;
      logDiagnostic(
        output,
        diagnostics,
        "info",
        "adapter.spawn",
        {
          adapterPath,
          request: asNonEmptyString(config.request),
          rideAddr: asNonEmptyString(config.rideAddr),
          args: args.length
        }
      );
      logDiagnostic(
        output,
        diagnostics,
        "debug",
        "adapter.env",
        {
          hasRideAddr: env.DYALOG_RIDE_ADDR !== undefined && env.DYALOG_RIDE_ADDR !== ""
        }
      );

      return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
    }
  });

  context.subscriptions.push(
    setupLaunchCommand,
    validateAdapterPathCommand,
    validateRideAddrCommand,
    toggleDiagnosticsVerboseCommand,
    generateDiagnosticBundleCommand,
    configProvider,
    descriptorFactory
  );
}

export function deactivate(): void {}

async function runSetupLaunchConfig(output: vscode.OutputChannel, diagnostics: DiagnosticHistory): Promise<void> {
  const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
  if (!workspaceFolder) {
    const message = "No workspace folder is open. Open a folder before running Dyalog setup.";
    logDiagnostic(output, diagnostics, "warn", "setup.launchConfig.noWorkspace", {});
    void vscode.window.showErrorMessage(message);
    return;
  }

  const launchDir = vscode.Uri.joinPath(workspaceFolder.uri, ".vscode");
  const launchURI = vscode.Uri.joinPath(workspaceFolder.uri, ".vscode", "launch.json");

  let currentText = "";
  try {
    const existing = await vscode.workspace.fs.readFile(launchURI);
    currentText = Buffer.from(existing).toString("utf8");
  } catch {
    currentText = "";
  }

  const starter = starterLaunchConfiguration("${workspaceFolder}/dap-adapter");
  const ensured = ensureLaunchConfigText(currentText, starter);
  if (typeof ensured.error === "string" && ensured.error !== "") {
    logDiagnostic(output, diagnostics, "error", "setup.launchConfig.invalidJson", { path: launchURI.fsPath });
    output.show(true);
    void vscode.window.showErrorMessage(ensured.error);
    return;
  }
  if (!ensured.changed) {
    logDiagnostic(output, diagnostics, "info", "setup.launchConfig.unchanged", { path: launchURI.fsPath });
    void vscode.window.showInformationMessage("Dyalog launch configuration already exists in launch.json.");
    return;
  }

  await vscode.workspace.fs.createDirectory(launchDir);
  await vscode.workspace.fs.writeFile(launchURI, Buffer.from(ensured.text, "utf8"));
  logDiagnostic(output, diagnostics, "info", "setup.launchConfig.updated", { path: launchURI.fsPath });
  void vscode.window.showInformationMessage("Added Dyalog debug configuration to .vscode/launch.json.");
}

async function runValidateAdapterPath(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory
): Promise<void> {
  const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
  const workspacePath = workspaceFolder?.uri?.fsPath ?? "";
  const executableName = process.platform === "win32" ? "dap-adapter.exe" : "dap-adapter";
  const defaultPath =
    resolveAdapterPath({}, workspaceFolder) ||
    (workspacePath !== "" ? path.join(workspacePath, executableName) : executableName);

  const userInput = await vscode.window.showInputBox({
    prompt: "Path to dap-adapter executable",
    value: defaultPath,
    ignoreFocusOut: true
  });
  if (userInput === undefined) {
    logDiagnostic(output, diagnostics, "debug", "setup.validateAdapterPath.cancelled", {});
    return;
  }

  const candidate = expandWorkspace(userInput, workspacePath);
  const result = validateAdapterPathCandidate(candidate);
  if (result.ok) {
    logDiagnostic(output, diagnostics, "info", "setup.validateAdapterPath.ok", { candidate });
    void vscode.window.showInformationMessage(result.message);
    return;
  }

  logDiagnostic(output, diagnostics, "error", "setup.validateAdapterPath.failed", { candidate });
  output.show(true);
  void vscode.window.showErrorMessage(result.message);
}

async function runValidateRideAddr(output: vscode.OutputChannel, diagnostics: DiagnosticHistory): Promise<void> {
  const userInput = await vscode.window.showInputBox({
    prompt: "RIDE endpoint (host:port)",
    value: "127.0.0.1:4502",
    ignoreFocusOut: true
  });
  if (userInput === undefined) {
    logDiagnostic(output, diagnostics, "debug", "setup.validateRideAddr.cancelled", {});
    return;
  }

  const result = validateRideAddress(userInput);
  if (result.ok) {
    logDiagnostic(output, diagnostics, "info", "setup.validateRideAddr.ok", { rideAddr: userInput.trim() });
    void vscode.window.showInformationMessage(result.message);
    return;
  }

  logDiagnostic(output, diagnostics, "error", "setup.validateRideAddr.failed", { rideAddr: userInput.trim() });
  output.show(true);
  void vscode.window.showErrorMessage(result.message);
}

async function runToggleDiagnosticsVerbose(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory
): Promise<void> {
  const config = vscode.workspace.getConfiguration("dyalogDap");
  const current = config.get<boolean>("diagnostics.verbose", false);
  const next = !current;
  await config.update("diagnostics.verbose", next, vscode.ConfigurationTarget.Global);
  logDiagnostic(output, diagnostics, "info", "diagnostics.verbose.toggled", { enabled: next });
  void vscode.window.showInformationMessage(
    `Dyalog DAP verbose diagnostics ${next ? "enabled" : "disabled"}.`
  );
}

async function runGenerateDiagnosticBundle(
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

function createDiagnosticHistory(limit: number): DiagnosticHistory {
  return {
    limit,
    lines: []
  };
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

function logDiagnostic(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory,
  level: "debug" | "info" | "warn" | "error",
  message: string,
  fields: DiagnosticFields
): void {
  if (level === "debug" && !isVerboseDiagnosticsEnabled()) {
    return;
  }
  const parts: string[] = [];
  for (const [key, value] of Object.entries(fields)) {
    parts.push(`${key}=${JSON.stringify(value)}`);
  }
  const suffix = parts.length > 0 ? ` ${parts.join(" ")}` : "";
  const line = `${new Date().toISOString()} [${level}] ${message}${suffix}`;
  output.appendLine(line);
  diagnostics.lines.push(line);
  if (diagnostics.lines.length > diagnostics.limit) {
    diagnostics.lines.splice(0, diagnostics.lines.length - diagnostics.limit);
  }
}

function isVerboseDiagnosticsEnabled(): boolean {
  return vscode.workspace.getConfiguration("dyalogDap").get<boolean>("diagnostics.verbose", false);
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
