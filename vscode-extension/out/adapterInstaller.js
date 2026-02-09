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
exports.installAdapterFromLatestRelease = installAdapterFromLatestRelease;
exports.selectAdapterAsset = selectAdapterAsset;
exports.parseChecksums = parseChecksums;
exports.checksumHex = checksumHex;
exports.ensureChecksumMatches = ensureChecksumMatches;
const node_path_1 = __importDefault(require("node:path"));
const promises_1 = __importDefault(require("node:fs/promises"));
const node_crypto_1 = __importDefault(require("node:crypto"));
const tar = __importStar(require("tar"));
const AdmZip = require("adm-zip");
const githubOwner = "xpqz";
const githubRepo = "dyalog-dap";
async function installAdapterFromLatestRelease(params) {
    const fetchImpl = params.fetchImpl ?? fetch;
    const release = await getLatestRelease(fetchImpl);
    const archiveAsset = selectAdapterAsset(release.assets, params.platform, params.arch);
    if (!archiveAsset) {
        throw new Error(`No adapter archive found for platform=${params.platform} arch=${params.arch} in latest release.`);
    }
    const checksumsAsset = release.assets.find((asset) => asset.name === "checksums.txt");
    if (!checksumsAsset) {
        throw new Error("Latest release does not include checksums.txt.");
    }
    const [archiveBytes, checksumsText] = await Promise.all([
        downloadBytes(fetchImpl, archiveAsset.browser_download_url),
        downloadText(fetchImpl, checksumsAsset.browser_download_url)
    ]);
    ensureChecksumMatches(archiveAsset.name, archiveBytes, parseChecksums(checksumsText));
    const targetDir = node_path_1.default.join(params.installRoot, release.tag_name);
    await promises_1.default.rm(targetDir, { recursive: true, force: true });
    await promises_1.default.mkdir(targetDir, { recursive: true });
    const archivePath = node_path_1.default.join(targetDir, archiveAsset.name);
    await promises_1.default.writeFile(archivePath, archiveBytes);
    await extractArchive(archivePath, targetDir);
    await promises_1.default.unlink(archivePath).catch(() => undefined);
    const adapterName = expectedAdapterBinaryName(params.platform);
    const adapterPath = await findFileRecursive(targetDir, adapterName);
    if (adapterPath === "") {
        throw new Error(`Installed archive did not contain ${adapterName}.`);
    }
    if (params.platform !== "win32") {
        await promises_1.default.chmod(adapterPath, 0o755);
    }
    return {
        adapterPath,
        versionTag: release.tag_name,
        assetName: archiveAsset.name
    };
}
function selectAdapterAsset(assets, platform, arch) {
    const platformToken = toPlatformToken(platform);
    const archToken = toArchToken(arch);
    if (platformToken === "" || archToken === "") {
        return undefined;
    }
    const expectedSuffix = platformToken === "windows" ? ".zip" : ".tar.gz";
    return assets.find((asset) => asset.name.startsWith("dyalog-dap_") &&
        asset.name.includes(`_${platformToken}_${archToken}`) &&
        asset.name.endsWith(expectedSuffix));
}
function parseChecksums(text) {
    const output = new Map();
    const lines = text.split(/\r?\n/);
    for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed === "") {
            continue;
        }
        const parts = trimmed.split(/\s+/);
        if (parts.length < 2) {
            continue;
        }
        const checksum = parts[0].trim().toLowerCase();
        const filename = parts[parts.length - 1].trim().replace(/^\*/, "");
        if (checksum !== "" && filename !== "") {
            output.set(filename, checksum);
        }
    }
    return output;
}
function checksumHex(content) {
    return node_crypto_1.default.createHash("sha256").update(content).digest("hex").toLowerCase();
}
function ensureChecksumMatches(assetName, content, checksums) {
    const expected = checksums.get(assetName);
    if (!expected) {
        throw new Error(`Missing checksum entry for ${assetName}.`);
    }
    const actual = checksumHex(content);
    if (actual !== expected.toLowerCase()) {
        throw new Error(`Checksum verification failed for ${assetName}.`);
    }
}
function toPlatformToken(platform) {
    if (platform === "darwin") {
        return "darwin";
    }
    if (platform === "linux") {
        return "linux";
    }
    if (platform === "win32") {
        return "windows";
    }
    return "";
}
function toArchToken(arch) {
    if (arch === "x64") {
        return "amd64";
    }
    if (arch === "arm64") {
        return "arm64";
    }
    return "";
}
function expectedAdapterBinaryName(platform) {
    return platform === "win32" ? "dap-adapter.exe" : "dap-adapter";
}
async function getLatestRelease(fetchImpl) {
    const url = `https://api.github.com/repos/${githubOwner}/${githubRepo}/releases/latest`;
    const response = await fetchImpl(url, {
        headers: {
            Accept: "application/vnd.github+json",
            "User-Agent": "dyalog-dap-vscode-extension"
        }
    });
    if (!response.ok) {
        throw new Error(`Failed to query GitHub releases: ${response.status} ${response.statusText}`);
    }
    const data = (await response.json());
    if (typeof data.tag_name !== "string" || !Array.isArray(data.assets)) {
        throw new Error("Unexpected GitHub release payload.");
    }
    return {
        tag_name: data.tag_name,
        assets: data.assets.filter((asset) => {
            return (!!asset &&
                typeof asset === "object" &&
                typeof asset.name === "string" &&
                typeof asset.browser_download_url === "string");
        })
    };
}
async function downloadBytes(fetchImpl, url) {
    const response = await fetchImpl(url);
    if (!response.ok) {
        throw new Error(`Failed to download ${url}: ${response.status} ${response.statusText}`);
    }
    return new Uint8Array(await response.arrayBuffer());
}
async function downloadText(fetchImpl, url) {
    const response = await fetchImpl(url);
    if (!response.ok) {
        throw new Error(`Failed to download ${url}: ${response.status} ${response.statusText}`);
    }
    return response.text();
}
async function extractArchive(archivePath, targetDir) {
    if (archivePath.endsWith(".tar.gz")) {
        await tar.x({
            file: archivePath,
            cwd: targetDir
        });
        return;
    }
    if (archivePath.endsWith(".zip")) {
        const zip = new AdmZip(archivePath);
        zip.extractAllTo(targetDir, true);
        return;
    }
    throw new Error(`Unsupported archive format: ${archivePath}`);
}
async function findFileRecursive(root, targetName) {
    const entries = await promises_1.default.readdir(root, { withFileTypes: true });
    for (const entry of entries) {
        const fullPath = node_path_1.default.join(root, entry.name);
        if (entry.isFile() && entry.name === targetName) {
            return fullPath;
        }
        if (entry.isDirectory()) {
            const child = await findFileRecursive(fullPath, targetName);
            if (child !== "") {
                return child;
            }
        }
    }
    return "";
}
