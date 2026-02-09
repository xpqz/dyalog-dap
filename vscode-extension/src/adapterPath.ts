import fs from "fs";
import path from "path";

type UnknownRecord = Record<string, unknown>;
export type WorkspaceFolderLike = {
  uri?: {
    fsPath?: string;
  };
};

export function resolveAdapterPath(
  config: UnknownRecord,
  workspaceFolder: WorkspaceFolderLike | undefined,
  env: NodeJS.ProcessEnv = process.env,
  fileExists: (candidate: string) => boolean = fs.existsSync,
  platform: NodeJS.Platform = process.platform
): string {
  const candidates: string[] = [];
  const windows = platform === "win32";
  const executableName = windows ? "dap-adapter.exe" : "dap-adapter";
  const workspacePath = workspaceFolder?.uri?.fsPath ?? "";

  const adapterPath = asNonEmptyString(config.adapterPath);
  if (adapterPath !== "") {
    candidates.push(expandWorkspace(adapterPath, workspacePath));
  }

  const envPath = asNonEmptyString(env.DYALOG_DAP_ADAPTER_PATH);
  if (envPath !== "") {
    candidates.push(envPath);
  }

  if (workspacePath !== "") {
    candidates.push(path.join(workspacePath, executableName));
    candidates.push(path.join(workspacePath, "bin", executableName));
    candidates.push(path.join(workspacePath, "dist", executableName));
  }

  for (const candidate of candidates) {
    if (candidate !== "" && fileExists(candidate)) {
      return candidate;
    }
  }
  return "";
}

export function expandWorkspace(value: string, workspacePath: string): string {
  if (workspacePath === "") {
    return value;
  }
  return value.replace(/\$\{workspaceFolder\}/g, workspacePath);
}

export function isObject(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function asNonEmptyString(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
}
