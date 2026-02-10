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
exports.activate = activate;
exports.deactivate = deactivate;
const vscode = __importStar(require("vscode"));
const contracts_1 = require("./contracts");
const supportCommands_1 = require("./commands/supportCommands");
const setupCommands_1 = require("./commands/setupCommands");
const descriptorFactory_1 = require("./debug/descriptorFactory");
const logger_1 = require("./diagnostics/logger");
function activate(context) {
    const output = vscode.window.createOutputChannel("Dyalog DAP");
    const diagnostics = (0, logger_1.createDiagnosticHistory)(2000);
    context.subscriptions.push(output);
    (0, logger_1.logDiagnostic)(output, diagnostics, "info", "extension.activate", {
        extension: "dyalog-dap",
        version: context.extension.packageJSON.version ?? "unknown"
    });
    const setupLaunchCommand = vscode.commands.registerCommand("dyalogDap.setupLaunchConfig", async () => {
        await (0, setupCommands_1.runSetupLaunchConfig)(output, diagnostics);
    });
    const validateAdapterPathCommand = vscode.commands.registerCommand("dyalogDap.validateAdapterPath", async () => {
        await (0, setupCommands_1.runValidateAdapterPath)(output, diagnostics);
    });
    const validateRideAddrCommand = vscode.commands.registerCommand("dyalogDap.validateRideAddr", async () => {
        await (0, setupCommands_1.runValidateRideAddr)(output, diagnostics);
    });
    const toggleDiagnosticsVerboseCommand = vscode.commands.registerCommand("dyalogDap.toggleDiagnosticsVerbose", async () => {
        await (0, setupCommands_1.runToggleDiagnosticsVerbose)(output, diagnostics);
    });
    const generateDiagnosticBundleCommand = vscode.commands.registerCommand("dyalogDap.generateDiagnosticBundle", async () => {
        await (0, supportCommands_1.runGenerateDiagnosticBundle)(output, diagnostics, context.extension.packageJSON.version ?? "unknown");
    });
    const installAdapterCommand = vscode.commands.registerCommand("dyalogDap.installAdapter", async () => {
        await (0, supportCommands_1.runInstallAdapter)(output, diagnostics, context);
    });
    const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
        resolveDebugConfiguration(_folder, config) {
            return (0, contracts_1.resolveDebugConfigurationContract)(config);
        }
    });
    const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory("dyalog-dap", (0, descriptorFactory_1.createAdapterDescriptorFactory)(output, diagnostics));
    context.subscriptions.push(setupLaunchCommand, validateAdapterPathCommand, validateRideAddrCommand, toggleDiagnosticsVerboseCommand, generateDiagnosticBundleCommand, installAdapterCommand, configProvider, descriptorFactory);
}
function deactivate() { }
