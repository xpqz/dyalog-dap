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
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.runGenerateDiagnosticBundle = runGenerateDiagnosticBundle;
exports.runInstallAdapter = runInstallAdapter;
const path_1 = __importDefault(require("path"));
const vscode = __importStar(require("vscode"));
const adapterInstaller_1 = require("../adapterInstaller");
const adapterPath_1 = require("../adapterPath");
const diagnosticsBundle_1 = require("../diagnosticsBundle");
const logger_1 = require("../diagnostics/logger");
async function runGenerateDiagnosticBundle(output, diagnostics, extensionVersion) {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    if (!workspaceFolder) {
        (0, logger_1.logDiagnostic)(output, diagnostics, "warn", "support.bundle.noWorkspace", {});
        void vscode.window.showErrorMessage("No workspace folder is open. Open a folder before generating a diagnostic bundle.");
        return;
    }
    const launchConfigs = readLaunchConfigurations(workspaceFolder);
    const workspacePath = workspaceFolder.uri.fsPath;
    const bundle = (0, diagnosticsBundle_1.buildDiagnosticBundle)({
        extensionVersion,
        workspaceName: workspaceFolder.name,
        diagnostics: diagnostics.lines,
        env: filterEnvironmentForBundle(process.env),
        configSnapshot: {
            launchConfigurations: launchConfigs,
            extensionConfiguration: {
                diagnosticsVerbose: (0, logger_1.isVerboseDiagnosticsEnabled)()
            },
            activeSession: toSerializable(vscode.debug.activeDebugSession?.configuration ?? null)
        },
        transcriptPointers: collectTranscriptPointers(workspacePath, launchConfigs, process.env)
    });
    const bundleDirectory = vscode.Uri.joinPath(workspaceFolder.uri, ".dyalog-dap", "support");
    const bundleFile = vscode.Uri.joinPath(bundleDirectory, `diagnostic-bundle-${new Date().toISOString().replace(/[:.]/g, "-")}.json`);
    await vscode.workspace.fs.createDirectory(bundleDirectory);
    await vscode.workspace.fs.writeFile(bundleFile, Buffer.from(`${JSON.stringify(bundle, null, 2)}\n`, "utf8"));
    (0, logger_1.logDiagnostic)(output, diagnostics, "info", "support.bundle.generated", {
        path: bundleFile.fsPath,
        diagnostics: bundle.diagnostics.recent.length,
        transcripts: bundle.transcripts.pointers.length
    });
    output.show(true);
    void vscode.window.showInformationMessage(`Dyalog diagnostic bundle written to ${bundleFile.fsPath}`);
}
async function runInstallAdapter(output, diagnostics, context) {
    try {
        const installRoot = path_1.default.join(context.globalStorageUri.fsPath, "adapter");
        const result = await (0, adapterInstaller_1.installAdapterFromLatestRelease)({
            installRoot,
            platform: process.platform,
            arch: process.arch
        });
        await vscode.workspace
            .getConfiguration("dyalogDap")
            .update("adapter.installedPath", result.adapterPath, vscode.ConfigurationTarget.Global);
        (0, logger_1.logDiagnostic)(output, diagnostics, "info", "adapter.install.success", {
            path: result.adapterPath,
            release: result.versionTag,
            asset: result.assetName
        });
        output.show(true);
        void vscode.window.showInformationMessage(`Installed adapter ${result.versionTag} (${result.assetName}) to ${result.adapterPath}.`);
    }
    catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        (0, logger_1.logDiagnostic)(output, diagnostics, "error", "adapter.install.failed", { error: message });
        output.show(true);
        void vscode.window.showErrorMessage(`Failed to install adapter: ${message}`);
    }
}
function readLaunchConfigurations(workspaceFolder) {
    const launchConfigs = vscode.workspace
        .getConfiguration("launch", workspaceFolder)
        .get("configurations", []);
    if (!Array.isArray(launchConfigs)) {
        return [];
    }
    return launchConfigs.filter(isRecord).map((item) => toSerializable(item));
}
function collectTranscriptPointers(workspacePath, launchConfigs, env) {
    const pointers = new Set();
    const envPointer = env.DYALOG_RIDE_TRANSCRIPTS_DIR;
    if (typeof envPointer === "string" && envPointer.trim() !== "") {
        pointers.add(envPointer.trim());
    }
    for (const config of launchConfigs) {
        if (asNonEmptyString(config.type) !== "dyalog-dap") {
            continue;
        }
        const direct = asNonEmptyString(config.rideTranscriptsDir);
        if (direct !== "") {
            pointers.add((0, adapterPath_1.expandWorkspace)(direct, workspacePath));
        }
    }
    pointers.add(path_1.default.join(workspacePath, "artifacts", "integration"));
    return Array.from(pointers);
}
function toSerializable(value) {
    try {
        return JSON.parse(JSON.stringify(value));
    }
    catch {
        return String(value);
    }
}
function filterEnvironmentForBundle(env) {
    const snapshot = {};
    const keepKeyPatterns = [/^DYALOG_/i, /^RIDE_/i, /^(PATH|HOME|USERPROFILE|SHELL|OS)$/i];
    for (const [key, value] of Object.entries(env)) {
        if (keepKeyPatterns.some((pattern) => pattern.test(key))) {
            snapshot[key] = value;
        }
    }
    return snapshot;
}
function asNonEmptyString(value) {
    if (typeof value !== "string") {
        return "";
    }
    return value.trim();
}
function isRecord(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value);
}
