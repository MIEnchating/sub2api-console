import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, it, expect, vi } from "vitest";
import { api, type AccountModelAutomationSchedule, type NewAPIChannel } from "@/api";
import { AccountModelAutomationPage } from "../account-model-automation-page";

function channel(id: string): NewAPIChannel {
  return {
    id,
    name: `渠道 ${id}`,
    type: 1,
    status: 1,
    models: ["model-a"],
    groups: [],
    version: "a".repeat(64),
  };
}
const schedule: AccountModelAutomationSchedule = {
  id: "saved",
  name: "跨页任务",
  enabled: true,
  interval_minutes: 60,
  last_run_at: null,
  next_run_at: null,
  request: {
    platform_id: "platform",
    group_ids: ["7"],
    channels: [{ id: "1201", version: "a".repeat(64) }],
    selected_models: ["model-a"],
    publish_missing: true,
    price_enabled: false,
    pricing_source: "official",
  },
};
let client: QueryClient;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  seedDictionaries(client);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [
      {
        id: "platform",
        name: "测试平台",
        base_url: "https://fixture.invalid",
        user_id: "1",
        admin_key_configured: true,
        updated_at: "2026-10-09T00:00:00Z",
      },
    ],
    local_groups: [],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "生产分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "groups").mockResolvedValue([
    {
      id: "7",
      name: "生产分组",
      account_count: 1,
      scheduling_open: 1,
      scheduling_closed: 0,
      scheduling_unknown: 0,
      strategy: "default",
      strategy_source: "global_default",
      participation_status: "participating",
      participation_reason: null,
      status: "healthy",
    },
  ]);
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([schedule]);
  vi.spyOn(api, "accountModelAutomationTaskGroups").mockResolvedValue([]);
  vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = new URL(String(input), "http://localhost");
    if (url.pathname.endsWith("/lookup"))
      return new Response(
        JSON.stringify({ items: url.searchParams.getAll("id").map(channel), failures: [] }),
      );
    const page = Number(url.searchParams.get("page"));
    const size = Number(url.searchParams.get("page_size"));
    return new Response(
      JSON.stringify({
        items: Array.from({ length: Math.min(size, 1301 - page * size) }, (_, i) =>
          channel(String(page * size + i + 1)),
        ),
        total: 1301,
      }),
    );
  });
});
afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function renderPage(): void {
  render(
    <QueryClientProvider client={client}>
      <AccountModelAutomationPage />
    </QueryClientProvider>,
  );
}

it("打开大目录的新增弹窗时只读一页，翻页后保留跨页选择且不缓存全部目录", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "添加任务" }));
  await screen.findByRole("textbox", { name: "运营任务名称" });
  expect(fetch).toHaveBeenCalledTimes(1);
  const channels = screen.getByRole("group", { name: "目标渠道" });
  await user.click(within(channels).getByRole("checkbox", { name: "渠道 1（1）" }));
  await user.click(within(channels).getByRole("button", { name: "转到下一页" }));
  await within(channels).findByRole("checkbox", { name: "渠道 51（51）" });
  expect(within(channels).getByRole("checkbox", { name: "渠道 1（1）" })).toBeChecked();
  expect(within(channels).queryByRole("checkbox", { name: "渠道 2（2）" })).not.toBeInTheDocument();
  expect(fetch).toHaveBeenCalledTimes(2);
  await waitFor(() =>
    expect(
      client
        .getQueryCache()
        .findAll({ queryKey: ["newapi-channels", "platform", "automation-editor", "page"] }),
    ).toHaveLength(1),
  );
});

it("编辑第1000条之后的已选渠道时按ID读取，保存保留完整范围", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  await screen.findByRole("checkbox", { name: "渠道 1201（1201）" });
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).includes("/lookup?id=1201"))).toBe(
    true,
  );
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(api.saveAccountModelAutomationSchedule).toHaveBeenCalledWith(
      expect.objectContaining({ channels: schedule.request.channels }),
    ),
  );
});

it("翻页读取失败时保留已选项和草稿，重试只读取当前页", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  const name = await screen.findByRole("textbox", { name: "运营任务名称" });
  await user.type(name, "草稿");
  vi.mocked(fetch).mockRejectedValueOnce(new Error("分页暂不可用"));
  const channels = screen.getByRole("group", { name: "目标渠道" });
  await user.click(within(channels).getByRole("button", { name: "转到下一页" }));
  await user.click(await within(channels).findByRole("button", { name: "重新读取" }));
  await within(channels).findByRole("checkbox", { name: "渠道 51（51）" });
  expect(name).toHaveValue("跨页任务草稿");
  expect(within(channels).getByRole("checkbox", { name: "渠道 1201（1201）" })).toBeChecked();
});

