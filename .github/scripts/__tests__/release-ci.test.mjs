import assert from "node:assert/strict";
import test from "node:test";
import { findReusableCI } from "../release-ci.mjs";

const sha = "a".repeat(40);
const repository = "owner/console";
const run = {
  id: 42,
  head_sha: sha,
  head_branch: "main",
  event: "push",
  status: "completed",
  conclusion: "success",
  repository: { full_name: repository },
};
function options(responses) {
  return {
    repository,
    sha,
    token: "test-token",
    attempts: 2,
    fetch: async () => ({ ok: true, json: async () => ({ workflow_runs: responses.shift() }) }),
    sleep: async () => {},
  };
}

test("同一提交 main push 的成功 CI 可以复用", async () => {
  assert.equal(await findReusableCI(options([[run]])), 42);
});
test("正在执行的同一提交 CI 完成后复用，不立即重复运行", async () => {
  assert.equal(
    await findReusableCI(options([[{ ...run, status: "in_progress", conclusion: null }], [run]])),
    42,
  );
});
for (const [name, change] of [
  ["其他提交", { head_sha: "b".repeat(40) }],
  ["PR", { event: "pull_request" }],
  ["其他分支", { head_branch: "feature" }],
  ["其他仓库", { repository: { full_name: "other/console" } }],
  ["失败", { conclusion: "failure" }],
  ["取消", { conclusion: "cancelled" }],
  ["跳过", { conclusion: "skipped" }],
]) {
  test(`${name} CI 不得作为发布成功依据`, async () => {
    assert.equal(await findReusableCI(options([[{ ...run, ...change }]])), null);
  });
}
test("最新重跑失败时不能复用较早的成功结果", async () => {
  assert.equal(
    await findReusableCI(options([[run, { ...run, id: 43, conclusion: "failure" }]])),
    null,
  );
});
test("没有 CI 或等待超时必须回退完整检查", async () => {
  assert.equal(await findReusableCI(options([[]])), null);
  const pending = { ...run, status: "queued", conclusion: null };
  assert.equal(await findReusableCI(options([[pending], [pending]])), null);
});
test("API 失败或响应异常必须回退完整检查", async () => {
  for (const fetch of [
    async () => {
      throw new Error("network");
    },
    async () => ({ ok: false }),
    async () => ({ ok: true, json: async () => ({}) }),
  ]) {
    assert.equal(await findReusableCI({ ...options([]), fetch }), null);
  }
});
