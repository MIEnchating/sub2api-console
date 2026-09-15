import { QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

it("选择多个账号和统一模型后先展示影响范围，再按稳定 ID 创建批量任务", async () => {
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
  await user.type(screen.getByRole("combobox", { name: "检测模型" }), "shared-model");
  await user.click(screen.getByRole("button", { name: "开始检测（2 个账号）" }));
  const confirm = await screen.findByRole("dialog", { name: "确认动画检测范围" });
  expect(
    within(confirm).getByText(/甲账号（ID 41）→ shared-model；乙账号（ID 42）→ shared-model/),
  ).toBeVisible();
  expect(calls).toHaveLength(0);
  await user.click(within(confirm).getByRole("button", { name: "确认并开始检测" }));
  await waitFor(() =>
    expect(calls).toEqual([
      {
        targets: [
          { account_id: "41", model: "shared-model" },
          { account_id: "42", model: "shared-model" },
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
      name: "生成中，等待动画结果",
    }),
  ).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  view.client.clear();
});

it("模型列表刷新失败时保留手动输入，且反馈不占用主体布局", async () => {
  const errorToast = vi.spyOn(toast, "error");
  let reject: (error: Error) => void = () => {};
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((_resolve, rejectPromise) => {
          reject = rejectPromise;
        }),
    ),
  );
  const view = setup();
  const model = screen.getByRole("combobox", { name: "检测模型" });
  fireEvent.change(model, { target: { value: "manual-model" } });
  fireEvent.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  expect(screen.getByRole("button", { name: "获取模型" })).toBeDisabled();
  await act(async () => reject(new Error("模型列表请求失败")));
  expect(await screen.findByText("模型列表请求失败")).toBeVisible();
  expect(errorToast).toHaveBeenCalledTimes(1);
  expect(model).toHaveValue("manual-model");
  expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled();
  view.client.clear();
});

it("仅选择账号而未填写模型时阻止提交，并允许使用统一模型", async () => {
  const view = setup();
  fireEvent.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  expect(await screen.findByText("请输入模型 ID")).toBeVisible();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.queryByRole("dialog", { name: "确认动画检测范围" })).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
    target: { value: "shared-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  expect(await screen.findByRole("dialog", { name: "确认动画检测范围" })).toHaveTextContent(
    "shared-model",
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

it("模型校验失败时通过提示反馈，且设置栏不插入覆盖主体的提示层", async () => {
  const view = setup();
  fireEvent.click(screen.getByRole("checkbox", { name: /检测 甲账号/ }));
  fireEvent.click(screen.getByRole("button", { name: "开始检测（1 个账号）" }));
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveAttribute(
      "aria-invalid",
      "true",
    ),
  );
  expect(
    document.querySelector('[data-slot="animation-settings-feedback"]'),
  ).not.toBeInTheDocument();
  const model = screen.getByRole("combobox", { name: "检测模型" });
  fireEvent.change(model, { target: { value: "fixture-model" } });
  await waitFor(() => expect(model).toHaveAttribute("aria-invalid", "false"));
  view.client.clear();
});
