import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import {
  api,
  type AccountModelAutomationSchedule,
  type GroupStatus,
  type NewAPIWorkspace,
  type Task,
} from "@/api";
import { AccountModelAutomationPage } from "../account-model-automation-page";

const workspace: NewAPIWorkspace = {
  platforms: ["a", "b"].map((id) => ({
    id,
    name: `平台 ${id}`,
    base_url: `https://${id}.example.test`,
    user_id: "1",
    admin_key_configured: true,
    updated_at: "2026-10-09T00:00:00Z",
  })),
  local_groups: [{ id: "7", name: "生产组", ratio: null }],
  bindings: [
    {
      platform_id: "b",
      newapi_group_id: "remote-7",
      newapi_group_name: "生产组",
      sub2api_group_id: "7",
      sync_ratio: true,
    },
  ],
  sub2api_base_url: "",
};
const schedule: AccountModelAutomationSchedule = {
  id: "a".repeat(32),
  name: "跨平台任务",
  request: {
    platform_id: "b",
    group_ids: ["7"],
    channels: [],
    selected_models: [],
    publish_missing: false,
    price_enabled: false,
    pricing_source: "official",
  },
  enabled: true,
  schedule_type: "interval",
  interval_minutes: 60,
  last_run_at: null,
  next_run_at: null,
};
const group: GroupStatus = {
  id: "7",
  name: "生产组",
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "default",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
};
let client: QueryClient;

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue(workspace);
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([schedule]);
  vi.spyOn(api, "accountModelAutomationTaskGroups").mockResolvedValue([]);
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(globalThis, "fetch").mockImplementation(
    async () => new Response(JSON.stringify({ items: [], total: 0 }), { status: 200 }),
  );
});
afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
});

function renderPage(): ReturnType<typeof render> {
  return render(
    <QueryClientProvider client={client}>
      <AccountModelAutomationPage />
    </QueryClientProvider>,
  );
}

it("进入运营列表时不预读渠道且可直接打开添加入口", async () => {
  renderPage();
  await screen.findByText("跨平台任务");
  await waitFor(() => expect(screen.getByRole("button", { name: "添加任务" })).toBeEnabled());
  expect(fetch).not.toHaveBeenCalled();
});

it("在运营列表手动运行已保存任务时提交该任务的稳定 ID", async () => {
  const started: Task = {
    id: "running-task",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "queued",
    progress: 0,
    message: "任务已排队",
    result: {},
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
  const run = vi.spyOn(api, "runAccountModelAutomationSchedule").mockResolvedValue(started);
  vi.spyOn(api, "task").mockResolvedValue(started);
  const user = userEvent.setup();
  renderPage();

  const actions = await screen.findByRole("group", { name: "跨平台任务 的操作" });
  await user.click(within(actions).getByRole("button", { name: "运行" }));

  await waitFor(() => expect(run).toHaveBeenCalledWith(schedule.id));
});

it.each(["取消", "Escape"])(
  "编辑渠道读取期间通过 %s 关闭时中止请求且迟到分页不重新打开弹窗",
  async (action) => {
    let resolve!: (response: Response) => void;
    vi.mocked(fetch).mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: "编辑" }));
    const dialog = await screen.findByRole("dialog", { name: "编辑运营任务" });
    expect(within(dialog).getByRole("status", { name: "正在读取渠道" })).toBeVisible();
    expect(within(dialog).queryByRole("button", { name: "保存任务" })).not.toBeInTheDocument();
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    expect(String(vi.mocked(fetch).mock.calls[0]?.[0])).toContain("/platforms/b/channels");
    const signal = vi.mocked(fetch).mock.calls[0]?.[1]?.signal;
    if (action === "Escape") await user.keyboard("{Escape}");
    else await user.click(within(dialog).getByRole("button", { name: "取消" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(signal?.aborted).toBe(true);
    await act(async () => {
      resolve(
        new Response(
          JSON.stringify({
            items: Array.from({ length: 100 }, (_, index) => ({ id: String(index + 1) })),
            total: 200,
          }),
          { status: 200 },
        ),
      );
    });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  },
);

it("渠道首次读取失败时保留取消和重试入口并在重试成功后显示原编辑内容", async () => {
  const notify = vi.spyOn(toast, "error");
  vi.mocked(fetch).mockRejectedValueOnce(new Error("渠道服务暂时不可用"));
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  const dialog = await screen.findByRole("dialog", { name: "编辑运营任务" });
  await user.click(await within(dialog).findByRole("button", { name: "重新读取" }));
  expect(await within(dialog).findByRole("textbox", { name: "运营任务名称" })).toHaveValue(
    "跨平台任务",
  );
  await waitFor(() =>
    expect(within(dialog).getByRole("textbox", { name: "运营任务名称" })).toHaveFocus(),
  );
  expect(notify).toHaveBeenCalledTimes(1);
  expect(screen.queryByText("渠道服务暂时不可用")).not.toBeInTheDocument();
});

it("渠道后台刷新期间保留编辑草稿与输入焦点", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  const name = await screen.findByRole("textbox", { name: "运营任务名称" });
  await user.clear(name);
  await user.type(name, "保留草稿");
  let resolve!: (response: Response) => void;
  vi.mocked(fetch).mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  act(() => {
    void client.invalidateQueries({ queryKey: ["newapi-channels", "b"] });
  });
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(name).toHaveValue("保留草稿");
  expect(name).toHaveFocus();
  expect(screen.queryByRole("status", { name: "正在读取渠道" })).not.toBeInTheDocument();
  await act(async () => {
    resolve(new Response(JSON.stringify({ items: [], total: 0 }), { status: 200 }));
  });
  expect(name).toHaveValue("保留草稿");
});

