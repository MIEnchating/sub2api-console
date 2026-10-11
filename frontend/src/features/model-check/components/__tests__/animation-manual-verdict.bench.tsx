// @vitest-environment node
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { afterAll, bench } from "vitest";
import type { AnimationResult, Task } from "@/api";
import { AnimationManualVerdictControls } from "../animation-manual-verdict";
import { collectAnimationTasks, selectAnimationTaskResult } from "../../lib/animation-task-results";

const rows: AnimationResult[] = Array.from({ length: 100 }, (_, index) => ({
  account_id: String(index),
  account_name: `账号 ${index}`,
  model: "test-model",
  request_id: `animation-${index}`,
  mode: "animation",
  status: "succeeded",
  duration_ms: 100,
  completed_at: "2026-10-09T00:00:00Z",
  html: `<html><body>${"生成内容".repeat(256)}</body></html>`,
  usage: { input_tokens: 10, output_tokens: 100 },
  manual_verdict: { verdict: "passed", updated_at: "2026-10-09T01:00:00Z", updated_by: "operator" },
}));
const task: Task = {
  id: "batch",
  skill: "sub2api-model-animation",
  operation: "account-model-combined",
  status: "succeeded",
  progress: 100,
  message: "检测完成",
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T01:00:00Z",
  result: {
    mode: "both",
    account_ids: rows.map((row) => row.account_id),
    animations: rows.flatMap((row) => [
      { ...row, mode: "precheck", request_id: `precheck-${row.account_id}` },
      row,
    ]),
  },
};
const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
client.setQueryData(["model-animation", "task", task.id], task);
afterAll(() => client.clear());
const controls = rows
  .slice(0, 20)
  .map((row) => (
    <AnimationManualVerdictControls
      key={row.account_id}
      compact
      result={{ ...row, source_task_id: task.id, source_task_status: task.status }}
    />
  ));

bench(
  "整批汇总后为 20 个控件分别取结果（原路径）",
  () => {
    for (const row of rows.slice(0, 20)) {
      const result = collectAnimationTasks([task], new Set()).results.get(row.account_id);
      if (result?.request_id !== row.request_id || result.manual_verdict?.verdict !== "passed")
        throw new Error("原路径未保留目标请求标记");
    }
  },
  { time: 1500, warmupTime: 500 },
);

bench(
  "按账号为 20 个控件选择结果",
  () => {
    for (const row of rows.slice(0, 20)) {
      const selection = selectAnimationTaskResult(task, row.account_id, row.request_id);
      if (
        selection.status !== "succeeded" ||
        selection.result?.manual_verdict?.verdict !== "passed"
      )
        throw new Error("按账号选择未保留目标请求标记");
    }
  },
  { time: 1500, warmupTime: 500 },
);

bench(
  "100 账号组合任务的 20 个可见手动标记控件",
  () => {
    const html = renderToStaticMarkup(
      <QueryClientProvider client={client}>{controls}</QueryClientProvider>,
    );
    if ((html.match(/aria-pressed="true"/g) ?? []).length !== 20)
      throw new Error("手动结论没有完整显示");
  },
  { time: 1500, warmupTime: 500 },
);
