"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.starterLaunchConfiguration = starterLaunchConfiguration;
exports.validateRideAddress = validateRideAddress;
exports.validateAdapterPathCandidate = validateAdapterPathCandidate;
exports.ensureLaunchConfigText = ensureLaunchConfigText;
const fs_1 = __importDefault(require("fs"));
function starterLaunchConfiguration(adapterPath) {
    return {
        name: "Dyalog: Launch (RIDE)",
        type: "dyalog-dap",
        request: "launch",
        rideAddr: "127.0.0.1:4502",
        launchExpression: "",
        adapterPath
    };
}
function validateRideAddress(value) {
    const text = value.trim();
    if (text === "") {
        return {
            ok: false,
            message: "RIDE address is empty. Use host:port, for example 127.0.0.1:4502."
        };
    }
    const idx = text.lastIndexOf(":");
    if (idx <= 0 || idx === text.length - 1) {
        return {
            ok: false,
            message: "RIDE address must be host:port (for example 127.0.0.1:4502)."
        };
    }
    const host = text.slice(0, idx).trim();
    const portText = text.slice(idx + 1).trim();
    const port = Number.parseInt(portText, 10);
    if (host === "" || Number.isNaN(port) || port < 1 || port > 65535) {
        return {
            ok: false,
            message: "RIDE port must be numeric and between 1 and 65535."
        };
    }
    return {
        ok: true,
        message: `RIDE address looks valid: ${host}:${port}.`
    };
}
function validateAdapterPathCandidate(candidatePath, fileExists = fs_1.default.existsSync) {
    const text = candidatePath.trim();
    if (text === "") {
        return {
            ok: false,
            message: "Adapter path is empty. Build dap-adapter (go build ./cmd/dap-adapter) or set DYALOG_DAP_ADAPTER_PATH."
        };
    }
    if (!fileExists(text)) {
        return {
            ok: false,
            message: `Adapter not found at ${text}. Build dap-adapter (go build ./cmd/dap-adapter) or set DYALOG_DAP_ADAPTER_PATH.`
        };
    }
    return {
        ok: true,
        message: `Adapter path is valid: ${text}`
    };
}
function ensureLaunchConfigText(currentText, starter) {
    let doc = {
        version: "0.2.0",
        configurations: []
    };
    const trimmed = currentText.trim();
    if (trimmed !== "") {
        const parsed = parseLaunchDocument(trimmed);
        if (!parsed.ok) {
            return {
                changed: false,
                text: currentText,
                error: "launch.json contains invalid JSON syntax. Fix launch.json and re-run 'Dyalog DAP: Setup Launch Configuration'."
            };
        }
        doc = {
            version: parsed.version,
            configurations: parsed.configurations
        };
    }
    const alreadyPresent = doc.configurations.some((cfg) => {
        if (!cfg || typeof cfg !== "object" || Array.isArray(cfg)) {
            return false;
        }
        const typeValue = cfg.type;
        return typeof typeValue === "string" && typeValue.trim() === "dyalog-dap";
    });
    if (alreadyPresent) {
        return {
            changed: false,
            text: JSON.stringify(doc, null, 2) + "\n"
        };
    }
    doc.configurations.push(starter);
    return {
        changed: true,
        text: JSON.stringify(doc, null, 2) + "\n"
    };
}
function parseLaunchDocument(text) {
    try {
        const sanitized = removeTrailingCommas(stripJSONComments(text));
        const parsed = JSON.parse(sanitized);
        const parsedVersion = typeof parsed.version === "string" ? parsed.version : "0.2.0";
        const parsedConfigs = Array.isArray(parsed.configurations)
            ? parsed.configurations
            : [];
        return {
            ok: true,
            version: parsedVersion,
            configurations: parsedConfigs
        };
    }
    catch {
        return { ok: false };
    }
}
function stripJSONComments(text) {
    let out = "";
    let inString = false;
    let escaped = false;
    for (let i = 0; i < text.length; i++) {
        const ch = text[i];
        const next = i + 1 < text.length ? text[i + 1] : "";
        if (inString) {
            out += ch;
            if (escaped) {
                escaped = false;
            }
            else if (ch === "\\") {
                escaped = true;
            }
            else if (ch === "\"") {
                inString = false;
            }
            continue;
        }
        if (ch === "\"") {
            inString = true;
            out += ch;
            continue;
        }
        if (ch === "/" && next === "/") {
            while (i < text.length && text[i] !== "\n") {
                i++;
            }
            if (i < text.length && text[i] === "\n") {
                out += "\n";
            }
            continue;
        }
        if (ch === "/" && next === "*") {
            i += 2;
            while (i < text.length) {
                if (text[i] === "*" && i + 1 < text.length && text[i + 1] === "/") {
                    i++;
                    break;
                }
                i++;
            }
            continue;
        }
        out += ch;
    }
    return out;
}
function removeTrailingCommas(text) {
    let out = "";
    let inString = false;
    let escaped = false;
    for (let i = 0; i < text.length; i++) {
        const ch = text[i];
        if (inString) {
            out += ch;
            if (escaped) {
                escaped = false;
            }
            else if (ch === "\\") {
                escaped = true;
            }
            else if (ch === "\"") {
                inString = false;
            }
            continue;
        }
        if (ch === "\"") {
            inString = true;
            out += ch;
            continue;
        }
        if (ch === ",") {
            let j = i + 1;
            while (j < text.length && /\s/.test(text[j])) {
                j++;
            }
            if (j < text.length && (text[j] === "}" || text[j] === "]")) {
                continue;
            }
        }
        out += ch;
    }
    return out;
}
