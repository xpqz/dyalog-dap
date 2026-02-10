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
exports.createAdapterDescriptorFactory = createAdapterDescriptorFactory;
const node_fs_1 = __importDefault(require("node:fs"));
const vscode = __importStar(require("vscode"));
const contracts_1 = require("../contracts");
const installedAdapterFallback_1 = require("../installedAdapterFallback");
const logger_1 = require("../diagnostics/logger");
function createAdapterDescriptorFactory(output, diagnostics) {
    return {
        createDebugAdapterDescriptor(session) {
            const config = (session.configuration ?? {});
            const workspacePath = session.workspaceFolder?.uri?.fsPath ?? "";
            const installedPath = vscode.workspace
                .getConfiguration("dyalogDap")
                .get("adapter.installedPath", "");
            const normalizedConfig = (0, installedAdapterFallback_1.applyInstalledAdapterFallback)(config, workspacePath, installedPath, node_fs_1.default.existsSync);
            const contract = (0, contracts_1.buildAdapterLaunchContract)(normalizedConfig, workspacePath, process.env);
            if (typeof contract.error === "string" && contract.error !== "") {
                (0, logger_1.logDiagnostic)(output, diagnostics, "error", "adapter.resolve.failed", {
                    rideAddr: asNonEmptyString(config.rideAddr),
                    workspace: session.workspaceFolder?.uri?.fsPath ?? ""
                });
                throw new Error(contract.error);
            }
            const adapterPath = contract.adapterPath;
            const args = contract.args;
            const env = contract.env;
            (0, logger_1.logDiagnostic)(output, diagnostics, "info", "adapter.spawn", {
                adapterPath,
                request: asNonEmptyString(config.request),
                rideAddr: asNonEmptyString(config.rideAddr),
                args: args.length
            });
            (0, logger_1.logDiagnostic)(output, diagnostics, "debug", "adapter.env", {
                hasRideAddr: env.DYALOG_RIDE_ADDR !== undefined && env.DYALOG_RIDE_ADDR !== ""
            });
            return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
        }
    };
}
function asNonEmptyString(value) {
    if (typeof value !== "string") {
        return "";
    }
    return value.trim();
}
