import fs from "node:fs";
import * as vscode from "vscode";
import { buildAdapterLaunchContract } from "../contracts";
import { applyInstalledAdapterFallback } from "../installedAdapterFallback";
import { logDiagnostic, type DiagnosticHistory } from "../diagnostics/logger";

type DebugConfig = vscode.DebugConfiguration;

export function createAdapterDescriptorFactory(
  output: vscode.OutputChannel,
  diagnostics: DiagnosticHistory
): vscode.DebugAdapterDescriptorFactory {
  return {
    createDebugAdapterDescriptor(session) {
      const config = (session.configuration ?? {}) as DebugConfig;
      const workspacePath = session.workspaceFolder?.uri?.fsPath ?? "";
      const installedPath = vscode.workspace
        .getConfiguration("dyalogDap")
        .get<string>("adapter.installedPath", "");
      const normalizedConfig = applyInstalledAdapterFallback(
        config as Record<string, unknown>,
        workspacePath,
        installedPath,
        fs.existsSync
      );
      const contract = buildAdapterLaunchContract(
        normalizedConfig as Record<string, unknown>,
        workspacePath,
        process.env
      );
      if (typeof contract.error === "string" && contract.error !== "") {
        logDiagnostic(output, diagnostics, "error", "adapter.resolve.failed", {
          rideAddr: asNonEmptyString(config.rideAddr),
          workspace: session.workspaceFolder?.uri?.fsPath ?? ""
        });
        throw new Error(contract.error);
      }

      const adapterPath = contract.adapterPath;
      const args = contract.args;
      const env = contract.env;
      logDiagnostic(output, diagnostics, "info", "adapter.spawn", {
        adapterPath,
        request: asNonEmptyString(config.request),
        rideAddr: asNonEmptyString(config.rideAddr),
        args: args.length
      });
      logDiagnostic(output, diagnostics, "debug", "adapter.env", {
        hasRideAddr: env.DYALOG_RIDE_ADDR !== undefined && env.DYALOG_RIDE_ADDR !== ""
      });

      return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
    }
  };
}

function asNonEmptyString(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
}
