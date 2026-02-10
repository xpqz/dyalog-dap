"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.applyInstalledAdapterFallback = applyInstalledAdapterFallback;
const node_fs_1 = __importDefault(require("node:fs"));
const adapterPath_1 = require("./adapterPath");
function applyInstalledAdapterFallback(config, workspacePath, installedPath, fileExists = node_fs_1.default.existsSync) {
    const configured = asNonEmptyString(config.adapterPath);
    if (configured !== "") {
        const resolved = (0, adapterPath_1.expandWorkspace)(configured, workspacePath);
        if (fileExists(resolved)) {
            return config;
        }
    }
    const installed = installedPath.trim();
    if (installed === "") {
        return config;
    }
    return {
        ...config,
        adapterPath: installed
    };
}
function asNonEmptyString(value) {
    if (typeof value !== "string") {
        return "";
    }
    return value.trim();
}
