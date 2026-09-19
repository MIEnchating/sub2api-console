import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import test from "node:test";

const workflow = readFileSync(new URL("../../workflows/release.yml", import.meta.url), "utf8");
const gate = workflow.match(
  /run: \|\n( +test "\$SOURCE_RESULT"[\s\S]*?)(?=\n {2}build-and-push:)/,
)?.[1];
assert.ok(gate, "release gate must exist");
const cases = [
  ["有效 CI 且回退跳过时放行", "success", "true", "skipped", true],
  ["完整回退检查成功时放行", "success", "false", "success", true],
  ["来源失败即使 CI 成功也阻止发布", "failure", "true", "skipped", false],
  ["来源取消时阻止发布", "cancelled", "false", "success", false],
  ["回退失败时阻止发布", "success", "false", "failure", false],
  ["回退取消时阻止发布", "success", "false", "cancelled", false],
  ["没有复用结果且回退跳过时阻止发布", "success", "", "skipped", false],
  ["复用 CI 但回退状态异常时阻止发布", "success", "true", "failure", false],
];
for (const [name, source, reuse, checks, allowed] of cases) {
  test(name, () => {
    const result = spawnSync("bash", ["-e", "-c", gate], {
      env: { ...process.env, SOURCE_RESULT: source, REUSE_CI: reuse, CHECKS_RESULT: checks },
    });
    assert.equal(result.status === 0, allowed);
  });
}
