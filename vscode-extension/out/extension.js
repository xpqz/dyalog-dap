"use strict";

const fs = require("fs");
const path = require("path");
const vscode = require("vscode");

function activate(context) {
  const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
    resolveDebugConfiguration(folder, config) {
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
      const config = session.configuration || {};
      const adapterPath = resolveAdapterPath(config, session.workspaceFolder);
      if (!adapterPath) {
        throw new Error(
          "Unable to locate dap-adapter. Set launch.json adapterPath or DYALOG_DAP_ADAPTER_PATH."
        );
      }

      const args = Array.isArray(config.adapterArgs) ? config.adapterArgs.map(String) : [];
      const env = { ...process.env };
      if (isObject(config.adapterEnv)) {
        for (const [key, value] of Object.entries(config.adapterEnv)) {
          env[String(key)] = String(value);
        }
      }
      if (config.rideAddr && !env.DYALOG_RIDE_ADDR) {
        env.DYALOG_RIDE_ADDR = String(config.rideAddr);
      }

      return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
    }
  });

  context.subscriptions.push(configProvider, descriptorFactory);
}

function deactivate() {}

function resolveAdapterPath(config, workspaceFolder) {
  const candidates = [];
  const windows = process.platform === "win32";
  const executableName = windows ? "dap-adapter.exe" : "dap-adapter";
  const workspacePath = workspaceFolder && workspaceFolder.uri ? workspaceFolder.uri.fsPath : "";

  if (typeof config.adapterPath === "string" && config.adapterPath.trim() !== "") {
    candidates.push(expandWorkspace(config.adapterPath.trim(), workspacePath));
  }

  if (typeof process.env.DYALOG_DAP_ADAPTER_PATH === "string" && process.env.DYALOG_DAP_ADAPTER_PATH !== "") {
    candidates.push(process.env.DYALOG_DAP_ADAPTER_PATH);
  }

  if (workspacePath) {
    candidates.push(path.join(workspacePath, executableName));
    candidates.push(path.join(workspacePath, "bin", executableName));
    candidates.push(path.join(workspacePath, "dist", executableName));
  }

  for (const candidate of candidates) {
    if (candidate && fs.existsSync(candidate)) {
      return candidate;
    }
  }
  return "";
}

function expandWorkspace(value, workspacePath) {
  if (!workspacePath) {
    return value;
  }
  return value.replace(/\$\{workspaceFolder\}/g, workspacePath);
}

function isObject(value) {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

module.exports = {
  activate,
  deactivate
};
