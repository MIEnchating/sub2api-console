import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { api, type GroupStatus, type NewAPIChannel, type Task } from "@/api";
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

function task(id: string, status: Task["status"], dryRun: boolean): Task {
  return {
    id,
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status,
    progress: status === "succeeded" ? 100 : 0,
    message: status === "succeeded" ? "预览已完成" : "任务已排队",
    result: {
      dry_run: dryRun,
      models: ["model-a"],
      pricing_missing: 0,
      pricing_plan: [
        {
          model: "model-a",
          input_ratio: "0.5",
          completion_ratio: "2",
          cache_ratio: "0.1",
          billing_expr: 'tier("peak", p * 0.5 + c * 2)',
        },
      ],
      items: [
        { channel_id: "11", status: "succeeded", added: ["model-a"], message: "预览：将新增模型" },
      ],
    },
    created_at: "2026-09-30T00:00:00Z",
    updated_at: "2026-09-30T00:00:00Z",
  };
}

let client: QueryClient | undefined;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
});
afterEach(() => {
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function selectModel(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "要上架的模型" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    ),
  );
  await user.click(screen.getByRole("combobox", { name: "要上架的模型" }));
  await user.click(await screen.findByRole("option", { name: "model-a" }));
  await user.keyboard("{Escape}");
}

function renderDialog() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <AccountModelAutomationDialog
        platformId="platform"
        channels={[channel]}
        open
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
}

const group: GroupStatus = {
  id: "7",
  name: "默认分组",
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

it("预览完成并显示渠道新增模型后，确认写入携带预览任务 ID", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [{ id: "7", name: "默认分组", ratio: null }],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "默认分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([]);
  const enqueue = vi
    .spyOn(api, "enqueueAccountModelAutomation")
    .mockResolvedValueOnce(task("discovery", "succeeded", true))
    .mockResolvedValueOnce(task("preview", "queued", true))
    .mockResolvedValueOnce(task("write", "queued", false));
  vi.spyOn(api, "task").mockResolvedValue(task("preview", "succeeded", true));

  renderDialog();
  await user.click(await screen.findByRole("checkbox", { name: /默认分组/ }));
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  await selectModel(user);
  await user.click(screen.getByRole("button", { name: "预览" }));

  expect(await screen.findByRole("region", { name: "运营预览结果" })).toHaveTextContent(
    "待上架（1 个）model-a",
  );
  expect(screen.getByLabelText("模型定价方案")).toHaveTextContent("缓存倍率 0.1");
  expect(screen.getByLabelText("模型定价方案")).toHaveTextContent("分时计费表达式");
  await user.click(screen.getByRole("button", { name: "确认写入" }));
  await waitFor(() => expect(enqueue).toHaveBeenCalledTimes(3));
  expect(enqueue).toHaveBeenNthCalledWith(
    2,
    expect.objectContaining({ dry_run: true, selected_models: ["model-a"] }),
  );
  expect(enqueue).toHaveBeenNthCalledWith(
    3,
    expect.objectContaining({ dry_run: false, preview_task_id: "preview" }),
  );
});

it("预览后改变目标渠道选择会撤销确认入口", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [{ id: "7", name: "默认分组", ratio: null }],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "默认分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([]);
  vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(task("preview", "queued", true));
  vi.spyOn(api, "task").mockResolvedValue(task("preview", "succeeded", true));

  renderDialog();
  await user.click(await screen.findByRole("checkbox", { name: /默认分组/ }));
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  await selectModel(user);
  await user.click(screen.getByRole("button", { name: "预览" }));
  expect(await screen.findByRole("button", { name: "确认写入" })).toBeEnabled();

  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  expect(screen.queryByRole("button", { name: "确认写入" })).not.toBeInTheDocument();
});

it("平台绑定分组时只显示已绑定的账号分组", async () => {
  vi.spyOn(api, "groups").mockResolvedValue([group, { ...group, id: "8", name: "未绑定分组" }]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [
      { id: "7", name: "默认分组", ratio: null },
      { id: "8", name: "未绑定分组", ratio: null },
    ],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "默认分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([]);

  renderDialog();

  expect(await screen.findByRole("checkbox", { name: /默认分组/ })).toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: /未绑定分组/ })).not.toBeInTheDocument();
  expect(screen.getByText("账号分组")).toBeInTheDocument();
});

it("添加弹窗按执行范围分组展示选择项并汇总所选数量", async () => {
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [{ id: "7", name: "默认分组", ratio: null }],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "默认分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([]);

  renderDialog();

  expect(await screen.findByRole("region", { name: "执行范围" })).toHaveTextContent(
    "已选择 0 个分组 · 0 个渠道",
  );
  expect(screen.getByRole("group", { name: "账号分组" })).toBeInTheDocument();
  expect(screen.getByRole("group", { name: "目标渠道" })).toBeInTheDocument();
  expect(screen.getByRole("region", { name: "同步策略" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
});

it("运行间隔超出允许范围时禁用保存任务", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [{ id: "7", name: "默认分组", ratio: null }],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "默认分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([]);

  renderDialog();
  await user.click(await screen.findByRole("checkbox", { name: /默认分组/ }));
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  await user.type(screen.getByRole("textbox", { name: "运营任务名称" }), "生产同步");
  await user.clear(screen.getByRole("spinbutton", { name: "运行间隔分钟" }));
  await user.type(screen.getByRole("spinbutton", { name: "运行间隔分钟" }), "10081");

  expect(screen.getByText("运行间隔需为 1 到 10080 分钟之间的整数。")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
});

it("保存运营任务时提交所选分组渠道和独立运行配置", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [{ id: "7", name: "默认分组", ratio: null }],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "默认分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue([]);
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue({
    id: "schedule-1",
    name: "生产同步",
    request: {
      group_ids: ["7"],
      platform_id: "platform",
      channels: [{ id: "11", version: channel.version }],
      selected_models: ["model-a"],
      publish_missing: true,
      price_enabled: true,
      pricing_source: "official",
    },
    enabled: true,
    interval_minutes: 45,
    last_run_at: null,
    next_run_at: "2026-09-30T00:45:00Z",
  });

  vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(
    task("discovery", "succeeded", true),
  );
  vi.spyOn(api, "task").mockResolvedValue(task("discovery", "succeeded", true));
  renderDialog();
  await user.click(await screen.findByRole("checkbox", { name: /默认分组/ }));
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  await user.type(screen.getByRole("textbox", { name: "运营任务名称" }), "生产同步");
  await user.clear(screen.getByRole("spinbutton", { name: "运行间隔分钟" }));
  await user.type(screen.getByRole("spinbutton", { name: "运行间隔分钟" }), "45");
  await selectModel(user);
  await user.click(screen.getByRole("button", { name: "保存任务" }));

  await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  expect(save).toHaveBeenCalledWith({
    name: "生产同步",
    group_ids: ["7"],
    platform_id: "platform",
    channels: [{ id: "11", version: channel.version }],
    selected_models: ["model-a"],
    publish_missing: true,
    price_enabled: true,
    pricing_source: "official",
    enabled: true,
    interval_minutes: 45,
  });
});
