import { QueryClientProvider, focusManager } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { AccountStatus } from "@/api";
import { account, task } from "@/features/accounts/__tests__/fixtures";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

afterEach(() => {
  focusManager.setFocused(undefined);
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function setup(
  priority: number | null,
  options: {
    accountsResponse?: () => Promise<Response>;
  } = {},
) {
  let current: AccountStatus = { ...account, platform: "openai", manual_priority: priority };
  const client = createConsoleQueryClient();
  client.setDefaultOptions({
    queries: { staleTime: Infinity, retry: false, refetchOnWindowFocus: false },
  });
  for (const key of [
    ["model-animation", "history"],
    ["model-animation", "schedules"],
  ])
    client.setQueryData(key, []);
  client.setQueryData(["accounts"], [current]);
  client.setQueryData(["policy"], {});
  for (const name of ["group", "platform"])
    client.setQueryData(["dictionaries", name], { items: [] });
  const completed = task("manual-41");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/accounts") return options.accountsResponse?.() ?? Response.json([current]);
      if (path === "/api/accounts/41/manual-priority") {
        current = { ...current, manual_priority: init?.method === "DELETE" ? null : 1 };
        completed.result = {
          account_id: "41",
          manual_priority: current.manual_priority,
          readback_confirmed: true,
          sync_balance_multiplier: false,
        };
        return Response.json(completed);
      }
      if (path === "/api/tasks/manual-41") return Response.json(completed);
      if (
        path === "/api/model-checks/animation-schedules" ||
        path === "/api/model-checks/animations"
      )
        return Response.json([]);
      throw new Error(`Unexpected request: ${path}`);
    }),
  );
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return {
    client,
    update: (value: number | null): void => {
      current = { ...current, manual_priority: value };
    },
    dispose: (): void => {
      view.unmount();
      client.clear();
    },
  };
}

it.each([null, 1])("其他页面修改手动控制（原值 %s）后，返回窗口立即更新标记", async (priority) => {
  const view = setup(priority);
  try {
    const next = priority === null ? 1 : null;
    act(() => focusManager.setFocused(false));
    view.update(next);
    act(() => focusManager.setFocused(true));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: next === null ? "设置手动控制" : "调整手动控制" }),
      ).toBeEnabled(),
    );
    expect(screen.queryByText("手动控制 #1") !== null).toBe(next !== null);
  } finally {
    view.dispose();
  }
});

it("持续停留检测页时，定期读取外部新增的手动控制状态", async () => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
  const view = setup(null);
  try {
    view.update(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(await screen.findByText("手动控制 #1")).toBeVisible();
  } finally {
    view.dispose();
  }
});

it("本页添加手动控制成功后立即显示标记和调整入口", async () => {
  const view = setup(null);
  const user = userEvent.setup();
  try {
    await user.click(screen.getByRole("button", { name: "设置手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    await user.click(within(dialog).getByRole("combobox", { name: "选择手动控制位置" }));
    await user.click(await screen.findByRole("option", { name: /^1 .*可用/ }));
    await user.click(within(dialog).getByRole("button", { name: "设置手动控制" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText("手动控制 #1")).toBeVisible();
    expect(screen.getByRole("button", { name: "调整手动控制" })).toBeEnabled();
  } finally {
    view.dispose();
  }
});

it("返回窗口读取失败时保留已有标记，再次返回读取成功后更新", async () => {
  const view = setup(1);
  const error = vi.spyOn(toast, "error");
  try {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ detail: "账号读取失败，请重试" }, { status: 403 }),
    );
    act(() => focusManager.setFocused(false));
    view.update(null);
    act(() => focusManager.setFocused(true));
    await waitFor(() => expect(error).toHaveBeenCalled());
    expect(screen.getByText("手动控制 #1")).toBeVisible();
    act(() => focusManager.setFocused(false));
    act(() => focusManager.setFocused(true));
    expect(await screen.findByRole("button", { name: "设置手动控制" })).toBeEnabled();
    expect(screen.queryByText("手动控制 #1")).not.toBeInTheDocument();
  } finally {
    view.dispose();
  }
});

it("手动控制筛选下取消成功后，账号立即退出列表", async () => {
  let resolveResponse!: (response: Response) => void;
  const response = new Promise<Response>((resolve) => {
    resolveResponse = resolve;
  });
  const view = setup(1, { accountsResponse: () => response });
  const user = userEvent.setup();
  try {
    await user.click(screen.getByRole("button", { name: "优先状态筛选" }));
    await user.click(await screen.findByRole("option", { name: "手动控制" }));
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "调整手动控制" }));
    const dialog = await screen.findByRole("dialog", { name: "手动控制" });
    fireEvent.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.queryByRole("article")).not.toBeInTheDocument());
  } finally {
    view.dispose();
    resolveResponse(Response.json([]));
  }
});

