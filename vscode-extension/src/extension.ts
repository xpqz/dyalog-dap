import * as vscode from "vscode";
import { activate as activateExtension, deactivate as deactivateExtension } from "./activation";

export function activate(context: vscode.ExtensionContext): void {
  activateExtension(context);
}

export function deactivate(): void {
  deactivateExtension();
}
