import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type AccountModelAutomationSchedule, type NewAPIChannel, type Task } from "@/api";
import { AccountModelAutomationDialog } from "../account-model-automation-dialog";

const channel: NewAPIChannel = {
  id: "11",
  name: "生产渠道",
  type: 1,
  status: 1,
  models: [],
  groups: [],
  version: "a".repeat(64),
};
const other: NewAPIChannel = { ...channel, id: "22", name: "备用渠道" };
const schedule: AccountModelAutomationSchedule = {
  id: "schedule-1",
  name: "生产模型同步",
  enabled: true,
  interval_minutes: 60,
  last_run_at: null,
  next_run_at: null,
  request: {
    group_ids: ["7"],
    platform_id: "platform",
    channels: [{ id: channel.id, version: channel.version }],
    selected_models: ["model-a"],
    publish_missing: true,
    price_enabled: true,
    pricing_source: "official",
  },
};
const preview: Task = {
  id: "preview-old",
  skill: "sub2api-account-model-automation",
  operation: "account-model-automation",
  status: "succeeded",
  progress: 100,
  message: "预览完成",
  result: { dry_run: true, models: ["model-a"], items: [] },
  created_at: "2026-10-09T00:00:00Z",
  updated_at: "2026-10-09T00:00:00Z",
};
let client: QueryClient;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
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
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [],
    sub2api_base_url: "",
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "生产分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
  });
  vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(preview);
  vi.spyOn(api, "task").mockResolvedValue(preview);
});
afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function editor(channels: NewAPIChannel[], editing = schedule): ReactElement {
  return (
    <QueryClientProvider client={client}>
      <AccountModelAutomationDialog
        open
        platformId="platform"
        channels={channels}
        schedule={editing}
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>
  );
}

it("已选渠道未读取到时禁止保存和预览，显式移除后才提交剩余范围", async () => {
  const user = userEvent.setup();
  render(
    editor([channel], {
      ...schedule,
      request: {
        ...schedule.request,
        channels: [
          { id: channel.id, version: channel.version },
          { id: other.id, version: other.version },
        ],
      },
    }),
  );
  await screen.findByRole("checkbox", { name: /生产分组/ });
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
  expect(screen.getByRole("group", { name: "目标渠道" })).toHaveAttribute("aria-invalid", "true");
  await user.click(screen.getByRole("button", { name: "移除未读取到的渠道 22" }));
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(api.saveAccountModelAutomationSchedule).toHaveBeenCalledWith(
      expect.objectContaining({ channels: [{ id: channel.id, version: channel.version }] }),
    ),
  );
  expect(api.enqueueAccountModelAutomation).not.toHaveBeenCalled();
});

it("目录重新出现缺失渠道时恢复提交能力并保留原选择和名称草稿", async () => {
  const user = userEvent.setup();
  const view = render(editor([]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  await user.type(screen.getByRole("textbox", { name: "运营任务名称" }), "草稿");
  view.rerender(editor([channel]));
  expect(screen.getByRole("checkbox", { name: /生产渠道/ })).toBeChecked();
  expect(screen.getByRole("textbox", { name: "运营任务名称" })).toHaveValue("生产模型同步草稿");
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(api.saveAccountModelAutomationSchedule).toHaveBeenCalledWith(
      expect.objectContaining({ channels: [{ id: channel.id, version: channel.version }] }),
    ),
  );
});

it("渠道目录重排时保存请求仍保留原选择顺序", async () => {
  const user = userEvent.setup();
  render(
    editor([other, channel], {
      ...schedule,
      request: {
        ...schedule.request,
        channels: [
          { id: channel.id, version: channel.version },
          { id: other.id, version: other.version },
        ],
      },
    }),
  );
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(api.saveAccountModelAutomationSchedule).toHaveBeenCalledWith(
      expect.objectContaining({
        channels: [
          { id: channel.id, version: channel.version },
          { id: other.id, version: other.version },
        ],
      }),
    ),
  );
});

