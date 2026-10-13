import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const ci = readFileSync(new URL("../../workflows/ci.yml", import.meta.url), "utf8");
const quality = readFileSync(new URL("../../workflows/quality.yml", import.meta.url), "utf8");

test("普通推送不触发 CI，面向 main 的 PR 创建、更新和关闭事件触发检查", () => {
  const events = ci.split("on:\n")[1].split("\npermissions:")[0];
  assert.doesNotMatch(events, /^ {2}push:/m);
  assert.match(events, /pull_request:\n {4}branches: \[main\]/);
  assert.match(events, /types: \[opened, synchronize, closed\]/);
});

test("PR 未合并就关闭时跳过质量检查，镜像预检仍依赖质量检查成功", () => {
  assert.match(
    ci,
    /checks:[\s\S]*?if: github.event.action != 'closed' \|\| github.event.pull_request.merged == true/,
  );
  assert.match(ci, /docker:[\s\S]*?needs: checks/);
  assert.doesNotMatch(ci, /always\(\)/);
});

test("PR 检查检出事件提交，合并后不随 main 后续推送变化", () => {
  for (const workflow of [ci, quality]) {
    const checkouts = workflow.match(/uses: actions\/checkout@[^\n]+\n(?: {8}[^\n]*\n)*/g) ?? [];
    assert.ok(checkouts.length > 0);
    for (const checkout of checkouts) {
      assert.match(checkout, /ref: \$\{\{ github.sha \}\}/);
    }
  }
});
