import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";

const script = new URL("../../backend/scripts/check-go.sh", import.meta.url);

for (const mode of ["vet", "test"]) {
  test(`${mode} discovers nested regression packages and preserves Go failures`, (t) => {
    const root = mkdtempSync(join(tmpdir(), "console-go-check-"));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    const packages = ["./internal/accounts/__tests__", "./internal/browser/proxy/__tests__"];
    for (const path of ["scripts", "bin", ...packages, "internal/empty/__tests__"]) {
      mkdirSync(join(root, path), { recursive: true });
    }
    for (const path of packages) {
      writeFileSync(join(root, path, "behavior_test.go"), "package regression_test\n");
      writeFileSync(join(root, path, "boundary_test.go"), "package regression_test\n");
    }
    cpSync(script, join(root, "scripts/check-go.sh"));
    const capture = join(root, "arguments.json");
    writeFileSync(
      join(root, "bin/go"),
      `#!${process.execPath}\n` +
        'require("node:fs").writeFileSync(process.env.GO_CHECK_CAPTURE, JSON.stringify(process.argv.slice(2)));\n' +
        "process.exit(Number(process.env.GO_CHECK_EXIT || 0));\n",
      { mode: 0o755 },
    );
    const env = {
      ...process.env,
      PATH: `${join(root, "bin")}:${process.env.PATH}`,
      GO_CHECK_CAPTURE: capture,
    };
    // Invoke from outside the backend so local and CI entry points resolve the same packages.
    execFileSync("bash", [join(root, "scripts/check-go.sh"), mode], { cwd: dirname(root), env });
    const args = JSON.parse(readFileSync(capture, "utf8"));
    assert.deepEqual(args, [
      ...(mode === "test" ? ["test", "-race"] : ["vet"]),
      "./...",
      ...packages,
    ]);
    const failed = spawnSync("bash", [join(root, "scripts/check-go.sh"), mode], {
      env: { ...env, GO_CHECK_EXIT: "7" },
    });
    assert.equal(failed.status, 7);
  });
}

for (const filename of ["quality.yml"]) {
  test(`${filename} requires discovered regression packages for vet and race tests`, () => {
    const workflow = readFileSync(new URL(`../workflows/${filename}`, import.meta.url), "utf8");
    for (const mode of ["vet", "test"]) {
      assert.ok(
        workflow.includes(`bash scripts/check-go.sh ${mode}`),
        `missing ${mode} package discovery`,
      );
    }
  });
}
