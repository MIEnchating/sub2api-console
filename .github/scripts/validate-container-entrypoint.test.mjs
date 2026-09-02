import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
const entrypoint = join(repositoryRoot, "backend", "docker-entrypoint.sh");
const recorder = `#!/bin/sh
printf '%s|%s\n' "$0" "$*" >>"$ENTRYPOINT_CALLS"
`;
const suExec = `${recorder}shift
exec "$@"
`;
const realpath = `${recorder}printf '%s\n' "$1"
`;

function runEntrypoint(socketPath, dataDirectory = "/var/lib/sub2api-test") {
  const directory = mkdtempSync(join(tmpdir(), "sub2api-entrypoint-test-"));
  const callsPath = join(directory, "calls.log");
  try {
    for (const command of ["mkdir", "chown", "chmod", "realpath", "su-exec"]) {
      const commandPath = join(directory, command);
      let implementation = recorder;
      if (command === "realpath") implementation = realpath;
      if (command === "su-exec") implementation = suExec;
      writeFileSync(commandPath, implementation);
      chmodSync(commandPath, 0o755);
    }
    const result = spawnSync("sh", [entrypoint, "true"], {
      encoding: "utf8",
      env: {
        ...process.env,
        PATH: `${directory}:${process.env.PATH}`,
        ENTRYPOINT_CALLS: callsPath,
        SUB2API_CONSOLE_DATA_DIR: dataDirectory,
        SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET: socketPath,
      },
    });
    return {
      ...result,
      calls: readFileSync(callsPath, "utf8"),
    };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

test("container entrypoint prepares an absolute socket parent directory", () => {
  const run = runEntrypoint("/run/sub2api-console/api.sock");

  assert.equal(run.status, 0, run.stderr);
  assert.match(run.calls, /mkdir\|-p \/run\/sub2api-console/);
  assert.match(run.calls, /chown\|console:console \/run\/sub2api-console/);
  assert.match(run.calls, /chmod\|0750 \/run\/sub2api-console/);
  assert.match(run.calls, /su-exec\|console true/);
});

test("container entrypoint rejects relative and root-level socket paths", () => {
  for (const socketPath of ["run/api.sock", "/api.sock"]) {
    const run = runEntrypoint(socketPath);

    assert.notEqual(run.status, 0);
    assert.match(run.stderr, /absolute file path with a non-root parent directory/);
    assert.doesNotMatch(run.calls, /mkdir\|-p \/run/);
  }
});

test("container entrypoint rejects a data directory resolving to the filesystem root", () => {
  const run = runEntrypoint("/run/sub2api-console/api.sock", "/");

  assert.notEqual(run.status, 0);
  assert.match(run.stderr, /DATA_DIR must not resolve to the filesystem root/);
  assert.doesNotMatch(run.calls, /chown\|-R console:console \/$/m);
  assert.doesNotMatch(run.calls, /su-exec/);
});

test("Docker build contexts exclude local environment files", () => {
  for (const directory of ["backend", "frontend"]) {
    const dockerignore = readFileSync(join(repositoryRoot, directory, ".dockerignore"), "utf8");
    assert.match(dockerignore, /^\.env\*$/m);
  }
});
