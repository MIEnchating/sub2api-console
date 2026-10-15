import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";
import type { AnimationQualityReview, AnimationResult } from "@/api";
import { AnimationQualityReviewPanel } from "../animation-quality-review";
import { AnimationQualityBadge } from "../animation-quality-badge";
import { AnimationPreview } from "../animation-preview";

const result: AnimationResult = {
  source_task_id: "task-1",
  source_task_status: "succeeded",
  account_id: "41",
  account_name: "动画账号",
  model: "test-model",
  request_id: "request-1",
  status: "succeeded",
  html: "<html><body>preview</body></html>",
  source: "<html>original</html>",
  prompt: "固定题目",
  duration_ms: 1000,
  completed_at: "2026-10-14T00:00:00Z",
};
const review: AnimationQualityReview = {
  prediction: {
    status: "unknown",
    score: null,
    reason: "model_unavailable",
    benchmark: "pelican-v1",
  },
  sample_id: "sample-1",
  benchmark: "pelican-v1",
  source_sha256: "source-hash",
  prompt_sha256: "prompt-hash",
};
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function setup(value = result, preview = false): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      {preview ? (
        <AnimationPreview result={value} />
      ) : (
        <AnimationQualityReviewPanel result={value} />
      )}
    </QueryClientProvider>,
  );
}
it.each(["normal", "suspicious", "unknown"] as const)(
  "自动标签 %s 显示文字及可访问名称",
  (status) => {
    render(<AnimationQualityBadge prediction={{ ...review.prediction, status }} />);
    expect(screen.getByLabelText(/^自动质量：/)).toHaveTextContent(
      { normal: "正常", suspicious: "降智", unknown: "无法判定" }[status],
    );
  },
);
it("历史记录没有自动结果时显示未检测，不补造分类", () => {
  render(<AnimationQualityBadge />);
  expect(screen.getByText("质量：未检测")).toBeVisible();
});
it("键盘聚焦自动标签时显示无法判定的原因，Escape 关闭提示", async () => {
  const user = userEvent.setup();
  render(<AnimationQualityBadge prediction={review.prediction} />);
  await user.tab();
  expect(screen.getByLabelText("自动质量：无法判定")).toHaveFocus();
  expect(await screen.findByRole("tooltip")).toHaveTextContent("模型未配置，先积累人工标注样本");
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("tooltip")).not.toBeInTheDocument());
});
it.each([
  { ...result, source: undefined },
  { ...result, prompt: undefined },
  { ...result, source_truncated: true },
  { ...result, source_task_id: undefined },
])("不完整历史结果禁止进入标注且不发请求", (value) => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  setup(value);
  expect(screen.getByText(/无法作为质量标注样本/)).toBeVisible();
  expect(screen.queryByRole("button", { name: "保存质量标注" })).not.toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});
