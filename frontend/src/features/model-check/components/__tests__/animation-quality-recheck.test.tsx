import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";
import type { AnimationQualityReview, AnimationResult } from "@/api";
import { AnimationQualityReviewPanel } from "../animation-quality-review";

const result: AnimationResult = {
  source_task_id: "generation-1",
  source_task_status: "running",
  phase: "completed",
  account_id: "41",
  account_name: "测试动画",
  request_id: "original-1",
  model: "generation-model",
  status: "succeeded",
  html: "<html>preview</html>",
  source: "<html>original</html>",
  prompt: "骑车题",
  duration_ms: 100,
  completed_at: "2026-10-15T00:00:00Z",
};
const review: AnimationQualityReview = {
  sample_id: "sample-1",
  source_sha256: "source",
  prompt_sha256: "prompt",
  benchmark: "benchmark",
  prediction: {
    status: "suspicious",
    score: null,
    benchmark: "benchmark",
    explanation: "旧自动理由",
  },
  annotation: {
    label: "normal",
    reason: "人工确认主体正常",
    source_group: "group",
    version: 1,
    updated_by: "operator",
    updated_at: result.completed_at,
  },
  recheck_version: 0,
};
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function setup(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationQualityReviewPanel result={result} />
    </QueryClientProvider>,
  );
}

it("键盘提交复核时发送原请求身份，成功更新自动标签并保留人工草稿", async () => {
  const writes: unknown[] = [];
  let released: (() => void) | undefined;
  let submitted = false;
  const pending = new Promise<void>((resolve) => {
    released = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        expect(input).toBe("/api/model-checks/animations/generation-1/quality/recheck");
        writes.push(JSON.parse(String(init.body)));
        await pending;
        submitted = true;
        return Response.json({ id: "recheck-1", status: "queued", message: "质量复核已排队" });
      }
      if (submitted)
        return Response.json({
          ...review,
          prediction: { ...review.prediction, status: "normal", explanation: "新的评审理由" },
          recheck_version: 2,
          recheck: { task_id: "recheck-1", status: "succeeded", message: "质量复核完成" },
        });
      return Response.json(review);
    }),
  );
  setup();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "人工纠正" }));
  const reason = screen.getByLabelText("判断理由（必填）");
  await user.clear(reason);
  await user.type(reason, "尚未保存的补充判断");
  const button = screen.getByRole("button", { name: "重新评审" });
  button.focus();
  await user.keyboard("{Enter}");
  await waitFor(() => expect(button).toBeDisabled());
  expect(writes).toEqual([{ account_id: "41", request_id: "original-1", version: 0 }]);
  expect(screen.getByText("旧自动理由")).toBeVisible();
  released?.();
  expect(await screen.findByText("新的评审理由")).toBeVisible();
  expect(screen.getByLabelText(/^自动质量/)).toHaveTextContent("正常");
  expect(reason).toHaveValue("尚未保存的补充判断");
  expect(screen.getByRole("button", { name: "重新评审" })).toBeEnabled();
});

it("重新打开页面时恢复正在复核状态并轮询至完成", async () => {
  let reads = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      reads++;
      return Response.json({
        ...review,
        recheck_version: 1,
        recheck: {
          task_id: "recheck-1",
          status: reads === 1 ? "running" : "succeeded",
          message: reads === 1 ? "正在复核原始动画" : "质量复核完成",
        },
      });
    }),
  );
  setup();
  expect(await screen.findByRole("button", { name: "正在复核" })).toBeDisabled();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole("button", { name: "重新评审" })).toBeEnabled(), {
    timeout: 4000,
  });
});

it("复核启动失败时提示原因并保留原自动结果供重试", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: string, init?: RequestInit) =>
      init?.method === "POST"
        ? Response.json({ detail: "请先选择评审账号和模型" }, { status: 422 })
        : Response.json(review),
    ),
  );
  setup();
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "重新评审" }));
  expect(await screen.findByText(/请先选择评审账号和模型/)).toBeVisible();
  expect(screen.getByText("旧自动理由")).toBeVisible();
  expect(screen.getByRole("button", { name: "重新评审" })).toBeEnabled();
});

it("复核任务失败时显示任务原因且不自动再次提交", async () => {
  const fetcher = vi.fn(async () =>
    Response.json({
      ...review,
      recheck_version: 1,
      recheck: {
        task_id: "recheck-1",
        status: "failed",
        message: "评审超时，上一次自动结果已保留",
      },
    }),
  );
  vi.stubGlobal("fetch", fetcher);
  setup();
  expect(await screen.findByText("评审超时，上一次自动结果已保留")).toBeVisible();
  expect(screen.getByRole("button", { name: "重新评审" })).toBeEnabled();
  expect(screen.getByText("旧自动理由")).toBeVisible();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("自动评审超时显示失败原因，重新评审成功后更新为正常且不重新生成动画", async () => {
  const writes: string[] = [];
  let retried = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        writes.push(url);
        retried = true;
        return Response.json({
          id: "retry-after-timeout",
          status: "queued",
          message: "质量复核已排队",
        });
      }
      return Response.json({
        ...review,
        annotation: undefined,
        prediction: retried
          ? {
              status: "normal",
              score: null,
              benchmark: "benchmark",
              source: "model_review",
              explanation: "本次评审完成，主要运动具备实现依据。",
            }
          : {
              status: "unknown",
              score: null,
              benchmark: "benchmark",
              source: "model_review",
              reason: "review_request_timeout",
            },
        recheck: retried
          ? { task_id: "retry-after-timeout", status: "succeeded", message: "质量复核完成" }
          : undefined,
      });
    }),
  );
  setup();
  const user = userEvent.setup();
  expect(await screen.findByLabelText("自动质量：评审失败")).toBeVisible();
  expect(screen.getByRole("heading", { name: "失败原因" })).toBeVisible();
  expect(screen.queryByRole("heading", { name: "判断理由" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认结果并收录样本" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "重新评审" }));
  expect(await screen.findByLabelText("自动质量：正常")).toBeVisible();
  expect(screen.getByRole("heading", { name: "判断理由" })).toBeVisible();
  expect(writes).toEqual(["/api/model-checks/animations/generation-1/quality/recheck"]);
});
