import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UpstreamActionsMenu } from "../upstream-actions-menu";

const preview = {
  upstream_id: "up_fixture",
  host: "fixture.example",
  excluded: false,
  account_count: 2,
  account_ids: ["41", "42"],
  accounts: [
    { id: "41", name: "账号甲", groups: [] },
    { id: "42", name: "账号乙", groups: [] },
  ],
};
const task = {
  id: "task-exclude",
  status: "succeeded",
  progress: 100,
  message: "上游排除设置已完成",
  result: { excluded: true, accounts: [] },
};
function mount(excluded = false, onViewGroups = (): void => {}): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <UpstreamActionsMenu host="fixture.example" excluded={excluded} onViewGroups={onViewGroups} />
    </QueryClientProvider>,
  );
}
function respond(value: unknown): Response {
  return new Response(JSON.stringify(value), { status: 200 });
}
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("键盘打开排除后显示账号范围，默认保留调度，取消不写入", async () => {
  const fetch = vi.fn().mockImplementation(() => Promise.resolve(respond(preview)));
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mount();
  await user.tab();
  await user.keyboard("{Enter}");
  await screen.findByRole("menu");
  await user.keyboard("{Home}{ArrowDown}{Enter}");
  expect(await screen.findByText("账号甲（ID：41）")).toBeVisible();
  expect(
    screen.getByRole("checkbox", { name: "同时关闭该上游下所有账号的调度" }),
  ).not.toBeChecked();
  await user.click(screen.getByRole("button", { name: "取消" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(screen.getByRole("button", { name: "更多操作" })).toHaveFocus());
  expect(fetch.mock.calls.every((call) => call[1]?.method !== "POST")).toBe(true);
});

it("选择关闭调度并确认时发送完整稳定账号范围，完成后显示结果", async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string) =>
      Promise.resolve(respond(url.includes("exclusion-preview") ? preview : task)),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mount();
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "排除上游" }));
  await screen.findByText("账号乙（ID：42）");
  await user.click(screen.getByRole("checkbox"));
  await user.click(screen.getByRole("button", { name: "确认排除" }));
  expect(await screen.findByText("上游排除设置已完成")).toBeVisible();
  expect(fetch).toHaveBeenCalledWith(
    "/api/upstreams/fixture.example/exclusion",
    expect.objectContaining({
      method: "POST",
      credentials: "include",
      body: JSON.stringify({
        upstream_id: "up_fixture",
        excluded: true,
        stop_scheduling: true,
        expected_account_ids: ["41", "42"],
      }),
    }),
  );
});

it("读取范围期间禁用确认并保留取消入口", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const user = userEvent.setup();
  mount();
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "排除上游" }));
  expect(screen.getByText("正在读取受影响账号")).toBeVisible();
  expect(screen.getByRole("button", { name: "确认排除" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  await act(async () => resolve(respond(preview)));
  await waitFor(() => expect(screen.getByRole("button", { name: "确认排除" })).toBeEnabled());
});

it("空账号上游取消排除时隐藏调度选择并只恢复管控", async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string) =>
      Promise.resolve(
        respond(
          url.includes("exclusion-preview")
            ? { ...preview, excluded: true, account_count: 0, account_ids: [], accounts: [] }
            : task,
        ),
      ),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mount(true);
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "取消排除上游" }));
  await screen.findByText("关联账号：0 个");
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认取消排除" }));
  await screen.findByText("上游排除设置已完成");
  expect(fetch).toHaveBeenCalledWith(
    "/api/upstreams/fixture.example/exclusion",
    expect.objectContaining({
      body: JSON.stringify({
        upstream_id: "up_fixture",
        excluded: false,
        stop_scheduling: false,
        expected_account_ids: [],
      }),
    }),
  );
});

it("范围读取失败时提供重试并禁止提交", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(
          new Response(JSON.stringify({ detail: "读取失败，请重试" }), { status: 500 }),
        ),
      ),
  );
  const user = userEvent.setup();
  mount();
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "排除上游" }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "确认排除" })).toBeDisabled();
});

it("已排除上游重试关闭调度时保留排除并默认选择关闭", async () => {
  const fetch = vi
    .fn()
    .mockImplementation((url: string) =>
      Promise.resolve(
        respond(url.includes("exclusion-preview") ? { ...preview, excluded: true } : task),
      ),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mount(true);
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "关闭已排除上游的账号调度" }));
  await screen.findByText("账号甲（ID：41）");
  expect(screen.getByRole("checkbox")).toBeChecked();
  await user.click(screen.getByRole("button", { name: "确认关闭调度" }));
  await screen.findByText("上游排除设置已完成");
  expect(fetch).toHaveBeenCalledWith(
    "/api/upstreams/fixture.example/exclusion",
    expect.objectContaining({
      body: JSON.stringify({
        upstream_id: "up_fixture",
        excluded: true,
        stop_scheduling: true,
        expected_account_ids: ["41", "42"],
      }),
    }),
  );
});

it("任务排队时关闭并重新打开弹窗仍展示原任务且不重复提交", async () => {
  const queued = { ...task, status: "queued", progress: 0, message: "上游排除设置已排队" };
  const fetch = vi
    .fn()
    .mockImplementation((url: string) =>
      Promise.resolve(respond(url.includes("exclusion-preview") ? preview : queued)),
    );
  vi.stubGlobal("fetch", fetch);
  const user = userEvent.setup();
  mount();
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "排除上游" }));
  await screen.findByText("账号甲（ID：41）");
  await user.click(screen.getByRole("button", { name: "确认排除" }));
  await screen.findByText("上游排除设置已排队");
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "更多操作" }));
  await user.click(await screen.findByRole("menuitem", { name: "排除上游" }));
  expect(await screen.findByText("上游排除设置已排队")).toBeVisible();
  expect(screen.queryByRole("button", { name: "确认排除" })).not.toBeInTheDocument();
  expect(fetch.mock.calls.filter((call) => call[1]?.method === "POST")).toHaveLength(1);
});

it.each([false, true])(
  "排除状态为 %s 时更多菜单仅显示对应排除操作且分组入口可用",
  async (excluded) => {
    const user = userEvent.setup();
    const viewGroups = vi.fn();
    mount(excluded, viewGroups);
    const trigger = screen.getByRole("button", { name: "更多操作" });
    expect(screen.getAllByRole("button")).toHaveLength(1);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    await user.click(trigger);
    await waitFor(() => expect(trigger).toHaveAttribute("aria-expanded", "true"));
    expect(
      screen.getByRole("menuitem", { name: excluded ? "取消排除上游" : "排除上游" }),
    ).toBeVisible();
    expect(
      screen.queryByRole("menuitem", { name: excluded ? "排除上游" : "取消排除上游" }),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "查看分组" }));
    expect(viewGroups).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  },
);
