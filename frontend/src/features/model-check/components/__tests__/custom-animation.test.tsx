import { QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast, Toaster } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("matchMedia", (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(() => true),
  }));
});
afterEach(() => {
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const secret = "isolated-custom-secret";
const queued = {
  id: "custom-task",
  skill: "sub2api-model-animation",
  operation: "account-model-animation",
  status: "running",
  progress: 0,
  message: "正在生成动画",
  result: {
    account_ids: ["custom-target"],
    targets: [{ account_id: "custom-target", model: "custom-model" }],
    animations: [],
  },
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};

async function setup(historyReady = true) {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(["accounts"], []);
  client.setQueryData(["model-animation", "schedules"], []);
  if (historyReady) client.setQueryData(["model-animation", "history"], []);
  const view = render(
    <QueryClientProvider client={client}>
      <Toaster />
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  await userEvent.click(screen.getByRole("tab", { name: "自定义接口" }));
  return { client, ...view };
}

function fill(): void {
  fireEvent.change(screen.getByRole("textbox", { name: "Base URL" }), {
    target: { value: "https://custom.example.invalid/v1" },
  });
  fireEvent.change(screen.getByLabelText("API Key"), { target: { value: secret } });
  fireEvent.change(screen.getByRole("combobox", { name: "检测模型" }), {
    target: { value: "custom-model" },
  });
}

it("没有账号时可确认自定义接口并创建任务，提交成功后清除 Key 和 mutation 中的凭据", async () => {
  const calls: unknown[] = [];
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
  const view = await setup();
  expect(screen.getByText("暂无自定义接口检测记录")).toBeVisible();
  fill();
  expect(screen.getByLabelText("API Key")).toHaveAttribute("type", "password");
  await userEvent.click(screen.getByRole("button", { name: "开始检测" }));
  const dialog = await screen.findByRole("dialog", { name: "确认自定义接口检测" });
  expect(dialog).toHaveTextContent("https://custom.example.invalid/v1");
  expect(dialog).toHaveTextContent("custom-model");
  expect(dialog).not.toHaveTextContent(secret);
  expect(calls).toHaveLength(0);
  await userEvent.click(within(dialog).getByRole("button", { name: "确认并开始检测" }));
  await waitFor(() => expect(screen.getByLabelText("API Key")).toHaveValue(""));
  expect(calls).toEqual([
    {
      targets: [],
      timeout_seconds: 120,
      custom: {
        base_url: "https://custom.example.invalid/v1",
        api_key: secret,
        platform: "openai",
        model: "custom-model",
      },
    },
  ]);
  expect(await screen.findByRole("status", { name: "生成中，等待动画结果" })).toBeVisible();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  await waitFor(() =>
    expect(
      JSON.stringify(
        view.client
          .getMutationCache()
          .getAll()
          .map((mutation) => mutation.state.variables),
      ),
    ).not.toContain(secret),
  );
  view.unmount();
  view.client.clear();
});

it("地址含凭据或 Key 为空时展示字段错误并阻止创建任务", async () => {
  const view = await setup();
  fireEvent.change(screen.getByRole("textbox", { name: "Base URL" }), {
    target: { value: "https://user:password@example.invalid" },
  });
  await userEvent.click(screen.getByRole("button", { name: "开始检测" }));
  expect(await screen.findByText("请输入 API Key")).toBeVisible();
  expect(screen.getByRole("textbox", { name: "Base URL" })).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByLabelText("API Key")).toHaveAttribute("aria-invalid", "true");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  view.unmount();
  view.client.clear();
});

it.each([false, true])(
  "自定义前置检测单题模式为 %s 时按选择提交和展示结果并清除 Key",
  async (single) => {
    const completed = {
      ...queued,
      operation: "account-model-precheck",
      status: "succeeded",
      progress: 100,
      result: {
        ...queued.result,
        mode: "precheck",
        animations: [
          {
            account_id: "custom-target",
            account_name: "自定义接口",
            mode: "precheck",
            model: "custom-model",
            status: "succeeded",
            request_id: "custom-precheck",
            completed_at: "2026-09-15T00:00:00Z",
            duration_ms: 20,
            precheck: {
              verdict: "passed",
              profile_version: "astra-v1",
              questions: [
                { id: "candy", verdict: "passed", answer: "21", request_id: "candy" },
                {
                  id: "knowledge-cutoff",
                  verdict: "passed",
                  answer: "无法提供日期",
                  request_id: "cutoff",
                },
              ].slice(0, single ? 1 : 2),
            },
          },
        ],
      },
    };
    const calls: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "POST") {
          calls.push(JSON.parse(String(init.body)));
          return Response.json(completed);
        }
        return Response.json(String(input).includes("/api/tasks/") ? completed : [completed]);
      }),
    );
    const view = await setup();
    fill();
    if (single) {
      await userEvent.click(screen.getByRole("button", { name: "选择前置检测题目" }));
      await userEvent.click(
        within(screen.getByRole("dialog", { name: "前置检测题目" })).getByRole("checkbox", {
          name: "知识截止日期",
        }),
      );
      await userEvent.keyboard("{Escape}");
    }
    await userEvent.click(screen.getByRole("button", { name: "前置检测" }));
    const dialog = await screen.findByRole("dialog", { name: "确认自定义接口检测" });
    expect(dialog).toHaveTextContent(single ? "前置检测请求（糖果题）" : "糖果题和知识截止日期题");
    await userEvent.click(within(dialog).getByRole("button", { name: "确认并开始检测" }));
    const result = await screen.findByRole("region", { name: "前置检测结果" });
    const questions = await within(result).findByRole("list", { name: "前置检测题目结果" });
    expect(questions).toHaveTextContent("糖果题通过");
    expect(within(questions).getAllByRole("listitem")).toHaveLength(single ? 1 : 2);
    if (single) expect(result).not.toHaveTextContent("知识截止日期");
    else expect(questions).toHaveTextContent("知识截止日期通过");
    await userEvent.click(within(result).getByRole("button", { name: "查看前置检测详情" }));
    const detail = await screen.findByRole("dialog", { name: "前置检测详情" });
    expect(within(detail).getByText("21")).toBeVisible();
    if (single) expect(within(detail).queryByText("无法提供日期")).not.toBeInTheDocument();
    else expect(within(detail).getByText("无法提供日期")).toBeVisible();
    expect(detail).not.toHaveTextContent(secret);
    await userEvent.click(within(detail).getByRole("button", { name: "关闭" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "前置检测详情" })).not.toBeInTheDocument(),
    );
    expect(calls).toEqual([
      {
        mode: "precheck",
        targets: [],
        precheck_questions: single ? ["candy"] : ["candy", "knowledge-cutoff"],
        timeout_seconds: 120,
        custom: {
          base_url: "https://custom.example.invalid/v1",
          api_key: secret,
          platform: "openai",
          model: "custom-model",
        },
      },
    ]);
    await waitFor(() => expect(screen.getByLabelText("API Key")).toHaveValue(""));
    view.unmount();
    view.client.clear();
  },
);

