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
exports.activate = activate;
exports.deactivate = deactivate;
const path_1 = __importDefault(require("path"));
const vscode = __importStar(require("vscode"));
const adapterPath_1 = require("./adapterPath");
const adapterInstaller_1 = require("./adapterInstaller");
const contracts_1 = require("./contracts");
const diagnosticsBundle_1 = require("./diagnosticsBundle");
const setup_1 = require("./setup");
function activate(context) {
    const output = vscode.window.createOutputChannel("Dyalog DAP");
    const diagnostics = createDiagnosticHistory(2000);
    context.subscriptions.push(output);
    logDiagnostic(output, diagnostics, "info", "extension.activate", {
        extension: "dyalog-dap",
        version: context.extension.packageJSON.version ?? "unknown"
    });
    const setupLaunchCommand = vscode.commands.registerCommand("dyalogDap.setupLaunchConfig", async () => {
        await runSetupLaunchConfig(output, diagnostics);
    });
    const validateAdapterPathCommand = vscode.commands.registerCommand("dyalogDap.validateAdapterPath", async () => {
        await runValidateAdapterPath(output, diagnostics);
    });
    const validateRideAddrCommand = vscode.commands.registerCommand("dyalogDap.validateRideAddr", async () => {
        await runValidateRideAddr(output, diagnostics);
    });
    const toggleDiagnosticsVerboseCommand = vscode.commands.registerCommand("dyalogDap.toggleDiagnosticsVerbose", async () => {
        await runToggleDiagnosticsVerbose(output, diagnostics);
    });
    const generateDiagnosticBundleCommand = vscode.commands.registerCommand("dyalogDap.generateDiagnosticBundle", async () => {
        await runGenerateDiagnosticBundle(output, diagnostics, context.extension.packageJSON.version ?? "unknown");
    });
    const installAdapterCommand = vscode.commands.registerCommand("dyalogDap.installAdapter", async () => {
        await runInstallAdapter(output, diagnostics, context);
    });
    const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
        resolveDebugConfiguration(_folder, config) {
            return (0, contracts_1.resolveDebugConfigurationContract)(config);
        }
    });
    const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory("dyalog-dap", {
        createDebugAdapterDescriptor(session) {
            const config = (session.configuration ?? {});
            const normalizedConfig = applyInstalledAdapterFallback(config);
            const contract = (0, contracts_1.buildAdapterLaunchContract)(normalizedConfig, session.workspaceFolder?.uri?.fsPath ?? "", process.env);
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
    });
    context.subscriptions.push(setupLaunchCommand, validateAdapterPathCommand, validateRideAddrCommand, toggleDiagnosticsVerboseCommand, generateDiagnosticBundleCommand, installAdapterCommand, configProvider, descriptorFactory);
}
function deactivate() { }
async function runSetupLaunchConfig(output, diagnostics) {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    if (!workspaceFolder) {
        const message = "No workspace folder is open. Open a folder before running Dyalog setup.";
        logDiagnostic(output, diagnostics, "warn", "setup.launchConfig.noWorkspace", {});
        void vscode.window.showErrorMessage(message);
        return;
    }
    const launchDir = vscode.Uri.joinPath(workspaceFolder.uri, ".vscode");
    const launchURI = vscode.Uri.joinPath(workspaceFolder.uri, ".vscode", "launch.json");
    let currentText = "";
    try {
        const existing = await vscode.workspace.fs.readFile(launchURI);
        currentText = Buffer.from(existing).toString("utf8");
    }
    catch {
        currentText = "";
    }
    const starter = (0, setup_1.starterLaunchConfiguration)("${workspaceFolder}/dap-adapter");
    const ensured = (0, setup_1.ensureLaunchConfigText)(currentText, starter);
    if (typeof ensured.error === "string" && ensured.error !== "") {
        logDiagnostic(output, diagnostics, "error", "setup.launchConfig.invalidJson", { path: launchURI.fsPath });
        output.show(true);
        void vscode.window.showErrorMessage(ensured.error);
        return;
    }
    if (!ensured.changed) {
        logDiagnostic(output, diagnostics, "info", "setup.launchConfig.unchanged", { path: launchURI.fsPath });
        void vscode.window.showInformationMessage("Dyalog launch configuration already exists in launch.json.");
        return;
    }
    await vscode.workspace.fs.createDirectory(launchDir);
    await vscode.workspace.fs.writeFile(launchURI, Buffer.from(ensured.text, "utf8"));
    logDiagnostic(output, diagnostics, "info", "setup.launchConfig.updated", { path: launchURI.fsPath });
    void vscode.window.showInformationMessage("Added Dyalog debug configuration to .vscode/launch.json.");
}
async function runValidateAdapterPath(output, diagnostics) {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    const workspacePath = workspaceFolder?.uri?.fsPath ?? "";
    const executableName = process.platform === "win32" ? "dap-adapter.exe" : "dap-adapter";
    const defaultPath = (0, adapterPath_1.resolveAdapterPath)({}, workspaceFolder) ||
        (workspacePath !== "" ? path_1.default.join(workspacePath, executableName) : executableName);
    const userInput = await vscode.window.showInputBox({
        prompt: "Path to dap-adapter executable",
        value: defaultPath,
        ignoreFocusOut: true
    });
    if (userInput === undefined) {
        logDiagnostic(output, diagnostics, "debug", "setup.validateAdapterPath.cancelled", {});
        return;
    }
    const candidate = (0, adapterPath_1.expandWorkspace)(userInput, workspacePath);
    const result = (0, setup_1.validateAdapterPathCandidate)(candidate);
    if (result.ok) {
        logDiagnostic(output, diagnostics, "info", "setup.validateAdapterPath.ok", { candidate });
        void vscode.window.showInformationMessage(result.message);
        return;
    }
    logDiagnostic(output, diagnostics, "error", "setup.validateAdapterPath.failed", { candidate });
    output.show(true);
    void vscode.window.showErrorMessage(result.message);
}
async function runValidateRideAddr(output, diagnostics) {
    const userInput = await vscode.window.showInputBox({
        prompt: "RIDE endpoint (host:port)",
        value: "127.0.0.1:4502",
        ignoreFocusOut: true
    });
    if (userInput === undefined) {
        logDiagnostic(output, diagnostics, "debug", "setup.validateRideAddr.cancelled", {});
        return;
    }
    const result = (0, setup_1.validateRideAddress)(userInput);
    if (result.ok) {
        logDiagnostic(output, diagnostics, "info", "setup.validateRideAddr.ok", { rideAddr: userInput.trim() });
        void vscode.window.showInformationMessage(result.message);
        return;
    }
    logDiagnostic(output, diagnostics, "error", "setup.validateRideAddr.failed", { rideAddr: userInput.trim() });
    output.show(true);
    void vscode.window.showErrorMessage(result.message);
}
async function runToggleDiagnosticsVerbose(output, diagnostics) {
    const config = vscode.workspace.getConfiguration("dyalogDap");
    const current = config.get("diagnostics.verbose", false);
    const next = !current;
    await config.update("diagnostics.verbose", next, vscode.ConfigurationTarget.Global);
    logDiagnostic(output, diagnostics, "info", "diagnostics.verbose.toggled", { enabled: next });
    void vscode.window.showInformationMessage(`Dyalog DAP verbose diagnostics ${next ? "enabled" : "disabled"}.`);
}
async function runGenerateDiagnosticBundle(output, diagnostics, extensionVersion) {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    if (!workspaceFolder) {
        logDiagnostic(output, diagnostics, "warn", "support.bundle.noWorkspace", {});
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
                diagnosticsVerbose: isVerboseDiagnosticsEnabled()
            },
            activeSession: toSerializable(vscode.debug.activeDebugSession?.configuration ?? null)
        },
        transcriptPointers: collectTranscriptPointers(workspacePath, launchConfigs, process.env)
    });
    const bundleDirectory = vscode.Uri.joinPath(workspaceFolder.uri, ".dyalog-dap", "support");
    const bundleFile = vscode.Uri.joinPath(bundleDirectory, `diagnostic-bundle-${new Date().toISOString().replace(/[:.]/g, "-")}.json`);
    await vscode.workspace.fs.createDirectory(bundleDirectory);
    await vscode.workspace.fs.writeFile(bundleFile, Buffer.from(`${JSON.stringify(bundle, null, 2)}\n`, "utf8"));
    logDiagnostic(output, diagnostics, "info", "support.bundle.generated", {
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
        logDiagnostic(output, diagnostics, "info", "adapter.install.success", {
            path: result.adapterPath,
            release: result.versionTag,
            asset: result.assetName
        });
        output.show(true);
        void vscode.window.showInformationMessage(`Installed adapter ${result.versionTag} (${result.assetName}) to ${result.adapterPath}.`);
    }
    catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        logDiagnostic(output, diagnostics, "error", "adapter.install.failed", { error: message });
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
function createDiagnosticHistory(limit) {
    return {
        limit,
        lines: []
    };
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
function applyInstalledAdapterFallback(config) {
    if (asNonEmptyString(config.adapterPath) !== "") {
        return config;
    }
    const installed = vscode.workspace
        .getConfiguration("dyalogDap")
        .get("adapter.installedPath", "")
        .trim();
    if (installed === "") {
        return config;
    }
    return {
        ...config,
        adapterPath: installed
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
function asNonEmptyString(value) {
    if (typeof value !== "string") {
        return "";
    }
    return value.trim();
}
function isRecord(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value);
}
