import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import type { AnimationQualityReview, AnimationResult } from "@/api";
import { AnimationQualityReviewPanel } from "../animation-quality-review";

const result: AnimationResult = {
  source_task_id: "task",
  account_id: "41",
  account_name: "动画账号",
  request_id: "request",
  model: "test",
  status: "succeeded",
  phase: "completed",
  source_task_status: "running",
  source: "<html>original</html>",
  html: "<html>preview</html>",
  prompt: "固定题目",
  duration_ms: 100,
  completed_at: "2026-10-15T00:00:00Z",
};
const review = {
  sample_id: "sample",
  source_sha256: "source",
  prompt_sha256: "prompt",
  benchmark: "test",
  recheck_version: 2,
  prediction_sha256: "review-hash",
  prediction: {
    status: "normal",
    source: "model_review",
    explanation: "主体与运动正常",
    score: null,
    benchmark: "test",
  },
} satisfies AnimationQualityReview & { prediction_sha256: string };
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
it("自动理由附带题目和原文证据时可直接核对，无需填写人工理由", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        ...review,
        prediction: {
          ...review.prediction,
          explanation: "车轮有旋转实现。仅依据源码，未运行或观看动画。",
          basis: [
            {
              requirement: "车轮转动",
              evidence: "wheel.style.transform = rotation",
              assessment: "每帧更新旋转角度，支持车轮转动要求。",
            },
          ],
        },
      }),
    ),
  );
  setup();
  await userEvent.setup().click(await screen.findByRole("button", { name: /原文依据/ }));
  expect(await screen.findByText("每帧更新旋转角度，支持车轮转动要求。")).toBeVisible();
  expect(screen.getByText("车轮转动")).toBeVisible();
  expect(screen.getByText("wheel.style.transform = rotation")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认结果并收录样本" })).toBeEnabled();
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
});
it("确认自动结果无需填写标签理由，按所见版本收录训练样本并禁止重复提交", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        expect(url).toBe("/api/model-checks/animations/task/quality/confirm");
        writes.push(JSON.parse(String(init.body)));
        return Response.json({
          ...review,
          annotation: {
            label: "normal",
            reason: review.prediction.explanation,
            source: "confirmed_auto",
            prediction_sha256: "review-hash",
            version: 1,
            source_group: "sha256:source",
            updated_by: "operator",
            updated_at: result.completed_at,
          },
        });
      }
      return Response.json(review);
    }),
  );
  setup();
  const user = userEvent.setup();
  const button = await screen.findByRole("button", { name: "确认结果并收录样本" });
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  button.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("button", { name: "已收录训练样本" })).toBeDisabled();
  expect(writes).toEqual([
    {
      account_id: "41",
      request_id: "request",
      version: 0,
      recheck_version: 2,
      prediction_sha256: "review-hash",
    },
  ]);
  expect(screen.getByText(/自动结果已确认/)).toBeVisible();
});
it.each([
  { ...review, prediction: { ...review.prediction, status: "unknown" } },
  { ...review, recheck: { task_id: "recheck", status: "running", message: "正在复核" } },
  { ...review, prediction: { ...review.prediction, explanation: "" } },
])("无法判定、缺失理由或正在复核时禁止确认收录", async (value) => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(value)),
  );
  setup();
  expect(await screen.findByRole("button", { name: "确认结果并收录样本" })).toBeDisabled();
});
it("确认发生版本冲突时提示重新读取且不显示收录成功", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "POST")
        return Response.json({ detail: "评审结果已变化，请重新读取后确认" }, { status: 409 });
      return Response.json(review);
    }),
  );
  setup();
  await userEvent.setup().click(await screen.findByRole("button", { name: "确认结果并收录样本" }));
  expect(await screen.findByText("评审结果已变化，请重新读取后确认")).toBeVisible();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "确认结果并收录样本" })).toBeEnabled(),
  );
  expect(screen.queryByRole("button", { name: "已收录训练样本" })).not.toBeInTheDocument();
});
it("已有人工纠正时明确显示更新样本，确认请求携带已有版本", async () => {
  const writes: unknown[] = [];
  const saved = {
    ...review,
    annotation: {
      label: "suspicious",
      reason: "人工理由",
      source_group: "group",
      version: 3,
      updated_by: "operator",
      updated_at: result.completed_at,
    },
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "POST") writes.push(JSON.parse(String(init.body)));
      return Response.json(saved);
    }),
  );
  setup();
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "使用自动结果更新样本" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ version: 3, prediction_sha256: "review-hash" });
});

it("收录请求未完成时显示等待并禁止再次确认", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "POST")
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      return Response.json(review);
    }),
  );
  setup();
  await userEvent.setup().click(await screen.findByRole("button", { name: "确认结果并收录样本" }));
  expect(await screen.findByRole("button", { name: "正在收录" })).toBeDisabled();
  await act(async () =>
    finish(Response.json({ detail: "存储暂时不可用，请重试" }, { status: 503 })),
  );
  expect(await screen.findByRole("button", { name: "确认结果并收录样本" })).toBeEnabled();
});

it("重新评审改变结果后已有样本不冒充已确认新结果", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        ...review,
        annotation: {
          source: "confirmed_auto",
          prediction_sha256: "older-review-hash",
          label: "suspicious",
          reason: "旧评审理由",
          version: 1,
          source_group: "group",
          updated_by: "operator",
          updated_at: result.completed_at,
        },
      }),
    ),
  );
  setup();
  expect(await screen.findByRole("button", { name: "使用自动结果更新样本" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "已收录训练样本" })).not.toBeInTheDocument();
  expect(screen.getByText("已标注：降智")).toBeVisible();
});