it("批次仍在运行时已完成的单项可以立即保存质量标注", async () => {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body));
        writes.push(body);
        return Response.json({
          ...review,
          annotation: {
            ...body,
            updated_by: "operator",
            updated_at: result.completed_at,
            version: 1,
          },
        });
      }
      return Response.json(review);
    }),
  );
  setup({ ...result, phase: "completed", source_task_status: "running" });
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "人工纠正" }));
  await user.click(screen.getByRole("radio", { name: "正常" }));
  await user.type(screen.getByLabelText("判断理由（必填）"), "主体和踩踏正常");
  await user.click(screen.getByRole("button", { name: "保存质量标注" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({ request_id: "request-1", account_id: "41", label: "normal" });
});
it("单项仍在评审时显示质量加载状态且不读取标注", () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  setup({ ...result, phase: "reviewing", source_task_status: "running" });
  expect(screen.getByRole("status", { name: "质量标注中" })).toHaveAttribute("aria-busy", "true");
  expect(fetch).not.toHaveBeenCalled();
});
it("读取期间显示局部加载，失败后提供重试，成功显示模型未配置", async () => {
  const user = userEvent.setup();
  let resolve!: (response: Response) => void;
  const fetch = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    )
    .mockResolvedValueOnce(Response.json(review));
  vi.stubGlobal("fetch", fetch);
  setup();
  expect(screen.getByText("正在读取质量标注")).toBeVisible();
  expect(screen.queryByRole("button", { name: "保存质量标注" })).not.toBeInTheDocument();
  await act(async () => {
    resolve(Response.json({ detail: "读取失败" }, { status: 500 }));
  });
  await user.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("模型未配置，先积累人工标注样本")).toBeVisible();
  expect(screen.queryByText(/模型得分：/)).not.toBeInTheDocument();
});
it("必填理由通过后按任务和请求保存人工标签，独立于自动标签", async () => {
  const user = userEvent.setup();
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method !== "PUT") return Response.json(review);
      const body = JSON.parse(String(init.body)) as Record<string, unknown>;
      bodies.push(body);
      return Response.json({
        ...review,
        annotation: {
          ...body,
          version: 1,
          updated_at: result.completed_at,
          updated_by: "operator",
        },
      });
    }),
  );
  setup();
  await user.click(await screen.findByRole("button", { name: "人工纠正" }));
  await user.click(screen.getByRole("button", { name: "保存质量标注" }));
  expect(await screen.findByText("请填写可复核的判断理由")).toBeVisible();
  expect(screen.getByLabelText("判断理由（必填）")).toHaveAttribute("aria-invalid", "true");
  expect(bodies).toEqual([]);
  const normal = screen.getByRole("radio", { name: "正常" });
  normal.focus();
  await user.keyboard(" ");
  expect(normal).toBeChecked();
  await user.type(screen.getByLabelText("判断理由（必填）"), "车轮转动连续，结构完整");
  await user.type(screen.getByLabelText("样本分组（可选）"), "模板甲");
  await user.click(screen.getByRole("button", { name: "保存质量标注" }));
  expect(await screen.findByText(/已标注：正常/)).toBeVisible();
  expect(bodies).toEqual([
    {
      account_id: "41",
      request_id: "request-1",
      label: "normal",
      reason: "车轮转动连续，结构完整",
      source_group: "模板甲",
      version: 0,
    },
  ]);
  expect(screen.getByLabelText("自动质量：无法判定")).toBeVisible();
});
it("并发版本冲突保留输入并允许重新读取，不覆盖别人的标注", async () => {
  const user = userEvent.setup();
  let reads = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === "PUT")
        return Response.json({ detail: "质量标注已被修改，请重新读取后保存" }, { status: 409 });
      reads++;
      return Response.json(
        reads === 1
          ? review
          : {
              ...review,
              annotation: {
                label: "normal",
                reason: "另一位标注者的理由",
                source_group: "group",
                version: 2,
                updated_at: result.completed_at,
                updated_by: "other",
              },
            },
      );
    }),
  );
  setup();
  await user.click(await screen.findByRole("button", { name: "人工纠正" }));
  await user.type(screen.getByLabelText("判断理由（必填）"), "我的判断理由");
  await user.click(screen.getByRole("button", { name: "保存质量标注" }));
  expect(await screen.findByText("质量标注已被修改，请重新读取后保存")).toBeVisible();
  expect(screen.getByLabelText("判断理由（必填）")).toHaveValue("我的判断理由");
  await user.click(screen.getByRole("button", { name: "重新读取标注" }));
  await waitFor(() =>
    expect(screen.getByLabelText("判断理由（必填）")).toHaveValue("另一位标注者的理由"),
  );
});
it("打开质量页签才读取标注，键盘切换后可返回动画预览", async () => {
  const user = userEvent.setup();
  const fetch = vi.fn(async () => Response.json(review));
  vi.stubGlobal("fetch", fetch);
  setup(result, true);
  await user.click(screen.getByRole("button", { name: "放大查看 动画账号 的动画" }));
  expect(fetch).not.toHaveBeenCalled();
  const qualityTab = screen.getByRole("tab", { name: "质量标注" });
  qualityTab.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("button", { name: "人工纠正" })).toBeVisible();
  expect(qualityTab).toHaveAttribute("aria-selected", "true");
  await user.click(screen.getByRole("tab", { name: /^动画$/ }));
  expect(screen.queryByRole("button", { name: "保存质量标注" })).not.toBeInTheDocument();
});
it("自动评审有理由时优先展示结果，人工纠正默认收起且单项不提供全局导出", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({
        ...review,
        prediction: {
          ...review.prediction,
          status: "suspicious",
          reason: undefined,
          source: "model_review",
          explanation: "脚踏连接缺失",
          model_version: "review-model",
        },
      }),
    ),
  );
  setup();
  expect(await screen.findByText("脚踏连接缺失")).toBeVisible();
  expect(screen.getByRole("button", { name: "人工纠正" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  expect(screen.queryByRole("radio")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "导出标注样本" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "人工纠正" }));
  expect(screen.getByRole("radio", { name: "无法判定" })).toBeChecked();
  expect(screen.getByLabelText("判断理由（必填）")).toHaveValue("");
  expect(screen.queryByRole("button", { name: "样本管理" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "导出全部样本" })).not.toBeInTheDocument();
});
