import path from "path";
import * as vscode from "vscode";
import { expandWorkspace, resolveAdapterPath } from "../adapterPath";
import {
  ensureLaunchConfigText,
  starterLaunchConfiguration,
  validateAdapterPathCandidate,
  validateRideAddress
} from "../setup";
import { logDiagnostic, type DiagnosticHistory } from "../diagnostics/logger";

export async function runSetupLaunchConfig(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory
): Promise<void> {
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

export async function runValidateAdapterPath(
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

export async function runValidateRideAddr(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory
): Promise<void> {
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

export async function runToggleDiagnosticsVerbose(
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
