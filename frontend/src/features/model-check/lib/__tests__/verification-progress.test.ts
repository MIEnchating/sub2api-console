import { expect, it } from "vitest";
import { verificationProgressProbes } from "../verification-progress";

it("同一题目在不同轮次执行时分别保留完成结果与当前请求", () => {
  const first = { id: "arithmetic", round: 1, status: "passed", answer: "**21**" };
  const current = { id: "arithmetic", round: 2, status: "running", request_id: "second-round" };
  const probes = verificationProgressProbes(
    {
      probe_plan: [
        { id: "arithmetic", round: 1 },
        { id: "arithmetic", round: 2 },
        { id: "structured", round: 2 },
      ],
      probes: [first],
      current_probe: current,
    },
    true,
  );
  expect(probes).toEqual([first, current, { id: "structured", round: 2, status: "pending" }]);
});

it("组合提前终止时将剩余计划标为未执行且不展示过期的运行状态", () => {
  const failure = { id: "arithmetic", round: 1, status: "error", error_code: "rate_limited" };
  const probes = verificationProgressProbes(
    {
      probe_plan: [
        { id: "arithmetic", round: 1 },
        { id: "constraints", round: 1 },
      ],
      probes: [failure],
      current_probe: { id: "constraints", round: 1, status: "running" },
    },
    false,
  );
  expect(probes).toEqual([failure, { id: "constraints", round: 1, status: "skipped" }]);
});

it("历史快照没有检测计划时保留已记录的回答且不生成等待项", () => {
  const completed = { id: "arithmetic", round: 1, status: "passed", answer: "21" };
  expect(verificationProgressProbes({ probes: [completed] }, true)).toEqual([completed]);
  expect(verificationProgressProbes({}, true)).toEqual([]);
  expect(verificationProgressProbes({ probes: [null, "invalid", completed] }, true)).toEqual([
    completed,
  ]);
});
