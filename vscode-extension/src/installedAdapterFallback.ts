import fs from "node:fs";
import { expandWorkspace } from "./adapterPath";

type DebugConfig = Record<string, unknown>;

export function applyInstalledAdapterFallback(
  config: DebugConfig,
  workspacePath: string,
  installedPath: string,
  fileExists: (candidate: string) => boolean = fs.existsSync
): DebugConfig {
  const configured = asNonEmptyString(config.adapterPath);
  if (configured !== "") {
    const resolved = expandWorkspace(configured, workspacePath);
    if (fileExists(resolved)) {
      return config;
    }
  }

  const installed = installedPath.trim();
  if (installed === "") {
    return config;
  }

  return {
    ...config,
    adapterPath: installed
  };
}

function asNonEmptyString(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
}
