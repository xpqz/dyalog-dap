import * as vscode from "vscode";

export type DiagnosticFields = Record<string, unknown>;

export type DiagnosticHistory = {
  readonly limit: number;
  lines: string[];
};

export function createDiagnosticHistory(limit: number): DiagnosticHistory {
  return {
    limit,
    lines: []
  };
}

export function logDiagnostic(
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

export function isVerboseDiagnosticsEnabled(): boolean {
  return vscode.workspace.getConfiguration("dyalogDap").get<boolean>("diagnostics.verbose", false);
}
