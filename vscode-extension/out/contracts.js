"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.resolveDebugConfigurationContract = resolveDebugConfigurationContract;
exports.buildAdapterLaunchContract = buildAdapterLaunchContract;
const node_os_1 = __importDefault(require("node:os"));
const node_path_1 = __importDefault(require("node:path"));
const fs_1 = __importDefault(require("fs"));
const adapterPath_1 = require("./adapterPath");
function resolveDebugConfigurationContract(config) {
    const defaultTranscriptsDir = "${workspaceFolder}/.dyalog-dap/transcripts";
    if (!config.type && !config.request && !config.name) {
        return {
            type: "dyalog-dap",
            name: "Dyalog: Launch (RIDE)",
            request: "launch",
            rideAddr: "127.0.0.1:4502",
            launchExpression: "",
            rideTranscriptsDir: defaultTranscriptsDir,
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
    if (!resolved.rideTranscriptsDir) {
        resolved.rideTranscriptsDir = defaultTranscriptsDir;
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
    if (!env.DYALOG_RIDE_TRANSCRIPTS_DIR) {
        const configured = asNonEmptyString(config.rideTranscriptsDir);
        if (configured !== "") {
            env.DYALOG_RIDE_TRANSCRIPTS_DIR = (0, adapterPath_1.expandWorkspace)(configured, workspacePath);
        }
        else if (workspacePath !== "") {
            env.DYALOG_RIDE_TRANSCRIPTS_DIR = node_path_1.default.join(workspacePath, ".dyalog-dap", "transcripts");
        }
        else {
            env.DYALOG_RIDE_TRANSCRIPTS_DIR = node_path_1.default.join(node_os_1.default.tmpdir(), "dyalog-dap", "transcripts");
        }
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
function asNonEmptyString(value) {
    if (typeof value !== "string") {
        return "";
    }
    return value.trim();
}
