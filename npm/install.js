#!/usr/bin/env node
// Fetch the ilk binary matching this platform from the GitHub release that
// corresponds to this package's version, and verify it against the release
// checksums before installing it.
//
// ilk is a Go binary; npm is one of several ways to get it. `bin/ilk` is a shim
// that execs whatever this script places in `bin/ilk-bin`.

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const crypto = require("node:crypto");
const zlib = require("node:zlib");
const { execFileSync } = require("node:child_process");

const REPO = "coflounder/ilk";
const { version } = require("./package.json");

const PLATFORMS = { darwin: "darwin", linux: "linux" };
const ARCHES = { x64: "amd64", arm64: "arm64" };

function fail(message, hint) {
  console.error(`\n@ilk/cli: ${message}`);
  if (hint) console.error(`  ${hint}`);
  console.error("");
  process.exit(1);
}

async function download(url, { redirects = 0 } = {}) {
  if (redirects > 10) throw new Error(`too many redirects for ${url}`);
  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok) {
    throw new Error(`GET ${url} → ${res.status} ${res.statusText}`);
  }
  return Buffer.from(await res.arrayBuffer());
}

// Extract a single named file from a gzipped tar archive. Bundling a tar library
// for one file in a postinstall script is not worth the dependency.
function extractFromTarGz(buffer, wanted) {
  const tar = zlib.gunzipSync(buffer);
  let offset = 0;
  while (offset + 512 <= tar.length) {
    const header = tar.subarray(offset, offset + 512);
    const name = header.subarray(0, 100).toString("utf8").replace(/\0.*$/, "");
    if (name === "") break; // end-of-archive marker
    const sizeField = header.subarray(124, 136).toString("utf8").replace(/\0.*$/, "").trim();
    const size = parseInt(sizeField, 8) || 0;
    const start = offset + 512;
    if (name === wanted || name === `./${wanted}`) {
      return tar.subarray(start, start + size);
    }
    offset = start + Math.ceil(size / 512) * 512;
  }
  return null;
}

async function main() {
  const platform = PLATFORMS[os.platform()];
  const arch = ARCHES[os.arch()];

  if (!platform || !arch) {
    fail(
      `no prebuilt binary for ${os.platform()}/${os.arch()}`,
      `Install from source instead: go install github.com/${REPO}/cmd/ilk@v${version}`
    );
  }

  const archive = `ilk_${version}_${platform}_${arch}.tar.gz`;
  const base = `https://github.com/${REPO}/releases/download/v${version}`;

  let payload;
  try {
    payload = await download(`${base}/${archive}`);
  } catch (err) {
    fail(
      `could not download ${archive}: ${err.message}`,
      `Install from source instead: go install github.com/${REPO}/cmd/ilk@v${version}`
    );
  }

  // A checksum mismatch is fatal. A missing checksums file is not — releases
  // before checksums existed, and mirrors, should still install.
  try {
    const checksums = (await download(`${base}/checksums.txt`)).toString("utf8");
    const line = checksums.split("\n").find((l) => l.trim().endsWith(archive));
    if (line) {
      const expected = line.trim().split(/\s+/)[0];
      const actual = crypto.createHash("sha256").update(payload).digest("hex");
      if (expected !== actual) {
        fail(`checksum mismatch for ${archive}`, "Refusing to install. This may be a corrupted download.");
      }
    }
  } catch {
    // No checksums published for this release; continue.
  }

  const binary = extractFromTarGz(payload, "ilk");
  if (!binary || binary.length === 0) {
    fail(`${archive} did not contain an ilk binary`);
  }

  const dest = path.join(__dirname, "bin", "ilk-bin");
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.writeFileSync(dest, binary, { mode: 0o755 });

  try {
    const out = execFileSync(dest, ["--version"], { encoding: "utf8" }).trim();
    console.log(`@ilk/cli: installed ${out}`);
  } catch {
    console.log(`@ilk/cli: installed ilk ${version}`);
  }
}

main().catch((err) => fail(err.message));
