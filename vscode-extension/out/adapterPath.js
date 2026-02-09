"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.resolveAdapterPath = resolveAdapterPath;
exports.expandWorkspace = expandWorkspace;
exports.isObject = isObject;
const fs_1 = __importDefault(require("fs"));
const path_1 = __importDefault(require("path"));
function resolveAdapterPath(config, workspaceFolder, env = process.env, fileExists = fs_1.default.existsSync) {
    const candidates = [];
    const windows = process.platform === "win32";
    const executableName = windows ? "dap-adapter.exe" : "dap-adapter";
    const workspacePath = workspaceFolder?.uri?.fsPath ?? "";
    const adapterPath = asNonEmptyString(config.adapterPath);
    if (adapterPath !== "") {
        candidates.push(expandWorkspace(adapterPath, workspacePath));
    }
    const envPath = asNonEmptyString(env.DYALOG_DAP_ADAPTER_PATH);
    if (envPath !== "") {
        candidates.push(envPath);
    }
    if (workspacePath !== "") {
        candidates.push(path_1.default.join(workspacePath, executableName));
        candidates.push(path_1.default.join(workspacePath, "bin", executableName));
        candidates.push(path_1.default.join(workspacePath, "dist", executableName));
    }
    for (const candidate of candidates) {
        if (candidate !== "" && fileExists(candidate)) {
            return candidate;
        }
    }
    return "";
}
function expandWorkspace(value, workspacePath) {
    if (workspacePath === "") {
        return value;
    }
    return value.replace(/\$\{workspaceFolder\}/g, workspacePath);
}
function isObject(value) {
    return !!value && typeof value === "object" && !Array.isArray(value);
}
function asNonEmptyString(value) {
    if (typeof value !== "string") {
        return "";
    }
    return value.trim();
}
