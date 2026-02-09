import path from "node:path";
import fs from "node:fs/promises";
import crypto from "node:crypto";
import * as tar from "tar";

const AdmZip = require("adm-zip") as {
  new (archivePath: string): { extractAllTo(targetPath: string, overwrite?: boolean): void };
};

export type ReleaseAsset = {
  name: string;
  browser_download_url: string;
};

type ReleaseResponse = {
  tag_name: string;
  assets: ReleaseAsset[];
};

type FetchLike = (input: string, init?: RequestInit) => Promise<Response>;

export type InstallAdapterResult = {
  adapterPath: string;
  versionTag: string;
  assetName: string;
};

const githubOwner = "xpqz";
const githubRepo = "dyalog-dap";

export async function installAdapterFromLatestRelease(params: {
  installRoot: string;
  platform: NodeJS.Platform;
  arch: string;
  fetchImpl?: FetchLike;
}): Promise<InstallAdapterResult> {
  const fetchImpl = params.fetchImpl ?? fetch;
  const release = await getLatestRelease(fetchImpl);
  const archiveAsset = selectAdapterAsset(release.assets, params.platform, params.arch);
  if (!archiveAsset) {
    throw new Error(
      `No adapter archive found for platform=${params.platform} arch=${params.arch} in latest release.`
    );
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

  const targetDir = path.join(params.installRoot, release.tag_name);
  await fs.rm(targetDir, { recursive: true, force: true });
  await fs.mkdir(targetDir, { recursive: true });
  const archivePath = path.join(targetDir, archiveAsset.name);
  await fs.writeFile(archivePath, archiveBytes);
  await extractArchive(archivePath, targetDir);
  await fs.unlink(archivePath).catch(() => undefined);

  const adapterName = expectedAdapterBinaryName(params.platform);
  const adapterPath = await findFileRecursive(targetDir, adapterName);
  if (adapterPath === "") {
    throw new Error(`Installed archive did not contain ${adapterName}.`);
  }
  if (params.platform !== "win32") {
    await fs.chmod(adapterPath, 0o755);
  }
  return {
    adapterPath,
    versionTag: release.tag_name,
    assetName: archiveAsset.name
  };
}

export function selectAdapterAsset(
  assets: ReleaseAsset[],
  platform: NodeJS.Platform,
  arch: string
): ReleaseAsset | undefined {
  const platformToken = toPlatformToken(platform);
  const archToken = toArchToken(arch);
  if (platformToken === "" || archToken === "") {
    return undefined;
  }
  const expectedSuffix = platformToken === "windows" ? ".zip" : ".tar.gz";
  return assets.find(
    (asset) =>
      asset.name.startsWith("dyalog-dap_") &&
      asset.name.includes(`_${platformToken}_${archToken}`) &&
      asset.name.endsWith(expectedSuffix)
  );
}

export function parseChecksums(text: string): Map<string, string> {
  const output = new Map<string, string>();
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

export function checksumHex(content: Uint8Array): string {
  return crypto.createHash("sha256").update(content).digest("hex").toLowerCase();
}

export function ensureChecksumMatches(
  assetName: string,
  content: Uint8Array,
  checksums: Map<string, string>
): void {
  const expected = checksums.get(assetName);
  if (!expected) {
    throw new Error(`Missing checksum entry for ${assetName}.`);
  }
  const actual = checksumHex(content);
  if (actual !== expected.toLowerCase()) {
    throw new Error(`Checksum verification failed for ${assetName}.`);
  }
}

function toPlatformToken(platform: NodeJS.Platform): string {
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

function toArchToken(arch: string): string {
  if (arch === "x64") {
    return "amd64";
  }
  if (arch === "arm64") {
    return "arm64";
  }
  return "";
}

function expectedAdapterBinaryName(platform: NodeJS.Platform): string {
  return platform === "win32" ? "dap-adapter.exe" : "dap-adapter";
}

async function getLatestRelease(fetchImpl: FetchLike): Promise<ReleaseResponse> {
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
  const data = (await response.json()) as Partial<ReleaseResponse>;
  if (typeof data.tag_name !== "string" || !Array.isArray(data.assets)) {
    throw new Error("Unexpected GitHub release payload.");
  }
  return {
    tag_name: data.tag_name,
    assets: data.assets.filter((asset): asset is ReleaseAsset => {
      return (
        !!asset &&
        typeof asset === "object" &&
        typeof (asset as ReleaseAsset).name === "string" &&
        typeof (asset as ReleaseAsset).browser_download_url === "string"
      );
    })
  };
}

async function downloadBytes(fetchImpl: FetchLike, url: string): Promise<Uint8Array> {
  const response = await fetchImpl(url);
  if (!response.ok) {
    throw new Error(`Failed to download ${url}: ${response.status} ${response.statusText}`);
  }
  return new Uint8Array(await response.arrayBuffer());
}

async function downloadText(fetchImpl: FetchLike, url: string): Promise<string> {
  const response = await fetchImpl(url);
  if (!response.ok) {
    throw new Error(`Failed to download ${url}: ${response.status} ${response.statusText}`);
  }
  return response.text();
}

async function extractArchive(archivePath: string, targetDir: string): Promise<void> {
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

async function findFileRecursive(root: string, targetName: string): Promise<string> {
  const entries = await fs.readdir(root, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(root, entry.name);
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
