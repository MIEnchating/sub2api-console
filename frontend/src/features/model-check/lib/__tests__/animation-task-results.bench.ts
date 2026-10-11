// @vitest-environment node
import { bench } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { collectAnimationTasks } from "../animation-task-results";

const tasks: Task[] = Array.from({ length: 100 }, (_, batch) => ({
  id: `batch-${batch}`,
  skill: "sub2api-model-animation",
  operation: "account-model-combined",
  status: batch < 90 ? "succeeded" : "running",
  progress: batch < 90 ? 100 : 50,
  message: "组合检测",
  created_at: new Date(1791504000000 + batch * 1000).toISOString(),
  updated_at: "2026-10-09T01:00:00Z",
  result: {
    mode: "both",
    account_ids: Array.from({ length: 20 }, (_, account) => String(batch * 20 + account)),
    animations: Array.from({ length: 20 }, (_, account): AnimationResult[] => {
      const row: AnimationResult = {
        account_id: String(batch * 20 + account),
        account_name: `账号 ${batch * 20 + account}`,
        model: "test-model",
        request_id: `request-${batch}-${account}`,
        status: "succeeded",
        duration_ms: 100,
        completed_at: "2026-10-09T01:00:00Z",
      };
      return [
        {
          ...row,
          mode: "precheck",
          precheck: {
            verdict: "passed",
            profile_version: "test-profile",
            questions: [
              { id: "candy", verdict: "passed", request_id: row.request_id, answer: "21" },
            ],
          },
        },
        {
          ...row,
          mode: "animation",
          phase: batch < 90 ? "completed" : "generating",
          svg: `<svg><text>${"动画结果".repeat(256)}</text></svg>`,
          usage: { input_tokens: 10, output_tokens: 100 },
        },
      ];
    }).flat(),
  },
}));
const submitting = new Set<string>();

bench(
  "100 个组合任务各 20 个账号汇总 4000 条结果",
  () => {
    const state = collectAnimationTasks(tasks, submitting);
    if (
      state.results.size !== 1800 ||
      state.precheckResults.size !== 2000 ||
      state.busyIDs.size !== 200
    ) {
      throw new Error("组合任务汇总丢失结果或活动账号");
    }
  },
  { time: 1500, warmupTime: 500 },
);