it("创建失败时解除禁用并保留输入以便重试，只通过 toast 展示错误", async () => {
  let rejectRequest: (error: Error) => void = () => {};
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((_resolve, reject) => {
          rejectRequest = reject;
        }),
    ),
  );
  const view = await setup();
  fill();
  await userEvent.click(screen.getByRole("button", { name: "开始检测" }));
  await userEvent.click(
    within(await screen.findByRole("dialog")).getByRole("button", { name: "确认并开始检测" }),
  );
  expect(screen.getByRole("textbox", { name: "Base URL", hidden: true })).toBeDisabled();
  await act(async () => rejectRequest(new Error("隔离接口连接失败")));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(await screen.findByText("隔离接口连接失败")).toBeVisible();
  expect(screen.getByLabelText("API Key")).toHaveValue(secret);
  expect(screen.getByRole("button", { name: "开始检测" })).toBeEnabled();
  expect(
    within(screen.getByRole("tabpanel", { name: "自定义接口" })).queryByText(/连接失败/),
  ).not.toBeInTheDocument();
  view.unmount();
  view.client.clear();
});

it("键盘切换检测来源后自定义 Key 被清除，选中状态与可见面板一致", async () => {
  const view = await setup();
  fill();
  const customTab = screen.getByRole("tab", { name: "自定义接口" });
  customTab.focus();
  await userEvent.keyboard("{ArrowLeft}{Enter}");
  await waitFor(() =>
    expect(screen.getByRole("tab", { name: "账号检测" })).toHaveAttribute("aria-selected", "true"),
  );
  expect(screen.queryByLabelText("API Key")).not.toBeInTheDocument();
  await userEvent.keyboard("{ArrowRight}{Enter}");
  await waitFor(() => expect(customTab).toHaveAttribute("aria-selected", "true"));
  expect(screen.getByLabelText("API Key")).toHaveValue("");
  view.unmount();
  view.client.clear();
});

it("首次读取历史记录时展示骨架屏，返回空列表后才展示空状态", async () => {
  let resolveHistory: (response: Response) => void = () => {};
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveHistory = resolve;
        }),
    ),
  );
  const view = await setup(false);
  expect(screen.getByRole("status", { name: "正在读取自定义检测记录" })).toHaveAttribute(
    "data-slot",
    "page-loading-skeleton",
  );
  expect(screen.queryByText("暂无自定义接口检测记录")).not.toBeInTheDocument();
  await act(async () => resolveHistory(Response.json([])));
  expect(await screen.findByText("暂无自定义接口检测记录")).toBeVisible();
  view.unmount();
  view.client.clear();
});

it("自定义历史读取失败时提供重试入口，恢复后展示空列表", async () => {
  let failed = true;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      if (failed) return Response.json({ detail: "历史服务暂不可用" }, { status: 503 });
      return Response.json([]);
    }),
  );
  const view = await setup(false);
  const panel = within(screen.getByRole("tabpanel", { name: "自定义接口" }));
  const retry = await panel.findByRole("button", { name: "重新读取" });
  expect(panel.queryByText("暂无自定义接口检测记录")).not.toBeInTheDocument();
  failed = false;
  await userEvent.click(retry);
  expect(await panel.findByText("暂无自定义接口检测记录")).toBeVisible();
  expect(panel.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  view.unmount();
  view.client.clear();
});
