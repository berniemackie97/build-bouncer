#!/usr/bin/env node
"use strict";

const { spawn } = require("node:child_process");
const { join } = require("node:path");
const { existsSync } = require("node:fs");

const platforms = {
  "darwin-arm64": "darwin_arm64",
  "darwin-x64": "darwin_amd64",
  "linux-arm64": "linux_arm64",
  "linux-x64": "linux_amd64",
  "win32-arm64": "windows_arm64",
  "win32-x64": "windows_amd64",
};

const key = `${process.platform}-${process.arch}`;
const target = platforms[key];
if (!target) {
  console.error(`build-bouncer: unsupported platform ${key}`);
  process.exit(1);
}

const executable = join(__dirname, "..", "platform", target,
  process.platform === "win32" ? "build-bouncer.exe" : "build-bouncer");

if (!existsSync(executable)) {
  console.error(`build-bouncer: packaged executable missing for ${key}; reinstall the package`);
  process.exit(1);
}

const child = spawn(executable, process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
});
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    if (child.exitCode === null && child.signalCode === null) {
      child.kill(signal);
    }
  });
}
child.once("error", (err) => {
  console.error(`build-bouncer: cannot start executable: ${err.message}`);
  process.exitCode = 1;
});
child.once("exit", (code, signal) => {
  process.exitCode = code === null
    ? signal === "SIGINT" ? 130 : signal === "SIGTERM" ? 143 : 1
    : code;
});