it("翻页读取期间禁止修改已选渠道和重复翻页，读取完成后恢复操作", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled());
  let resolve!: (response: Response) => void;
  vi.mocked(fetch).mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const channels = screen.getByRole("group", { name: "目标渠道" });
  await user.click(within(channels).getByRole("button", { name: "转到下一页" }));
  await waitFor(() => expect(channels).toHaveAttribute("aria-disabled", "true"));
  expect(within(channels).getByRole("checkbox", { name: "渠道 1201（1201）" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(within(channels).getByRole("button", { name: "转到上一页" })).toBeDisabled();
  expect(within(channels).getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(within(channels).getByRole("button", { name: "重新读取渠道" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  await act(async () =>
    resolve(new Response(JSON.stringify({ items: [channel("51")], total: 1301 }))),
  );
  await waitFor(() =>
    expect(within(channels).getByRole("button", { name: "转到上一页" })).toBeEnabled(),
  );
  expect(channels).not.toHaveAttribute("aria-disabled", "true");
  expect(within(channels).getByRole("checkbox", { name: "渠道 1201（1201）" })).not.toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(within(channels).getByRole("button", { name: "重新读取渠道" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled();
});

it("关闭编辑弹窗时中止已选渠道详情请求且迟到结果不重新打开", async () => {
  let resolve!: (response: Response) => void;
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation((input, init) =>
    String(input).includes("/lookup")
      ? new Promise((done) => {
          resolve = done;
        })
      : base(input, init),
  );
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  const pending = vi
    .mocked(fetch)
    .mock.calls.find(([url]) => String(url).includes("/lookup"))![1]?.signal;
  await user.keyboard("{Escape}");
  await waitFor(() => expect(pending?.aborted).toBe(true));
  await act(async () =>
    resolve(new Response(JSON.stringify({ items: [channel("1201")], failures: [] }))),
  );
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("当前页刷新得到已选渠道新版本后翻页，保存仍使用已读取的新版本", async () => {
  const editing = {
    ...schedule,
    request: { ...schedule.request, channels: [{ id: "1", version: "a".repeat(64) }] },
  };
  vi.mocked(api.accountModelAutomationSchedules).mockResolvedValue([editing]);
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled());
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const response = await base(input, init);
    if (String(input).includes("page=0")) {
      const data = (await response.json()) as { items: NewAPIChannel[]; total: number };
      return new Response(
        JSON.stringify({
          ...data,
          items: data.items.map((item) =>
            item.id === "1" ? { ...item, version: "b".repeat(64) } : item,
          ),
        }),
      );
    }
    return response;
  });
  await act(async () => {
    await client.refetchQueries({
      queryKey: ["newapi-channels", "platform", "automation-editor", "page"],
    });
  });
  const channels = screen.getByRole("group", { name: "目标渠道" });
  await user.click(within(channels).getByRole("button", { name: "转到下一页" }));
  await within(channels).findByRole("checkbox", { name: "渠道 51（51）" });
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(api.saveAccountModelAutomationSchedule).toHaveBeenCalledWith(
      expect.objectContaining({ channels: [{ id: "1", version: "b".repeat(64) }] }),
    ),
  );
});

it("总数未知时允许继续翻页，末页不足一页后停止向后翻页", async () => {
  vi.mocked(fetch).mockImplementation(async (input) => {
    const url = new URL(String(input), "http://localhost");
    const page = Number(url.searchParams.get("page"));
    return new Response(
      JSON.stringify({
        items: Array.from({ length: page === 0 ? 50 : 2 }, (_, i) =>
          channel(String(page * 50 + i + 1)),
        ),
        total: -1,
      }),
    );
  });
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "添加任务" }));
  const channels = await screen.findByRole("group", { name: "目标渠道" });
  await user.click(within(channels).getByRole("button", { name: "转到下一页" }));
  await within(channels).findByRole("checkbox", { name: "渠道 52（52）" });
  expect(within(channels).getByRole("button", { name: "转到下一页" })).toBeDisabled();
  expect(within(channels).getByRole("button", { name: "转到上一页" })).toBeEnabled();
});

it("已知目录只有一页时不展示无用分页控件", async () => {
  vi.mocked(fetch).mockImplementation(
    async () => new Response(JSON.stringify({ items: [channel("1")], total: 1 })),
  );
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "添加任务" }));
  const channels = await screen.findByRole("group", { name: "目标渠道" });
  expect(within(channels).getByRole("checkbox", { name: "渠道 1（1）" })).toBeVisible();
  expect(within(channels).queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
});
