import assert from "node:assert/strict";
import { Buffer } from "node:buffer";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import process from "node:process";
import { dirname, join } from "node:path";
import { fileURLToPath, URL } from "node:url";
import { test } from "node:test";

const checker = fileURLToPath(new URL("../check-coverage.mjs", import.meta.url));

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "console-audit-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const init = spawnSync("git", ["init", "--quiet", root], { encoding: "utf8" });
  assert.equal(init.status, 0, init.stderr);
  function write(path, content) {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), content);
  }
  const script = "docs/reviews/full-file-audit/check-coverage.mjs";
  write(script, "");
  copyFileSync(checker, join(root, script));
  function check() {
    const result = spawnSync(process.execPath, [script, "--write"], {
      cwd: root,
      encoding: "utf8",
    });
    assert.equal(result.stderr, "");
    return {
      status: result.status,
      report: JSON.parse(
        readFileSync(join(root, "docs/reviews/full-file-audit/coverage.json"), "utf8"),
      ),
    };
  }
  return { write, check };
}

test("构建备份和 Go 测试二进制不算待审源码，部署配置与命名相近的源码仍纳入", (t) => {
  const repo = fixture(t);
  const sources = [
    "frontend/src/features/dist-view.tsx",
    "frontend/e2e/deployment.e2e.ts",
    "frontend/dist-mapping.ts",
    "frontend/dist.previous-deploy-policy.ts",
    "backend/internal/example/fixture.test",
    "backend/Dockerfile",
    ".github/workflows/quality.yml",
  ];
  const artifacts = [
    "frontend/dist/static/index.js",
    "frontend/dist.previous-deploy-20261008T0420Z/static/index.js",
    "frontend/test-results/report.txt",
    "frontend/playwright-report/index.html",
  ];
  for (const path of [...sources, ...artifacts]) repo.write(path, "fixture source\n");
  repo.write("backend/business.test", Buffer.from([0x7f, 0x45, 0x4c, 0x46, 0, 0]));
  const { status, report } = repo.check();
  assert.equal(status, 1, "真实源码尚未审查时必须失败");
  assert.deepEqual(report.records.map((item) => item.path).sort(), sources.sort());
  assert.deepEqual(
    report.excluded.map((item) => item.path).sort(),
    [...artifacts, "backend/business.test"].sort(),
  );
});

test("有效逐文件记录通过，源码变化后必须重新审查且不会自动刷新记录", (t) => {
  const repo = fixture(t);
  const path = "frontend/src/example.ts";
  const content = "export const value = 1;\n";
  const hash = createHash("sha256").update(content).digest("hex");
  const ledger = `${path}\t${hash}\treviewed\tfixture assessment\n`;
  repo.write(path, content);
  repo.write("docs/reviews/full-file-audit/fixture.tsv", ledger);
  assert.equal(repo.check().status, 0);

  repo.write(path, "export const value = 2;\n");
  const changed = repo.check();
  assert.equal(changed.status, 1);
  assert.equal(changed.report.records[0].status, "changed");
  assert.equal(changed.report.counts.reviewed, 0);
  assert.equal(repo.check().report.records[0].status, "changed");
});
