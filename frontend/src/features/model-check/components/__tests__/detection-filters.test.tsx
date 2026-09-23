import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";
import type { Task } from "@/api";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(accountID?: string, history: Task[] = [], unsupported = false): () => void {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    Array.from({ length: 25 }, (_, index) => ({
      id: String(index + 1),
      name: `检测账号 ${index + 1}`,
      groups: [index === 24 ? "目标分组" : "其他分组"],
      platform: unsupported && index === 0 ? "gemini" : "openai",
      manual_priority: index === 24 ? 1 : null,
    })),
  );
  client.setQueryData(["policy"], {});
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["terminal-continuity", "history"], history);
  for (const dictionary of ["group", "platform"])
    client.setQueryData(["dictionaries", dictionary], { items: [] });
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel accountID={accountID} />
    </QueryClientProvider>,
  );
  return () => {
    view.unmount();
    client.clear();
  };
}

it.each(["前置检测", "终端续接检测"])("%s 全选跨页全部账号并允许重新勾选第 21 个", async (tab) => {
  const posts: { targets: unknown[] }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") posts.push(JSON.parse(String(init.body)));
      if (init?.method !== "POST" && !String(_input).includes("/api/tasks/"))
        return Response.json([]);
      return Response.json({ id: "filtered-task", status: "succeeded", result: {} });
    }),
  );
  const dispose = setup();
  fireEvent.click(screen.getByRole("tab", { name: tab }));
  const panel = screen.getByRole("tabpanel", { name: tab });
  fireEvent.click(await within(panel).findByRole("button", { name: "全选账号" }));
  const submitName = tab === "前置检测" ? "前置检测（25）" : "开始检测（25 个账号）";
  expect(within(panel).getByRole("button", { name: submitName })).toBeEnabled();
  fireEvent.click(within(panel).getByRole("button", { name: "转到下一页" }));
  const checkbox = within(panel).getByRole("checkbox", { name: /检测账号 21\b/ });
  expect(checkbox).toBeChecked();
  fireEvent.click(checkbox);
  expect(checkbox).not.toBeChecked();
  expect(checkbox).not.toHaveAttribute("aria-disabled", "true");
  fireEvent.click(checkbox);
  expect(checkbox).toBeChecked();
  fireEvent.change(within(panel).getByRole("combobox", { name: "检测模型" }), {
    target: { value: "shared-model" },
  });
  fireEvent.click(within(panel).getByRole("button", { name: submitName }));
  if (tab === "前置检测") {
    await waitFor(() => expect(posts[0]?.targets).toHaveLength(25));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  } else {
    expect(await screen.findByRole("dialog", { name: "确认终端续接检测范围" })).toHaveTextContent(
      "25 个账号",
    );
  }
  dispose();
});

it("切换检测标签时保留搜索及分组、平台、优先状态，重置同步生效", async () => {
  const user = userEvent.setup();
  const dispose = setup();
  fireEvent.change(screen.getByRole("textbox", { name: /搜索.*检测账号/ }), {
    target: { value: "检测账号" },
  });
  for (const [filter, option] of [
    ["分组", "目标分组"],
    ["平台", "openai"],
    ["优先状态", "手动控制"],
  ]) {
    await user.click(screen.getByRole("button", { name: `${filter}筛选` }));
    await user.click(await screen.findByRole("option", { name: option }));
    await user.keyboard("{Escape}");
  }
  for (const tab of ["前置检测", "终端续接检测", "账号检测"]) {
    await user.click(screen.getByRole("tab", { name: tab }));
    const panel = screen.getByRole("tabpanel", { name: tab });
    expect(await within(panel).findByRole("textbox", { name: /搜索.*检测账号/ })).toHaveValue(
      "检测账号",
    );
    expect(within(panel).getByRole("button", { name: "分组筛选" })).toHaveTextContent("目标分组");
    expect(within(panel).getByRole("button", { name: "平台筛选" })).toHaveTextContent("openai");
    expect(within(panel).getByRole("button", { name: "优先状态筛选" })).toHaveTextContent(
      "手动控制",
    );
    expect(within(panel).getAllByRole("checkbox", { name: /检测账号/ })).toHaveLength(1);
    await user.click(within(panel).getByRole("button", { name: "全选账号" }));
    expect(within(panel).getByRole("checkbox", { name: /检测账号 25\b/ })).toBeChecked();
  }
  await user.click(screen.getByRole("tab", { name: "终端续接检测" }));
  const terminal = screen.getByRole("tabpanel", { name: "终端续接检测" });
  await user.click(await within(terminal).findByRole("button", { name: "重置筛选" }));
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByRole("textbox", { name: /搜索.*检测账号/ })).toHaveValue("");
  expect(screen.getAllByRole("checkbox", { name: /检测账号/ })).toHaveLength(12);
  dispose();
});

