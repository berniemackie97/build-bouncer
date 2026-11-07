import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

const dist = resolve(process.argv[2] ?? "dist");
const root = resolve(import.meta.dirname, "..");
const metadata = JSON.parse(readFileSync(join(dist, "metadata.json"), "utf8"));
const version = metadata.version;
if (typeof version !== "string" || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`invalid release version: ${version}`);
}

const manifest = readFileSync(join(dist, `build-bouncer_${version}_checksums.txt`), "utf8");
const checksums = new Map();
for (const line of manifest.split("\n")) {
  if (!line.trim()) continue;
  const match = /^([a-f0-9]{64})\s+\*?([^/\\]+)$/.exec(line.trim());
  if (!match || checksums.has(match[2])) throw new Error("invalid or repeated checksum entry");
  checksums.set(match[2], match[1]);
}

const targets = [
  ["darwin", "amd64"], ["darwin", "arm64"],
  ["linux", "amd64"], ["linux", "arm64"],
  ["windows", "amd64"], ["windows", "arm64"],
];

const packageRoot = join(root, "npm");
const platformRoot = join(packageRoot, "platform");
rmSync(platformRoot, { recursive: true, force: true });
for (const [os, arch] of targets) {
  const executable = os === "windows" ? "build-bouncer.exe" : "build-bouncer";
  const ext = os === "windows" ? ".zip" : ".tar.gz";
  const name = `build-bouncer_${version}_${os}_${arch}${ext}`;
  const source = join(dist, name);
  const expected = checksums.get(name);
  if (!expected || !existsSync(source) || !statSync(source).isFile()) {
    throw new Error(`missing release artifact or checksum: ${name}`);
  }
  const actual = createHash("sha256").update(readFileSync(source)).digest("hex");
  if (actual !== expected) throw new Error(`checksum mismatch: ${name}`);
  const destination = join(platformRoot, `${os}_${arch}`);
  mkdirSync(destination, { recursive: true });
  if (os === "windows") {
    execFileSync("unzip", ["-q", source, executable, "-d", destination]);
  } else {
    execFileSync("tar", ["-xzf", source, "-C", destination, executable]);
  }
  if (!statSync(join(destination, executable)).isFile()) {
    throw new Error(`missing executable in ${name}`);
  }
}

const packageFile = join(packageRoot, "package.json");
const pkg = JSON.parse(readFileSync(packageFile, "utf8"));
pkg.version = version;
pkg.private = false;
writeFileSync(packageFile, JSON.stringify(pkg, null, 2) + "\n");
if (existsSync(join(root, "LICENSE"))) {
  copyFileSync(join(root, "LICENSE"), join(packageRoot, "LICENSE"));
}
console.log(`Staged npm package ${pkg.name}@${version} for ${targets.length} targets`);
