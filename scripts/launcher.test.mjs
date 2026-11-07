import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import test from "node:test";

test("npm launcher executes the local binary", () => {
  const result = spawnSync(process.execPath, [join(import.meta.dirname, "..", "npm", "bin", "build-bouncer.cjs"), "--version"], {
    encoding: "utf8",
    timeout: 30000,
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /^build-bouncer /);
});