it.each([null, 1])(
  "整份列表刷新未返回时，任务确认后立即更新手动控制（原值 %s）",
  async (priority) => {
    let resolveResponse!: (response: Response) => void;
    const response = new Promise<Response>((resolve) => {
      resolveResponse = resolve;
    });
    const view = setup(priority, { accountsResponse: () => response });
    const user = userEvent.setup();
    try {
      await user.click(
        screen.getByRole("button", { name: priority === null ? "设置手动控制" : "调整手动控制" }),
      );
      const dialog = await screen.findByRole("dialog", { name: "手动控制" });
      if (priority === null) {
        await user.click(within(dialog).getByRole("combobox", { name: "选择手动控制位置" }));
        await user.click(await screen.findByRole("option", { name: /^1 .*可用/ }));
        await user.click(within(dialog).getByRole("button", { name: "设置手动控制" }));
      } else {
        fireEvent.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
      }
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(screen.queryByText("手动控制 #1") !== null).toBe(priority === null);
      expect(
        screen.getByRole("button", { name: priority === null ? "调整手动控制" : "设置手动控制" }),
      ).toBeEnabled();
    } finally {
      view.dispose();
      resolveResponse(Response.json([]));
    }
  },
);

it.each([null, 1])(
  "旧列表响应晚到时，不覆盖任务已确认的手动控制状态（原值 %s）",
  async (priority) => {
    let resolveOld!: (response: Response) => void;
    let resolveFresh!: (response: Response) => void;
    const oldResponse = new Promise<Response>((resolve) => {
      resolveOld = resolve;
    });
    const freshResponse = new Promise<Response>((resolve) => {
      resolveFresh = resolve;
    });
    const accountsResponse = vi
      .fn()
      .mockReturnValueOnce(oldResponse)
      .mockReturnValue(freshResponse);
    const view = setup(priority, { accountsResponse });
    const user = userEvent.setup();
    try {
      act(() => {
        void view.client.invalidateQueries({ queryKey: ["accounts"] });
      });
      await waitFor(() => expect(accountsResponse).toHaveBeenCalledTimes(1));
      await user.click(
        screen.getByRole("button", { name: priority === null ? "设置手动控制" : "调整手动控制" }),
      );
      const dialog = await screen.findByRole("dialog", { name: "手动控制" });
      if (priority === null) {
        await user.click(within(dialog).getByRole("combobox", { name: "选择手动控制位置" }));
        await user.click(await screen.findByRole("option", { name: /^1 .*可用/ }));
        await user.click(within(dialog).getByRole("button", { name: "设置手动控制" }));
      } else {
        fireEvent.click(within(dialog).getByRole("button", { name: "取消手动控制" }));
      }
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      await act(async () => {
        resolveOld(Response.json([{ ...account, platform: "openai", manual_priority: priority }]));
      });
      expect(screen.queryByText("手动控制 #1") !== null).toBe(priority === null);
      expect(
        screen.getByRole("button", { name: priority === null ? "调整手动控制" : "设置手动控制" }),
      ).toBeEnabled();
    } finally {
      view.dispose();
      resolveOld(Response.json([]));
      resolveFresh(Response.json([]));
    }
  },
);
