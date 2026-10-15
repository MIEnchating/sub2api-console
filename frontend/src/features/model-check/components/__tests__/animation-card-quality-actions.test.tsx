import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";
import type { AnimationQualityReview, AnimationResult } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCardResult } from "../animation-card-result";

const result: AnimationResult = {
  source_task_id: "generation-1",
  source_task_status: "running",
  phase: "completed",
  account_id: "41",
  account_name: "测试账号",
  request_id: "original-1",
  model: "test-model",
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
  benchmark: "test",
  prediction: { status: "suspicious", score: null, benchmark: "test" },
  recheck_version: 3,
};
const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function setup(value: AnimationResult = result): void {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationCardResult result={value} retryDisabled onRetry={() => {}} />
    </QueryClientProvider>,
  );
}

it("批次仍运行时可从卡片键盘复核已完成单项，按最新版本提交并同步标签", async () => {
  let finished = false;
  let submitted = false;
  const writes: unknown[] = [];
  const fetcher = vi.fn(async (url: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      expect(url).toBe("/api/model-checks/animations/generation-1/quality/recheck");
      writes.push(JSON.parse(String(init.body)));
      submitted = true;
      return Response.json({ id: "review-1", status: "queued", message: "复核已排队" });
    }
    return Response.json({
      ...review,
      prediction: { ...review.prediction, status: finished ? "normal" : "suspicious" },
      recheck: submitted
        ? { task_id: "review-1", status: finished ? "succeeded" : "running", message: "正在复核" }
        : undefined,
    });
  });
  vi.stubGlobal("fetch", fetcher);
  setup();
  expect(fetcher).not.toHaveBeenCalled();
  const button = screen.getByRole("button", { name: "复核" });
  button.focus();
  await userEvent.setup().keyboard("{Enter}");
  await waitFor(() =>
    expect(writes).toEqual([{ account_id: "41", request_id: "original-1", version: 3 }]),
  );
  expect(screen.getByRole("button", { name: "正在复核" })).toBeDisabled();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  finished = true;
  await waitFor(() => expect(screen.getByLabelText("自动质量：正常")).toBeVisible(), {
    timeout: 4000,
  });
  expect(screen.getByRole("button", { name: "复核" })).toBeEnabled();
  expect(writes).toHaveLength(1);
});

it("读取最新复核版本期间禁止重复点击，读取失败提示一次且不提交", async () => {
  let finish!: (response: Response) => void;
  const fetcher = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  vi.stubGlobal("fetch", fetcher);
  setup();
  await userEvent.setup().click(screen.getByRole("button", { name: "复核" }));
  expect(screen.getByRole("button", { name: "正在复核" })).toBeDisabled();
  await act(async () =>
    finish(Response.json({ detail: "评审记录读取失败，请重试" }, { status: 503 })),
  );
  expect(await screen.findByText(/评审记录读取失败，请重试/)).toBeVisible();
  expect(screen.getByRole("button", { name: "复核" })).toBeEnabled();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("点击时已有活动复核则显示进度而不重复提交", async () => {
  const methods: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      methods.push(init?.method ?? "GET");
      return Response.json({
        ...review,
        recheck: { task_id: "active", status: "running", message: "正在复核" },
      });
    }),
  );
  setup();
  await userEvent.setup().click(screen.getByRole("button", { name: "复核" }));
  expect(await screen.findByRole("button", { name: "正在复核" })).toBeDisabled();
  expect(methods).toContain("GET");
  expect(methods).not.toContain("POST");
});

it.each([
  { phase: "reviewing" },
  { status: "failed" },
  { source_task_id: undefined },
  { source: undefined },
  { source_truncated: true },
  { mode: "precheck" },
] satisfies Partial<AnimationResult>[])("结果缺失或未完成时不显示卡片复核入口：%j", (change) => {
  setup({ ...result, ...change });
  expect(screen.queryByRole("button", { name: "复核" })).not.toBeInTheDocument();
});

