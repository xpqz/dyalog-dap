import path from "path";
import * as vscode from "vscode";
import { expandWorkspace, isObject, resolveAdapterPath } from "./adapterPath";
import {
  ensureLaunchConfigText,
  starterLaunchConfiguration,
  validateAdapterPathCandidate,
  validateRideAddress
} from "./setup";

type DebugConfig = vscode.DebugConfiguration;
type DiagnosticFields = Record<string, unknown>;

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Dyalog DAP");
  context.subscriptions.push(output);
  logDiagnostic(output, "info", "extension.activate", {
    extension: "dyalog-dap",
    version: context.extension.packageJSON.version ?? "unknown"
  });

  const setupLaunchCommand = vscode.commands.registerCommand("dyalogDap.setupLaunchConfig", async () => {
    await runSetupLaunchConfig(output);
  });
  const validateAdapterPathCommand = vscode.commands.registerCommand(
    "dyalogDap.validateAdapterPath",
    async () => {
      await runValidateAdapterPath(output);
    }
  );
  const validateRideAddrCommand = vscode.commands.registerCommand("dyalogDap.validateRideAddr", async () => {
    await runValidateRideAddr(output);
  });
  const toggleDiagnosticsVerboseCommand = vscode.commands.registerCommand(
    "dyalogDap.toggleDiagnosticsVerbose",
    async () => {
      await runToggleDiagnosticsVerbose(output);
    }
  );

  const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
    resolveDebugConfiguration(_folder, config: vscode.DebugConfiguration) {
      if (!config.type && !config.request && !config.name) {
        return {
          type: "dyalog-dap",
          name: "Dyalog: Launch (RIDE)",
          request: "launch",
          rideAddr: "127.0.0.1:4502",
          adapterPath: "${workspaceFolder}/dap-adapter"
        };
      }
      if (!config.type) {
        config.type = "dyalog-dap";
      }
      if (!config.request) {
        config.request = "launch";
      }
      if (!config.name) {
        config.name = "Dyalog: Debug";
      }
      return config;
    }
  });

  const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory("dyalog-dap", {
    createDebugAdapterDescriptor(session) {
      const config = (session.configuration ?? {}) as DebugConfig;
      const adapterPath = resolveAdapterPath(config, session.workspaceFolder);
      if (adapterPath === "") {
        const message =
          "Unable to locate dap-adapter. Run 'Dyalog DAP: Validate Adapter Path', set launch.json adapterPath, or set DYALOG_DAP_ADAPTER_PATH.";
        logDiagnostic(output, "error", "adapter.resolve.failed", {
          rideAddr: asNonEmptyString(config.rideAddr),
          workspace: session.workspaceFolder?.uri?.fsPath ?? ""
        });
        throw new Error(message);
      }

      const args = Array.isArray(config.adapterArgs) ? config.adapterArgs.map(String) : [];
      const env: Record<string, string> = { ...process.env } as Record<string, string>;
      if (isObject(config.adapterEnv)) {
        for (const [key, value] of Object.entries(config.adapterEnv)) {
          env[String(key)] = String(value);
        }
      }
      if (typeof config.rideAddr === "string" && config.rideAddr !== "" && !env.DYALOG_RIDE_ADDR) {
        env.DYALOG_RIDE_ADDR = config.rideAddr;
      }
      logDiagnostic(output, "info", "adapter.spawn", {
        adapterPath,
        request: asNonEmptyString(config.request),
        rideAddr: asNonEmptyString(config.rideAddr),
        args: args.length
      });
      logDiagnostic(output, "debug", "adapter.env", {
        hasRideAddr: env.DYALOG_RIDE_ADDR !== undefined && env.DYALOG_RIDE_ADDR !== ""
      });

      return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
    }
  });

  context.subscriptions.push(
    setupLaunchCommand,
    validateAdapterPathCommand,
    validateRideAddrCommand,
    toggleDiagnosticsVerboseCommand,
    configProvider,
    descriptorFactory
  );
}

export function deactivate(): void {}

async function runSetupLaunchConfig(output: vscode.OutputChannel): Promise<void> {
  const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
  if (!workspaceFolder) {
    const message = "No workspace folder is open. Open a folder before running Dyalog setup.";
    logDiagnostic(output, "warn", "setup.launchConfig.noWorkspace", {});
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
    logDiagnostic(output, "error", "setup.launchConfig.invalidJson", { path: launchURI.fsPath });
    output.show(true);
    void vscode.window.showErrorMessage(ensured.error);
    return;
  }
  if (!ensured.changed) {
    logDiagnostic(output, "info", "setup.launchConfig.unchanged", { path: launchURI.fsPath });
    void vscode.window.showInformationMessage("Dyalog launch configuration already exists in launch.json.");
    return;
  }

  await vscode.workspace.fs.createDirectory(launchDir);
  await vscode.workspace.fs.writeFile(launchURI, Buffer.from(ensured.text, "utf8"));
  logDiagnostic(output, "info", "setup.launchConfig.updated", { path: launchURI.fsPath });
  void vscode.window.showInformationMessage("Added Dyalog debug configuration to .vscode/launch.json.");
}

async function runValidateAdapterPath(output: vscode.OutputChannel): Promise<void> {
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
    logDiagnostic(output, "debug", "setup.validateAdapterPath.cancelled", {});
    return;
  }

  const candidate = expandWorkspace(userInput, workspacePath);
  const result = validateAdapterPathCandidate(candidate);
  if (result.ok) {
    logDiagnostic(output, "info", "setup.validateAdapterPath.ok", { candidate });
    void vscode.window.showInformationMessage(result.message);
    return;
  }

  logDiagnostic(output, "error", "setup.validateAdapterPath.failed", { candidate });
  output.show(true);
  void vscode.window.showErrorMessage(result.message);
}

async function runValidateRideAddr(output: vscode.OutputChannel): Promise<void> {
  const userInput = await vscode.window.showInputBox({
    prompt: "RIDE endpoint (host:port)",
    value: "127.0.0.1:4502",
    ignoreFocusOut: true
  });
  if (userInput === undefined) {
    logDiagnostic(output, "debug", "setup.validateRideAddr.cancelled", {});
    return;
  }

  const result = validateRideAddress(userInput);
  if (result.ok) {
    logDiagnostic(output, "info", "setup.validateRideAddr.ok", { rideAddr: userInput.trim() });
    void vscode.window.showInformationMessage(result.message);
    return;
  }

  logDiagnostic(output, "error", "setup.validateRideAddr.failed", { rideAddr: userInput.trim() });
  output.show(true);
  void vscode.window.showErrorMessage(result.message);
}

async function runToggleDiagnosticsVerbose(output: vscode.OutputChannel): Promise<void> {
  const config = vscode.workspace.getConfiguration("dyalogDap");
  const current = config.get<boolean>("diagnostics.verbose", false);
  const next = !current;
  await config.update("diagnostics.verbose", next, vscode.ConfigurationTarget.Global);
  logDiagnostic(output, "info", "diagnostics.verbose.toggled", { enabled: next });
  void vscode.window.showInformationMessage(
    `Dyalog DAP verbose diagnostics ${next ? "enabled" : "disabled"}.`
  );
}

function logDiagnostic(
  output: vscode.OutputChannel,
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
  output.appendLine(`${new Date().toISOString()} [${level}] ${message}${suffix}`);
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
