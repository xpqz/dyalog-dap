import os from "node:os";
import path from "node:path";
import fs from "fs";
import { expandWorkspace, isObject, resolveAdapterPath, WorkspaceFolderLike } from "./adapterPath";

type UnknownRecord = Record<string, unknown>;

export type AdapterLaunchContract = {
  adapterPath: string;
  args: string[];
  env: Record<string, string>;
  error?: string;
};

export function resolveDebugConfigurationContract(config: UnknownRecord): UnknownRecord {
  const defaultTranscriptsDir = "${workspaceFolder}/.dyalog-dap/transcripts";
  if (!config.type && !config.request && !config.name) {
    return {
      type: "dyalog-dap",
      name: "Dyalog: Launch (RIDE)",
      request: "launch",
      rideAddr: "127.0.0.1:4502",
      launchExpression: "",
      rideTranscriptsDir: defaultTranscriptsDir,
      adapterPath: "${workspaceFolder}/dap-adapter"
    };
  }

  const resolved: UnknownRecord = { ...config };
  if (!resolved.type) {
    resolved.type = "dyalog-dap";
  }
  if (!resolved.request) {
    resolved.request = "launch";
  }
  if (!resolved.name) {
    resolved.name = "Dyalog: Debug";
  }
  if (!resolved.rideTranscriptsDir) {
    resolved.rideTranscriptsDir = defaultTranscriptsDir;
  }
  return resolved;
}

export function buildAdapterLaunchContract(
  config: UnknownRecord,
  workspacePath: string,
  baseEnv: NodeJS.ProcessEnv = process.env,
  fileExists: (candidate: string) => boolean = fs.existsSync,
  platform: NodeJS.Platform = process.platform
): AdapterLaunchContract {
  const workspaceFolder =
    workspacePath !== ""
      ? ({
          uri: {
            fsPath: workspacePath
          }
        } as WorkspaceFolderLike)
      : undefined;

  const adapterPath = resolveAdapterPath(config, workspaceFolder, baseEnv, fileExists, platform);

  const env = toStringEnv(baseEnv);
  if (isObject(config.adapterEnv)) {
    for (const [key, value] of Object.entries(config.adapterEnv)) {
      env[String(key)] = String(value);
    }
  }

  if (adapterPath === "") {
    return {
      adapterPath: "",
      args: [],
      env,
      error:
        "Unable to locate dap-adapter. Run 'Dyalog DAP: Validate Adapter Path', set launch.json adapterPath, or set DYALOG_DAP_ADAPTER_PATH."
    };
  }

  const args = Array.isArray(config.adapterArgs) ? config.adapterArgs.map(String) : [];
  if (typeof config.rideAddr === "string" && config.rideAddr !== "" && !env.DYALOG_RIDE_ADDR) {
    env.DYALOG_RIDE_ADDR = config.rideAddr;
  }
  if (!env.DYALOG_RIDE_TRANSCRIPTS_DIR) {
    const configured = asNonEmptyString(config.rideTranscriptsDir);
    if (configured !== "") {
      env.DYALOG_RIDE_TRANSCRIPTS_DIR = expandWorkspace(configured, workspacePath);
    } else if (workspacePath !== "") {
      env.DYALOG_RIDE_TRANSCRIPTS_DIR = path.join(workspacePath, ".dyalog-dap", "transcripts");
    } else {
      env.DYALOG_RIDE_TRANSCRIPTS_DIR = path.join(os.tmpdir(), "dyalog-dap", "transcripts");
    }
  }

  return {
    adapterPath,
    args,
    env
  };
}

function toStringEnv(env: NodeJS.ProcessEnv): Record<string, string> {
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(env)) {
    if (typeof value === "string") {
      result[key] = value;
    }
  }
  return result;
}

function asNonEmptyString(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
}