it("终端检测选择全部疑似异常后提交完整的 25 个稳定账号", async () => {
  const history: Task = {
    id: "suspected-history",
    skill: "sub2api-terminal-continuity",
    operation: "account-terminal-continuity",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "2026-09-22T00:00:00Z",
    updated_at: "2026-09-22T00:00:00Z",
    result: {
      checks: Array.from({ length: 25 }, (_, index) => ({
        account_id: String(index + 1),
        account_name: `检测账号 ${index + 1}`,
        model: "shared-model",
        request_id: `request-${index + 1}`,
        verdict: "suspected",
        duration_ms: 10,
        completed_at: "2026-09-22T00:00:00Z",
      })),
    },
  };
  const requests: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        requests.push(JSON.parse(String(init.body)));
        return Response.json({ ...history, id: "new-check" });
      }
      return Response.json([history]);
    }),
  );
  const dispose = setup(undefined, [history]);
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: "终端续接检测" }));
  const panel = screen.getByRole("tabpanel", { name: "终端续接检测" });
  await user.click(await within(panel).findByRole("button", { name: "选择疑似异常（25）" }));
  fireEvent.change(within(panel).getByRole("combobox", { name: "检测模型" }), {
    target: { value: "shared-model" },
  });
  await user.click(within(panel).getByRole("button", { name: "开始检测（25 个账号）" }));
  const dialog = screen.getByRole("dialog", { name: "确认终端续接检测范围" });
  await user.click(within(dialog).getByRole("button", { name: "确认并开始检测" }));
  expect(requests).toEqual([
    {
      targets: Array.from({ length: 25 }, (_, index) => ({
        account_id: String(index + 1),
        model: "shared-model",
      })),
      timeout_seconds: 120,
      rounds: 1,
    },
  ]);
  dispose();
});

it("终端全选排除忙碌及不支持的账号，搜索无结果时禁用全选", async () => {
  const busy: Task = {
    id: "busy-terminal",
    skill: "sub2api-terminal-continuity",
    operation: "account-terminal-continuity",
    status: "running",
    progress: 1,
    message: "检测中",
    created_at: "2026-09-22T00:00:00Z",
    updated_at: "2026-09-22T00:00:00Z",
    result: { account_ids: ["2"], checks: [] },
  };
  const dispose = setup(undefined, [busy], true);
  fireEvent.click(screen.getByRole("tab", { name: "终端续接检测" }));
  const panel = screen.getByRole("tabpanel", { name: "终端续接检测" });
  fireEvent.click(await within(panel).findByRole("button", { name: "全选账号" }));
  expect(within(panel).getByRole("button", { name: "开始检测（23 个账号）" })).toBeEnabled();
  for (const id of [1, 2]) {
    const checkbox = within(panel).getByRole("checkbox", { name: `检测终端续接 检测账号 ${id}` });
    expect(checkbox).not.toBeChecked();
    expect(checkbox).toHaveAttribute("aria-disabled", "true");
  }
  fireEvent.change(within(panel).getByRole("textbox", { name: /搜索.*检测账号/ }), {
    target: { value: "不存在" },
  });
  expect(within(panel).getByText("没有匹配的账号")).toBeVisible();
  expect(within(panel).getByRole("button", { name: "全选账号" })).toBeDisabled();
  dispose();
});

it("账号详情进入终端检测时继续限定同一稳定账号", async () => {
  const dispose = setup("25");
  fireEvent.click(screen.getByRole("tab", { name: "终端续接检测" }));
  const panel = screen.getByRole("tabpanel", { name: "终端续接检测" });
  expect(await within(panel).findByRole("checkbox", { name: /检测账号 25\b/ })).toBeVisible();
  expect(within(panel).getAllByRole("checkbox", { name: /检测账号/ })).toHaveLength(1);
  dispose();
});
