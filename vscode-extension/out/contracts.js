"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.resolveDebugConfigurationContract = resolveDebugConfigurationContract;
exports.buildAdapterLaunchContract = buildAdapterLaunchContract;
const fs_1 = __importDefault(require("fs"));
const adapterPath_1 = require("./adapterPath");
function resolveDebugConfigurationContract(config) {
    if (!config.type && !config.request && !config.name) {
        return {
            type: "dyalog-dap",
            name: "Dyalog: Launch (RIDE)",
            request: "launch",
            rideAddr: "127.0.0.1:4502",
            adapterPath: "${workspaceFolder}/dap-adapter"
        };
    }
    const resolved = { ...config };
    if (!resolved.type) {
        resolved.type = "dyalog-dap";
    }
    if (!resolved.request) {
        resolved.request = "launch";
    }
    if (!resolved.name) {
        resolved.name = "Dyalog: Debug";
    }
    return resolved;
}
function buildAdapterLaunchContract(config, workspacePath, baseEnv = process.env, fileExists = fs_1.default.existsSync, platform = process.platform) {
    const workspaceFolder = workspacePath !== ""
        ? {
            uri: {
                fsPath: workspacePath
            }
        }
        : undefined;
    const adapterPath = (0, adapterPath_1.resolveAdapterPath)(config, workspaceFolder, baseEnv, fileExists, platform);
    const env = toStringEnv(baseEnv);
    if ((0, adapterPath_1.isObject)(config.adapterEnv)) {
        for (const [key, value] of Object.entries(config.adapterEnv)) {
            env[String(key)] = String(value);
        }
    }
    if (adapterPath === "") {
        return {
            adapterPath: "",
            args: [],
            env,
            error: "Unable to locate dap-adapter. Run 'Dyalog DAP: Validate Adapter Path', set launch.json adapterPath, or set DYALOG_DAP_ADAPTER_PATH."
        };
    }
    const args = Array.isArray(config.adapterArgs) ? config.adapterArgs.map(String) : [];
    if (typeof config.rideAddr === "string" && config.rideAddr !== "" && !env.DYALOG_RIDE_ADDR) {
        env.DYALOG_RIDE_ADDR = config.rideAddr;
    }
    return {
        adapterPath,
        args,
        env
    };
}
function toStringEnv(env) {
    const result = {};
    for (const [key, value] of Object.entries(env)) {
        if (typeof value === "string") {
            result[key] = value;
        }
    }
    return result;
}