it("已选渠道版本变化后隐藏旧确认入口，重新预览使用新版本", async () => {
  const user = userEvent.setup();
  const view = render(editor([channel]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("button", { name: "预览" }));
  await screen.findByRole("button", { name: "确认写入" });
  const updated = { ...channel, version: "b".repeat(64) };
  view.rerender(editor([updated]));
  expect(screen.queryByRole("button", { name: "确认写入" })).not.toBeInTheDocument();
  expect(screen.getByText(/重新预览后再确认写入/)).toBeVisible();
  vi.mocked(api.enqueueAccountModelAutomation).mockResolvedValue({ ...preview, id: "preview-new" });
  vi.mocked(api.task).mockResolvedValue({ ...preview, id: "preview-new" });
  await user.click(screen.getByRole("button", { name: "预览" }));
  await screen.findByRole("button", { name: "确认写入" });
  expect(api.enqueueAccountModelAutomation).toHaveBeenLastCalledWith(
    expect.objectContaining({
      channels: [{ id: updated.id, version: updated.version }],
      dry_run: true,
    }),
  );
});

it("预览创建尚未返回时渠道版本变化，迟到结果不能恢复旧确认入口", async () => {
  let resolve!: (task: Task) => void;
  vi.mocked(api.enqueueAccountModelAutomation).mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const user = userEvent.setup();
  const view = render(editor([channel]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("button", { name: "预览" }));
  view.rerender(editor([{ ...channel, version: "b".repeat(64) }]));
  await act(async () => resolve(preview));
  await screen.findByRole("region", { name: "运营预览结果" });
  expect(screen.queryByRole("button", { name: "确认写入" })).not.toBeInTheDocument();
  expect(api.enqueueAccountModelAutomation).toHaveBeenCalledTimes(1);
});

it("未选渠道变化或目录重排时保留当前有效预览", async () => {
  const user = userEvent.setup();
  const view = render(editor([channel, other]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("button", { name: "预览" }));
  await screen.findByRole("button", { name: "确认写入" });
  view.rerender(editor([{ ...other, version: "b".repeat(64) }, channel]));
  expect(screen.getByRole("button", { name: "确认写入" })).toBeEnabled();
});

it("已选分组从绑定目录消失时隐藏旧确认入口并阻止保存", async () => {
  const user = userEvent.setup();
  const view = render(editor([channel]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("button", { name: "预览" }));
  await screen.findByRole("button", { name: "确认写入" });
  client.setQueryData(["newapi-workspace", "platform"], {
    platforms: [],
    local_groups: [],
    sub2api_base_url: "",
    bindings: [],
  });
  view.rerender(editor([channel]));
  expect(screen.queryByRole("button", { name: "确认写入" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
  expect(screen.getByText(/所选分组已不在当前平台绑定中/)).toBeVisible();
});

it("预览创建期间分组成员数量变化时迟到结果不能恢复确认入口", async () => {
  let resolve!: (task: Task) => void;
  vi.mocked(api.enqueueAccountModelAutomation).mockReturnValue(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const user = userEvent.setup();
  const view = render(editor([channel]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("button", { name: "预览" }));
  client.setQueryData(["groups"], [{ ...(await api.groups())[0]!, account_count: 2 }]);
  view.rerender(editor([channel]));
  await act(async () => resolve(preview));
  await screen.findByRole("region", { name: "运营预览结果" });
  expect(screen.queryByRole("button", { name: "确认写入" })).not.toBeInTheDocument();
});

it("目录返回重复已选渠道时禁止提交有歧义的范围", async () => {
  render(editor([channel, { ...channel, version: "b".repeat(64) }]));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
});

it.each([50, 51])("已有 %i 个已选渠道时限制新增，超限必须移除后才能提交", async (count) => {
  const channels = Array.from({ length: 52 }, (_, index) => ({
    ...channel,
    id: String(index + 1),
    name: `渠道 ${index + 1}`,
  }));
  const editing = {
    ...schedule,
    request: {
      ...schedule.request,
      channels: channels.slice(0, count).map((item) => ({ id: item.id, version: item.version })),
    },
  };
  const user = userEvent.setup();
  client.setQueryData(["groups"], await api.groups());
  client.setQueryData(["newapi-workspace", "platform"], await api.newAPIWorkspace("platform"));
  render(editor(channels, editing));
  await screen.findByRole("checkbox", { name: /生产分组/ });
  expect(screen.getByRole("checkbox", { name: "渠道 52（52）" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  if (count > 50) {
    expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: "渠道 51（51）" }));
  }
  expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled();
});
