import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(): {
  client: ReturnType<typeof createConsoleQueryClient>;
  close: ReturnType<typeof vi.fn>;
} {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    [
      { id: "41", name: "甲账号", groups: ["测试"], platform: "openai" },
      { id: "42", name: "乙账号", groups: [], platform: "anthropic" },
      { id: "43", name: "人工账号", groups: [], platform: "openai", manual_priority: 1 },
    ],
  );
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  const close = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return { client, close };
}

it("选择多个账号和统一模型后直接按稳定 ID 创建批量任务且不弹窗", async () => {
  const user = userEvent.setup();
  const calls: unknown[] = [];
  const queued = {
    id: "animation-1",
    skill: "sub2api-model-animation",
    operation: "account-model-animation",
    status: "queued",
    progress: 0,
    message: "正在等待动画生成",
    result: { account_ids: ["41", "42"], animations: [] },
    created_at: "2026-09-13T00:00:00Z",
    updated_at: "2026-09-13T00:00:00Z",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        calls.push(JSON.parse(String(init.body)));
        expect(init.credentials).toBe("include");
        return Response.json(queued);
      }
      return Response.json(String(_input).includes("/api/tasks/") ? queued : [queued]);
    }),
  );
  const view = setup();
  expect(screen.getByRole("checkbox", { name: /检测 人工账号/ })).not.toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  await user.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  await user.click(screen.getByRole("checkbox", { name: /检测 乙账号/ }));
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  await user.clear(screen.getByRole("combobox", { name: "检测模型" }));
  await user.type(screen.getByRole("combobox", { name: "检测模型" }), "custom-model");
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "开始检测（2 个账号）" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  await waitFor(() =>
    expect(calls).toEqual([
      {
        targets: [
          { account_id: "41", model: "custom-model" },
          { account_id: "42", model: "custom-model" },
        ],
        timeout_seconds: 120,
      },
    ]),
  );
  expect(
    await within(screen.getByRole("group", { name: "动画检测操作" })).findByRole("button", {
      name: "取消任务",
    }),
  ).toBeEnabled();
  expect(
    within(screen.getByRole("article", { name: "账号 甲账号" })).getByRole("status", {
      name: "排队中，等待并发槽位",
    }),
  ).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(view.client.getQueryData(["tasks"])).toEqual([
    expect.objectContaining({
      id: queued.id,
      operation: queued.operation,
      status: "queued",
      system_info: true,
    }),
  ]);
  view.client.clear();
});

it("默认模型无需读取上游模型列表，选择账号后可直接检测", () => {
  const fetcher = vi.fn<typeof fetch>();
  vi.stubGlobal("fetch", fetcher);
  const view = setup();
  fireEvent.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled();
  expect(fetcher.mock.calls.some(([url]) => String(url).endsWith("/models"))).toBe(false);
  expect(screen.getByRole("button", { name: "开始检测（1 个账号）" })).toBeEnabled();
  view.client.clear();
});

it("仅选择账号即可提交默认模型，无需填写模型", async () => {
  const posts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") posts.push(JSON.parse(String(init.body)));
      if (init?.method !== "POST" && !String(_input).includes("/api/tasks/"))
        return Response.json([]);
      return Response.json({ id: "validation-task", status: "succeeded", result: {} });
    }),
  );
  const view = setup();
  fireEvent.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  await waitFor(() =>
    expect(posts).toEqual([
      { targets: [{ account_id: "41", model: "gpt-6-astra" }], timeout_seconds: 120 },
    ]),
  );
  view.client.clear();
});

it("首次读取账号时展示骨架屏并禁止提交", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false } });
  render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  expect(await screen.findByRole("status", { name: "正在读取账号" })).toBeVisible();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  expect(screen.getByRole("status", { name: "正在读取账号" })).toHaveAttribute(
    "data-slot",
    "page-loading-skeleton",
  );
  client.clear();
});

it("没有字段错误或模型读取提示时不渲染提示占位区", () => {
  const view = setup();
  expect(
    document.querySelector('[data-slot="animation-settings-feedback"]'),
  ).not.toBeInTheDocument();
  view.client.clear();
});

it("默认模型无校验错误，超时字段错误不会覆盖主体", async () => {
  const view = setup();
  fireEvent.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  fireEvent.change(screen.getByRole("spinbutton", { name: "请求超时（秒）" }), {
    target: { value: "1" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  expect(await screen.findByText("超时不能小于 5 秒")).toBeVisible();
  expect(
    document.querySelector('[data-slot="animation-settings-feedback"]'),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  view.client.clear();
});
