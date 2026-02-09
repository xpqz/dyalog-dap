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
const contracts_1 = require("./contracts");
const setup_1 = require("./setup");
function activate(context) {
    const output = vscode.window.createOutputChannel("Dyalog DAP");
    context.subscriptions.push(output);
    logDiagnostic(output, "info", "extension.activate", {
        extension: "dyalog-dap",
        version: context.extension.packageJSON.version ?? "unknown"
    });
    const setupLaunchCommand = vscode.commands.registerCommand("dyalogDap.setupLaunchConfig", async () => {
        await runSetupLaunchConfig(output);
    });
    const validateAdapterPathCommand = vscode.commands.registerCommand("dyalogDap.validateAdapterPath", async () => {
        await runValidateAdapterPath(output);
    });
    const validateRideAddrCommand = vscode.commands.registerCommand("dyalogDap.validateRideAddr", async () => {
        await runValidateRideAddr(output);
    });
    const toggleDiagnosticsVerboseCommand = vscode.commands.registerCommand("dyalogDap.toggleDiagnosticsVerbose", async () => {
        await runToggleDiagnosticsVerbose(output);
    });
    const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
        resolveDebugConfiguration(_folder, config) {
            return (0, contracts_1.resolveDebugConfigurationContract)(config);
        }
    });
    const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory("dyalog-dap", {
        createDebugAdapterDescriptor(session) {
            const config = (session.configuration ?? {});
            const contract = (0, contracts_1.buildAdapterLaunchContract)(config, session.workspaceFolder?.uri?.fsPath ?? "", process.env);
            if (typeof contract.error === "string" && contract.error !== "") {
                logDiagnostic(output, "error", "adapter.resolve.failed", {
                    rideAddr: asNonEmptyString(config.rideAddr),
                    workspace: session.workspaceFolder?.uri?.fsPath ?? ""
                });
                throw new Error(contract.error);
            }
            const adapterPath = contract.adapterPath;
            const args = contract.args;
            const env = contract.env;
            logDiagnostic(output, "info", "adapter.spawn", {
                adapterPath,
                request: asNonEmptyString(config.request),
                rideAddr: asNonEmptyString(config.rideAddr),
                args: args.length
            });
            logDiagnostic(output, "debug", "adapter.env", {
                hasRideAddr: env.DYALOG_RIDE_ADDR !== undefined && env.DYALOG_RIDE_ADDR !== ""
            });
            return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
        }
    });
    context.subscriptions.push(setupLaunchCommand, validateAdapterPathCommand, validateRideAddrCommand, toggleDiagnosticsVerboseCommand, configProvider, descriptorFactory);
}
function deactivate() { }
async function runSetupLaunchConfig(output) {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    if (!workspaceFolder) {
        const message = "No workspace folder is open. Open a folder before running Dyalog setup.";
        logDiagnostic(output, "warn", "setup.launchConfig.noWorkspace", {});
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
        logDiagnostic(output, "error", "setup.launchConfig.invalidJson", { path: launchURI.fsPath });
        output.show(true);
        void vscode.window.showErrorMessage(ensured.error);
        return;
    }
    if (!ensured.changed) {
        logDiagnostic(output, "info", "setup.launchConfig.unchanged", { path: launchURI.fsPath });
        void vscode.window.showInformationMessage("Dyalog launch configuration already exists in launch.json.");
        return;
    }
    await vscode.workspace.fs.createDirectory(launchDir);
    await vscode.workspace.fs.writeFile(launchURI, Buffer.from(ensured.text, "utf8"));
    logDiagnostic(output, "info", "setup.launchConfig.updated", { path: launchURI.fsPath });
    void vscode.window.showInformationMessage("Added Dyalog debug configuration to .vscode/launch.json.");
}
async function runValidateAdapterPath(output) {
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
        logDiagnostic(output, "debug", "setup.validateAdapterPath.cancelled", {});
        return;
    }
    const candidate = (0, adapterPath_1.expandWorkspace)(userInput, workspacePath);
    const result = (0, setup_1.validateAdapterPathCandidate)(candidate);
    if (result.ok) {
        logDiagnostic(output, "info", "setup.validateAdapterPath.ok", { candidate });
        void vscode.window.showInformationMessage(result.message);
        return;
    }
    logDiagnostic(output, "error", "setup.validateAdapterPath.failed", { candidate });
    output.show(true);
    void vscode.window.showErrorMessage(result.message);
}
async function runValidateRideAddr(output) {
    const userInput = await vscode.window.showInputBox({
        prompt: "RIDE endpoint (host:port)",
        value: "127.0.0.1:4502",
        ignoreFocusOut: true
    });
    if (userInput === undefined) {
        logDiagnostic(output, "debug", "setup.validateRideAddr.cancelled", {});
        return;
    }
    const result = (0, setup_1.validateRideAddress)(userInput);
    if (result.ok) {
        logDiagnostic(output, "info", "setup.validateRideAddr.ok", { rideAddr: userInput.trim() });
        void vscode.window.showInformationMessage(result.message);
        return;
    }
    logDiagnostic(output, "error", "setup.validateRideAddr.failed", { rideAddr: userInput.trim() });
    output.show(true);
    void vscode.window.showErrorMessage(result.message);
}
async function runToggleDiagnosticsVerbose(output) {
    const config = vscode.workspace.getConfiguration("dyalogDap");
    const current = config.get("diagnostics.verbose", false);
    const next = !current;
    await config.update("diagnostics.verbose", next, vscode.ConfigurationTarget.Global);
    logDiagnostic(output, "info", "diagnostics.verbose.toggled", { enabled: next });
    void vscode.window.showInformationMessage(`Dyalog DAP verbose diagnostics ${next ? "enabled" : "disabled"}.`);
}
function logDiagnostic(output, level, message, fields) {
    if (level === "debug" && !isVerboseDiagnosticsEnabled()) {
        return;
    }
    const parts = [];
    for (const [key, value] of Object.entries(fields)) {
        parts.push(`${key}=${JSON.stringify(value)}`);
    }
    const suffix = parts.length > 0 ? ` ${parts.join(" ")}` : "";
    output.appendLine(`${new Date().toISOString()} [${level}] ${message}${suffix}`);
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
