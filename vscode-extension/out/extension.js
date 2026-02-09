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
const adapterPath_1 = require("./adapterPath");
function activate(context) {
    const configProvider = vscode.debug.registerDebugConfigurationProvider("dyalog-dap", {
        resolveDebugConfiguration(_folder, config) {
            if (!config.type && !config.request && !config.name) {
                return {
                    type: "dyalog-dap",
                    name: "Dyalog: Launch (RIDE)",
                    request: "launch",
                    rideAddr: "127.0.0.1:4502",
                    adapterPath: "${workspaceFolder}/dap-adapter"
                };
            }
            if (!config.type) {
                config.type = "dyalog-dap";
            }
            if (!config.request) {
                config.request = "launch";
            }
            if (!config.name) {
                config.name = "Dyalog: Debug";
            }
            return config;
        }
    });
    const descriptorFactory = vscode.debug.registerDebugAdapterDescriptorFactory("dyalog-dap", {
        createDebugAdapterDescriptor(session) {
            const config = (session.configuration ?? {});
            const adapterPath = (0, adapterPath_1.resolveAdapterPath)(config, session.workspaceFolder);
            if (adapterPath === "") {
                throw new Error("Unable to locate dap-adapter. Set launch.json adapterPath or DYALOG_DAP_ADAPTER_PATH.");
            }
            const args = Array.isArray(config.adapterArgs) ? config.adapterArgs.map(String) : [];
            const env = { ...process.env };
            if ((0, adapterPath_1.isObject)(config.adapterEnv)) {
                for (const [key, value] of Object.entries(config.adapterEnv)) {
                    env[String(key)] = String(value);
                }
            }
            if (typeof config.rideAddr === "string" && config.rideAddr !== "" && !env.DYALOG_RIDE_ADDR) {
                env.DYALOG_RIDE_ADDR = config.rideAddr;
            }
            return new vscode.DebugAdapterExecutable(adapterPath, args, { env });
        }
    });
    context.subscriptions.push(configProvider, descriptorFactory);
}
function deactivate() { }
