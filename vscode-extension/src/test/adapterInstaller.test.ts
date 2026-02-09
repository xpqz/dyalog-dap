import test from "node:test";
import assert from "node:assert/strict";
import {
  checksumHex,
  ensureChecksumMatches,
  parseChecksums,
  selectAdapterAsset
} from "../adapterInstaller";

test("selectAdapterAsset picks archive for platform and arch", () => {
  const assets = [
    { name: "checksums.txt", browser_download_url: "x" },
    { name: "dyalog-dap_0.0.1_linux_amd64.tar.gz", browser_download_url: "x" },
    { name: "dyalog-dap_0.0.1_darwin_arm64.tar.gz", browser_download_url: "x" },
    { name: "dyalog-dap_0.0.1_windows_amd64.zip", browser_download_url: "x" }
  ];

  const darwin = selectAdapterAsset(assets, "darwin", "arm64");
  assert.equal(darwin?.name, "dyalog-dap_0.0.1_darwin_arm64.tar.gz");

  const linux = selectAdapterAsset(assets, "linux", "x64");
  assert.equal(linux?.name, "dyalog-dap_0.0.1_linux_amd64.tar.gz");

  const windows = selectAdapterAsset(assets, "win32", "x64");
  assert.equal(windows?.name, "dyalog-dap_0.0.1_windows_amd64.zip");
});

test("parseChecksums parses checksum map and verify succeeds/fails", () => {
  const content = Buffer.from("adapter bytes", "utf8");
  const checksum = checksumHex(content);
  const checksums = parseChecksums(
    `${checksum}  dyalog-dap_0.0.1_linux_amd64.tar.gz\n` +
      `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  checksums.txt\n`
  );

  ensureChecksumMatches("dyalog-dap_0.0.1_linux_amd64.tar.gz", content, checksums);

  assert.throws(() => {
    ensureChecksumMatches(
      "dyalog-dap_0.0.1_windows_amd64.zip",
      content,
      checksums
    );
  }, /missing checksum/i);

  assert.throws(() => {
    ensureChecksumMatches(
      "dyalog-dap_0.0.1_linux_amd64.tar.gz",
      Buffer.from("tampered", "utf8"),
      checksums
    );
  }, /checksum verification failed/i);
});
