import { readFileSync } from "node:fs";

const [report] = JSON.parse(readFileSync(process.argv[2], "utf8"));
if (!report || !Array.isArray(report.files)) {
  throw new Error("npm pack did not produce a file manifest");
}
const packed = new Set(report.files.map((file) => file.path));
for (const os of ["darwin", "linux", "windows"]) {
  for (const arch of ["amd64", "arm64"]) {
    const file = os === "windows" ? "build-bouncer.exe" : "build-bouncer";
    if (!packed.has(`platform/${os}_${arch}/${file}`)) {
      throw new Error(`npm package is missing ${os}/${arch}`);
    }
  }
}
if (!packed.has("bin/build-bouncer.cjs")) {
  throw new Error("npm package is missing its CLI entrypoint");
}