it("渠道加载后的预览运行中按 Escape 关闭外层弹窗时取消临时任务", async () => {
  const preview: Task = {
    id: "editor-preview",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "running",
    progress: 0,
    message: "正在预览运营任务",
    result: { dry_run: true },
    created_at: "2026-10-09T00:00:00Z",
    updated_at: "2026-10-09T00:00:00Z",
  };
  vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(preview);
  vi.spyOn(api, "task").mockResolvedValue(preview);
  vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  const previewButton = await screen.findByRole("button", { name: "预览" });
  await waitFor(() => expect(previewButton).toBeEnabled());
  await user.click(previewButton);
  await screen.findByRole("status", { name: "正在预览运营任务" });
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(api.cancelTask).toHaveBeenCalledWith(preview.id));
});

it("任务组首次读取时显示骨架并在失败后提供局部重试", async () => {
  let reject!: (error: Error) => void;
  vi.mocked(api.accountModelAutomationTaskGroups).mockImplementationOnce(
    () =>
      new Promise((_, fail) => {
        reject = fail;
      }),
  );
  const user = userEvent.setup();
  renderPage();
  const region = await screen.findByRole("region", { name: "运营任务组" });
  expect(within(region).getByRole("status", { name: "正在读取任务组" })).toHaveAttribute(
    "data-slot",
    "page-loading-skeleton",
  );
  await act(async () => {
    reject(new Error("任务组读取失败"));
  });
  await user.click(await within(region).findByRole("button", { name: "重新读取" }));
  await within(region).findByText("选择两个或更多运营任务后，可创建任务组连续执行。");
});

it("渠道刷新失败时保留草稿并禁止提交，重试成功后恢复提交入口", async () => {
  const user = userEvent.setup();
  renderPage();
  await user.click(await screen.findByRole("button", { name: "编辑" }));
  const name = await screen.findByRole("textbox", { name: "运营任务名称" });
  await user.type(name, "保留草稿");
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled());
  let reject!: (error: Error) => void;
  vi.mocked(fetch).mockImplementationOnce(
    () =>
      new Promise((_, fail) => {
        reject = fail;
      }),
  );
  await user.click(screen.getByRole("button", { name: "重新读取渠道" }));
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
  await act(async () => reject(new Error("刷新渠道失败")));
  expect(name).toHaveValue("跨平台任务保留草稿");
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "重新读取渠道" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled());
  expect(name).toHaveValue("跨平台任务保留草稿");
});
