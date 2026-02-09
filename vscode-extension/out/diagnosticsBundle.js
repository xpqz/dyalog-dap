"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.buildDiagnosticBundle = buildDiagnosticBundle;
exports.redactBundleValue = redactBundleValue;
const sensitiveKeyPattern = /(token|secret|password|passwd|api[-_]?key|authorization|cookie|credential|private[-_]?key|access[-_]?key|(?:^|[_-])pat(?:$|[_-])|pat$)/i;
const endpointKeyPattern = /(addr|endpoint|host)/i;
const pathKeyPattern = /(path|bin|launch|cwd|dir)/i;
function buildDiagnosticBundle(input) {
    const diagnostics = input.diagnostics.slice(-500);
    return {
        schemaVersion: "1",
        generatedAt: new Date().toISOString(),
        extension: {
            name: "dyalog-dap",
            version: input.extensionVersion
        },
        workspace: {
            name: input.workspaceName
        },
        diagnostics: {
            recent: diagnostics
        },
        environment: redactBundleValue(input.env),
        configSnapshot: redactBundleValue(input.configSnapshot),
        transcripts: {
            pointers: input.transcriptPointers
        }
    };
}
function redactBundleValue(value, key = "") {
    const keyText = key.toLowerCase();
    if (sensitiveKeyPattern.test(keyText)) {
        return "<redacted>";
    }
    if (Array.isArray(value)) {
        return value.map((item) => redactBundleValue(item, key));
    }
    if (isRecord(value)) {
        const out = {};
        for (const [childKey, childValue] of Object.entries(value)) {
            out[childKey] = redactBundleValue(childValue, childKey);
        }
        return out;
    }
    if (isPrimitive(value)) {
        if (typeof value === "string") {
            if (endpointKeyPattern.test(keyText)) {
                return "<redacted-endpoint>";
            }
            if (pathKeyPattern.test(keyText)) {
                return "<redacted-path>";
            }
        }
        return value;
    }
    return String(value);
}
function isPrimitive(value) {
    return (value === null ||
        typeof value === "string" ||
        typeof value === "number" ||
        typeof value === "boolean");
}
function isRecord(value) {
    return !!value && typeof value === "object" && !Array.isArray(value);
}
