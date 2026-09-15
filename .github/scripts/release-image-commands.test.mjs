import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const currentDigest = `sha256:${"a".repeat(64)}`;
const candidateDigest = `sha256:${"b".repeat(64)}`;

function run(script, scenario) {
  const directory = mkdtempSync(join(tmpdir(), "console-release-test-"));
  const output = join(directory, "output");
  const calls = join(directory, "calls");
  try {
    writeFileSync(output, "");
    writeFileSync(calls, "");
    writeFileSync(join(directory, "docker"), `#!${process.execPath}
const fs = require("node:fs");
const args = process.argv.slice(2);
const scenario = JSON.parse(process.env.RELEASE_SCENARIO);
fs.appendFileSync(process.env.RELEASE_CALLS, JSON.stringify(args) + "\\n");
if (args[2] === "create") {
  fs.writeFileSync(process.env.RELEASE_PROMOTED, "yes");
  process.exit(0);
}
const ref = args[3];
const latest = ref.endsWith(":latest");
if (scenario.registryError) { process.stderr.write("ERROR: unauthorized: authentication required"); process.exit(1); }
if ((latest && scenario.missingLatest) || (!latest && !ref.includes("@") && scenario.missingVersion)) {
  process.stderr.write("ERROR: registry.example/console:tag: not found"); process.exit(1);
}
const promoted = fs.existsSync(process.env.RELEASE_PROMOTED);
const previous = latest && !promoted;
const digest = previous ? "${currentDigest}" : "${candidateDigest}";
if (args[5].includes(".Manifest")) { console.log(JSON.stringify({ digest })); process.exit(0); }
const old = ref.includes("${currentDigest}");
const version = old ? (scenario.latestVersion || "v2026.09.13") : "v2026.09.14";
const revision = old ? "previous" : (scenario.candidateRevision || "candidate");
const platform = { config: { Labels: {
  "org.opencontainers.image.version": version,
  "org.opencontainers.image.revision": revision,
} } };
console.log(JSON.stringify({ "linux/amd64": platform, "linux/arm64": platform }));
`, { mode: 0o755 });
    const result = spawnSync(process.execPath, [new URL(`./${script}.mjs`, import.meta.url).pathname], {
      encoding: "utf8",
      timeout: 30_000,
      env: {
        ...process.env,
        PATH: `${directory}:${process.env.PATH}`,
        IMAGE: "registry.example/console",
        GITHUB_REF_NAME: "v2026.09.14",
        GITHUB_SHA: "candidate",
        GITHUB_OUTPUT: output,
        RELEASE_SCENARIO: JSON.stringify(scenario),
        RELEASE_CALLS: calls,
        RELEASE_PROMOTED: join(directory, "promoted"),
      },
    });
    return {
      ...result,
      output: readFileSync(output, "utf8"),
      calls: readFileSync(calls, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line)),
    };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

test("existing matching version is reused without rebuilding its published tag", () => {
  const result = run("prepare-release-image", {});
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.output, "reuse=true\n");
  assert.ok(result.calls.every((args) => args[2] === "inspect"));
});

test("missing version permits building while registry authentication failure fails closed", () => {
  const missing = run("prepare-release-image", { missingVersion: true });
  assert.equal(missing.status, 0, missing.stderr);
  assert.equal(missing.output, "reuse=false\n");
  const denied = run("prepare-release-image", { registryError: true });
  assert.notEqual(denied.status, 0);
  assert.equal(denied.output, "");
});

test("older candidate or reused version with another revision is rejected before build", () => {
  for (const scenario of [{ latestVersion: "v2026.09.15" }, { candidateRevision: "other" }]) {
    const result = run("prepare-release-image", scenario);
    assert.notEqual(result.status, 0);
    assert.equal(result.output, "");
  }
});

test("promotion uses the verified immutable digest and checks the resulting latest", () => {
  const result = run("promote-release-image", {});
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(result.calls.filter((args) => args[2] === "create"), [[
    "buildx", "imagetools", "create", "--tag", "registry.example/console:latest", `registry.example/console@${candidateDigest}`,
  ]]);
  assert.deepEqual(result.calls.at(-2).slice(0, 4), ["buildx", "imagetools", "inspect", "registry.example/console:latest"]);
});

test("first latest initialization and rollback attempts stop before any remote write", () => {
  for (const scenario of [{ missingLatest: true }, { latestVersion: "v2026.09.15" }]) {
    const result = run("promote-release-image", scenario);
    assert.notEqual(result.status, 0);
    assert.ok(result.calls.every((args) => args[2] === "inspect"));
  }
});
