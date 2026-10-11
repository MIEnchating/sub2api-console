import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(): () => void {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    Array.from({ length: 25 }, (_, index) => ({
      id: String(index + 1),
      name: `检测账号 ${index + 1}`,
      groups: [index === 24 ? "目标分组" : "其他分组"],
      platform: "openai",
      manual_priority: index === 24 ? 1 : null,
    })),
  );
  client.setQueryData(["policy"], {});
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  seedDictionaries(client, { group: [{ value: "1", name: "目标分组" }] });
  const view = render(
    <QueryClientProvider client={client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  return () => {
    view.unmount();
    client.clear();
  };
}

it("前置检测全选跨页全部账号并允许重新勾选第 21 个", async () => {
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
  fireEvent.click(screen.getByRole("tab", { name: "前置检测" }));
  const panel = screen.getByRole("tabpanel", { name: "前置检测" });
  fireEvent.click(await within(panel).findByRole("button", { name: "全选账号" }));
  const submitName = "前置检测（25）";
  expect(within(panel).getByRole("button", { name: submitName })).toBeEnabled();
  fireEvent.click(within(panel).getByRole("button", { name: "转到下一页" }));
  const checkbox = within(panel).getByRole("checkbox", { name: /检测账号 21\b/ });
  expect(checkbox).toBeChecked();
  fireEvent.click(checkbox);
  expect(checkbox).not.toBeChecked();
  expect(checkbox).not.toHaveAttribute("aria-disabled", "true");
  fireEvent.click(checkbox);
  expect(checkbox).toBeChecked();
  expect(within(panel).getByRole("combobox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  fireEvent.click(within(panel).getByRole("button", { name: submitName }));
  await waitFor(() => expect(posts[0]?.targets).toHaveLength(25));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
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
    ["平台", "OpenAI"],
    ["优先状态", "手动控制"],
  ]) {
    await user.click(screen.getByRole("button", { name: `${filter}筛选` }));
    await user.click(await screen.findByRole("option", { name: option }));
    await user.keyboard("{Escape}");
  }
  for (const tab of ["前置检测", "账号检测"]) {
    await user.click(screen.getByRole("tab", { name: tab }));
    const panel = screen.getByRole("tabpanel", { name: tab });
    expect(await within(panel).findByRole("textbox", { name: /搜索.*检测账号/ })).toHaveValue(
      "检测账号",
    );
    expect(within(panel).getByRole("button", { name: "分组筛选" })).toHaveTextContent("目标分组");
    expect(within(panel).getByRole("button", { name: "平台筛选" })).toHaveTextContent("OpenAI");
    expect(within(panel).getByRole("button", { name: "优先状态筛选" })).toHaveTextContent(
      "手动控制",
    );
    expect(within(panel).getAllByRole("checkbox", { name: /检测账号/ })).toHaveLength(1);
    await user.click(within(panel).getByRole("button", { name: "全选账号" }));
    expect(within(panel).getByRole("checkbox", { name: /检测账号 25\b/ })).toBeChecked();
  }
  await user.click(screen.getByRole("tab", { name: "账号检测" }));
  const accountPanel = screen.getByRole("tabpanel", { name: "账号检测" });
  await user.click(await within(accountPanel).findByRole("button", { name: "重置筛选" }));
  await user.click(screen.getByRole("tab", { name: "前置检测" }));
  expect(screen.getByRole("textbox", { name: /搜索.*检测账号/ })).toHaveValue("");
  expect(screen.getAllByRole("checkbox", { name: /检测账号/ })).toHaveLength(12);
  dispose();
});
