import * as vscode from "vscode";
import { resolveDebugConfigurationContract } from "./contracts";
import { runGenerateDiagnosticBundle, runInstallAdapter } from "./commands/supportCommands";
import {
  runSetupLaunchConfig,
  runToggleDiagnosticsVerbose,
  runValidateAdapterPath,
  runValidateRideAddr
} from "./commands/setupCommands";
import { createAdapterDescriptorFactory } from "./debug/descriptorFactory";
import { createDiagnosticHistory, logDiagnostic } from "./diagnostics/logger";

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Dyalog DAP");
  const diagnostics = createDiagnosticHistory(2000);
  context.subscriptions.push(output);
  logDiagnostic(output, diagnostics, "info", "extension.activate", {
    extension: "dyalog-dap",
    version: context.extension.packageJSON.version ?? "unknown"
  });

  const setupLaunchCommand = vscode.commands.registerCommand("dyalogDap.setupLaunchConfig", async () => {
    await runSetupLaunchConfig(output, diagnostics);
  });
  const validateAdapterPathCommand = vscode.commands.registerCommand("dyalogDap.validateAdapterPath", async () => {
    await runValidateAdapterPath(output, diagnostics);
  });
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
  const installAdapterCommand = vscode.commands.registerCommand("dyalogDap.installAdapter", async () => {
    await runInstallAdapter(output, diagnostics, context);
  });

  const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
    resolveDebugConfiguration(_folder, config: vscode.DebugConfiguration) {
      return resolveDebugConfigurationContract(config as Record<string, unknown>) as vscode.DebugConfiguration;
    }
  });

  const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory(
    "dyalog-dap",
    createAdapterDescriptorFactory(output, diagnostics)
  );

  context.subscriptions.push(
    setupLaunchCommand,
    validateAdapterPathCommand,
    validateRideAddrCommand,
    toggleDiagnosticsVerboseCommand,
    generateDiagnosticBundleCommand,
    installAdapterCommand,
    configProvider,
    descriptorFactory
  );
}

export function deactivate(): void {}
