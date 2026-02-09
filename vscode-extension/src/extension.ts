import * as vscode from "vscode";
import { isObject, resolveAdapterPath } from "./adapterPath";

type DebugConfig = vscode.DebugConfiguration;

export function activate(context: vscode.ExtensionContext): void {
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
        throw new Error(
          "Unable to locate dap-adapter. Set launch.json adapterPath or DYALOG_DAP_ADAPTER_PATH."
        );
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

      return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
    }
  });

  context.subscriptions.push(configProvider, descriptorFactory);
}

export function deactivate(): void {}
