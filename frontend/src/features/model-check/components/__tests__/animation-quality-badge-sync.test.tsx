import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationQualityReview, AnimationResult } from "@/api";
import { AnimationPreview } from "../animation-preview";

const result: AnimationResult = {
  source_task_id: "generation-1",
  source_task_status: "succeeded",
  account_id: "41",
  account_name: "测试动画",
  request_id: "request-1",
  model: "test-model",
  status: "succeeded",
  phase: "completed",
  html: "<html>preview</html>",
  source: "<html>source</html>",
  prompt: "骑车题",
  duration_ms: 10,
  completed_at: "2026-10-15T00:00:00Z",
  quality: {
    status: "suspicious",
    score: null,
    benchmark: "test",
    reviewed_at: "2026-10-15T00:00:00Z",
  },
};
const review: AnimationQualityReview = {
  sample_id: "sample",
  benchmark: "test",
  source_sha256: "source",
  prompt_sha256: "prompt",
  recheck_version: 2,
  prediction: {
    status: "normal",
    score: null,
    benchmark: "test",
    reviewed_at: "2026-10-15T01:00:00Z",
  },
  recheck: { task_id: "recheck-1", status: "succeeded", message: "复核完成" },
};
const key = ["animation-quality", "generation-1", "41", "request-1"];
const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
function setup(value = result): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <AnimationPreview result={value} />
    </QueryClientProvider>,
  );
  return client;
}

it("复核详情返回正常而原任务缓存仍旧时卡片立即显示正常", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(review)),
  );
  setup();
  const user = userEvent.setup();
  const trigger = screen.getByRole("button", { name: "放大查看 测试动画 的动画" });
  expect(within(trigger).getByLabelText("自动质量：降智")).toBeVisible();
  await user.click(trigger);
  await user.click(screen.getByRole("tab", { name: "质量标注" }));
  expect(
    await within(screen.getByRole("region", { name: "作品质量标注" })).findByLabelText(
      "自动质量：正常",
    ),
  ).toBeVisible();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(within(trigger).getByLabelText("自动质量：正常")).toBeVisible());
});

it("复核时关闭弹窗后卡片继续读取任务结果并更新标签", async () => {
  let finish: ((response: Response) => void) | undefined;
  const completed = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const fetcher = vi.fn(() => completed);
  vi.stubGlobal("fetch", fetcher);
  const client = setup();
  act(() =>
    client.setQueryData(key, {
      ...review,
      prediction: result.quality,
      recheck_version: 1,
      recheck: { task_id: "recheck-1", status: "running", message: "正在复核" },
    }),
  );
  await waitFor(() => expect(fetcher).toHaveBeenCalled(), { timeout: 4000 });
  await act(async () => {
    finish?.(Response.json(review));
    await completed;
  });
  expect(await screen.findByLabelText("自动质量：正常")).toBeVisible();
});

it("未打开质量标注时卡片不额外读取质量接口", () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  setup();
  expect(screen.getByLabelText("自动质量：降智")).toBeVisible();
  expect(fetcher).not.toHaveBeenCalled();
});

it("同账号的新请求不会沿用旧请求的复核标签", () => {
  vi.stubGlobal("fetch", vi.fn());
  const client = setup({ ...result, request_id: "new-request" });
  act(() => client.setQueryData(key, review));
  expect(screen.getByLabelText("自动质量：降智")).toBeVisible();
});

it("任务已含更新的复核结果时不被较旧的质量缓存覆盖", () => {
  vi.stubGlobal("fetch", vi.fn());
  const client = setup({
    ...result,
    quality: { ...result.quality!, reviewed_at: "2026-10-15T02:00:00Z" },
  });
  act(() => client.setQueryData(key, review));
  expect(screen.getByLabelText("自动质量：降智")).toBeVisible();
});
