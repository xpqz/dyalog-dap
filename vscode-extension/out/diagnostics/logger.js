"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.createDiagnosticHistory = createDiagnosticHistory;
exports.logDiagnostic = logDiagnostic;
exports.isVerboseDiagnosticsEnabled = isVerboseDiagnosticsEnabled;
const vscode = __importStar(require("vscode"));
function createDiagnosticHistory(limit) {
    return {
        limit,
        lines: []
    };
}
function logDiagnostic(output, diagnostics, level, message, fields) {
    if (level === "debug" && !isVerboseDiagnosticsEnabled()) {
        return;
    }
    const parts = [];
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
function isVerboseDiagnosticsEnabled() {
    return vscode.workspace.getConfiguration("dyalogDap").get("diagnostics.verbose", false);
}