it("卡片确认收录直接使用已展示的自动结果，保存后禁止重复收录", async () => {
  const prediction = {
    ...review.prediction,
    source: "model_review",
    status: "normal",
    explanation: "主体完整",
  } as const;
  const current = { ...review, prediction, prediction_sha256: "fingerprint" };
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        writes.push(JSON.parse(String(init.body)));
        return Response.json({
          ...current,
          annotation: {
            source: "confirmed_auto",
            label: "normal",
            reason: "主体完整",
            prediction_sha256: "fingerprint",
            version: 1,
          },
        });
      }
      return Response.json(current);
    }),
  );
  setup({ ...result, quality: prediction });
  await userEvent.setup().click(screen.getByRole("button", { name: "确认收录" }));
  expect(await screen.findByRole("button", { name: "已收录" })).toBeDisabled();
  expect(writes).toEqual([
    {
      account_id: "41",
      request_id: "original-1",
      version: 0,
      recheck_version: 3,
      prediction_sha256: "fingerprint",
    },
  ]);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("卡片确认时发现自动结论变化则更新展示并要求重新确认，不收录未看过的结论", async () => {
  const methods: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      methods.push(init?.method ?? "GET");
      return Response.json({
        ...review,
        prediction_sha256: "new",
        prediction: {
          ...review.prediction,
          source: "model_review",
          status: "normal",
          explanation: "新的结论",
        },
      });
    }),
  );
  setup({
    ...result,
    quality: { ...review.prediction, source: "model_review", explanation: "旧结论" },
  });
  await userEvent.setup().click(screen.getByRole("button", { name: "确认收录" }));
  expect(await screen.findByText(/评审结果已变化/)).toBeVisible();
  expect(methods).not.toContain("POST");
  expect(screen.getByLabelText("自动质量：正常")).toBeVisible();
});

it.each([
  ["normal", "正常", "不降智"],
  ["suspicious", "降智", "降智"],
  ["unknown", "无法判定", "未知"],
] as const)(
  "质量标注为 %s 时显示对应结论，旧手动判定不覆盖也不再提供编辑",
  (status, label, verdict) => {
    setup({
      ...result,
      quality: { ...review.prediction, status },
      manual_verdict: { verdict: "passed", updated_at: result.completed_at, updated_by: "legacy" },
    });
    expect(screen.getByLabelText(`自动质量：${label}`)).toHaveAttribute(
      "aria-description",
      verdict,
    );
    expect(
      screen.queryByRole("button", { name: /^(降智|不降智|清除标记)$/ }),
    ).not.toBeInTheDocument();
  },
);

it("人工质量纠正优先于自动结果，不受旧手动判定影响", () => {
  setup({
    ...result,
    quality: { ...review.prediction, status: "suspicious" },
    quality_annotation: {
      source: "manual",
      label: "normal",
      reason: "人工纠正",
      source_group: "group",
      version: 1,
      updated_at: result.completed_at,
      updated_by: "operator",
    },
  });
  expect(screen.getByLabelText("质量标注：正常")).toHaveAttribute("aria-description", "不降智");
  expect(screen.getByRole("button", { name: "已标注" })).toBeDisabled();
});

it("卡片收录与复核采用统一的 32px 无边框图标按钮，键盘聚焦可见中文提示", async () => {
  setup({
    ...result,
    quality: { ...review.prediction, source: "model_review", explanation: "主体完整" },
  });
  const user = userEvent.setup();
  await user.tab();
  for (const name of ["确认收录", "复核"]) {
    const button = screen.getByRole("button", { name });
    expect(button).toHaveAttribute("data-variant", "ghost");
    expect(button).toHaveClass("size-8");
    expect(button.textContent).toBe("");
    await user.tab();
    expect(button).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(name);
    expect(button).toHaveAccessibleDescription(name === "复核" ? "复核原动画质量" : name);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("tooltip")).not.toBeInTheDocument());
  }
});
