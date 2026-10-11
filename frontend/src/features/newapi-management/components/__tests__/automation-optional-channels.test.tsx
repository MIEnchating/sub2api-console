import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
const schedule: AccountModelAutomationSchedule = {
  id: "schedule-1",
  name: "账号模型同步",
  enabled: true,
  interval_minutes: 60,
  last_run_at: null,
  next_run_at: null,
  request: {
    group_ids: ["7"],
    platform_id: "platform",
    channels: [],
    publish_missing: true,
    price_enabled: false,
    pricing_source: "official",
  },
};
let client: QueryClient;

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
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
});
afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderDialog(
  channels: NewAPIChannel[] = [channel],
  editing?: AccountModelAutomationSchedule,
): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountModelAutomationDialog
        platformId="platform"
        channels={channels}
        schedule={editing}
        open
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
}

it("没有可用渠道时，只选分组和名称也能保存且不启用价格同步", async () => {
  const user = userEvent.setup();
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog([]);
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.type(screen.getByRole("textbox", { name: "运营任务名称" }), "账号模型同步");
  expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        group_ids: ["7"],
        channels: [],
        price_enabled: false,
      }),
    ),
  );
});

it("运营任务名称超过后端 128 字符上限时禁用保存并提示长度", async () => {
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog([]);
  fireEvent.change(screen.getByRole("textbox", { name: "运营任务名称" }), {
    target: { value: "中".repeat(129) },
  });
  await userEvent.setup().click(await screen.findByRole("checkbox", { name: /生产分组/ }));

  expect(screen.getByRole("textbox", { name: "运营任务名称" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByText("任务名称不能超过 128 个字符")).toBeVisible();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  expect(save).not.toHaveBeenCalled();
});

it("运营任务名称恰好 128 个 Unicode 字符时允许保存", async () => {
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog([]);
  fireEvent.change(screen.getByRole("textbox", { name: "运营任务名称" }), {
    target: { value: "😀".repeat(128) },
  });
  await userEvent.setup().click(await screen.findByRole("checkbox", { name: /生产分组/ }));

  expect(screen.getByRole("textbox", { name: "运营任务名称" })).toHaveAttribute(
    "aria-invalid",
    "false",
  );
  expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled();
  await userEvent.setup().click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "😀".repeat(128),
      }),
    ),
  );
});

it("未选择渠道时，预览请求只包含分组并关闭价格同步", async () => {
  const user = userEvent.setup();
  const preview: Task = {
    id: "preview",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "succeeded",
    progress: 100,
    message: "预览已完成",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
    result: {
      dry_run: true,
      models: ["model-a"],
      items: [{ account_id: "10", status: "succeeded" }],
    },
  };
  const enqueue = vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(preview);
  vi.spyOn(api, "task").mockResolvedValue(preview);
  renderDialog();
  expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  expect(screen.getByRole("button", { name: "预览" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "预览" }));
  await waitFor(() =>
    expect(enqueue).toHaveBeenCalledWith(
      expect.objectContaining({
        group_ids: ["7"],
        channels: [],
        price_enabled: false,
        dry_run: true,
      }),
    ),
  );
  expect(await screen.findByRole("region", { name: "运营预览结果" })).toHaveTextContent(
    "模型 1 个",
  );
  expect(screen.queryByText(/待定价/)).not.toBeInTheDocument();
  expect(screen.queryByLabelText("模型定价方案")).not.toBeInTheDocument();
});

it("选择渠道后可同步价格，取消最后一个渠道后价格同步变为未选中且禁用", async () => {
  const user = userEvent.setup();
  renderDialog();
  const price = screen.getByRole("checkbox", { name: "自动同步模型价格" });
  expect(price).not.toBeChecked();
  expect(price).toHaveAttribute("aria-disabled", "true");
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  expect(price).not.toHaveAttribute("aria-disabled", "true");
  expect(price).toBeChecked();
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  expect(price).not.toBeChecked();
  expect(price).toHaveAttribute("aria-disabled", "true");
});

it("编辑已有任务并取消全部渠道时保存空渠道范围和关闭价格同步", async () => {
  const user = userEvent.setup();
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog([channel], {
    ...schedule,
    request: {
      ...schedule.request,
      channels: [{ id: channel.id, version: channel.version }],
      price_enabled: true,
    },
  });
  await screen.findByRole("checkbox", { name: /生产分组/ });
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "schedule-1",
        channels: [],
        price_enabled: false,
      }),
    ),
  );
});

it("读取可用模型后不默认全选，只保存手动勾选的模型", async () => {
  const user = userEvent.setup();
  const discovered: Task = {
    id: "discovery",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "succeeded",
    progress: 100,
    message: "读取完成",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
    result: { dry_run: true, models: ["model-a", "model-b"], items: [] },
  };
  const enqueue = vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(discovered);
  vi.spyOn(api, "task").mockResolvedValue(discovered);
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("checkbox", { name: /生产渠道/ }));
  await user.type(screen.getByRole("textbox", { name: "运营任务名称" }), "指定模型");
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await waitFor(() =>
    expect(enqueue).toHaveBeenCalledWith(
      expect.objectContaining({
        dry_run: true,
        channels: [],
        publish_missing: false,
        price_enabled: false,
        selected_models: [],
      }),
    ),
  );
  const selector = await screen.findByRole("combobox", { name: "要上架的模型" });
  expect(screen.queryByRole("button", { name: "确认写入" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  selector.focus();
  await user.keyboard("{Enter}");
  const model = await screen.findByRole("option", { name: "model-b" });
  expect(model).toHaveAttribute("aria-selected", "false");
  await user.click(model);
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        selected_models: ["model-b"],
        publish_missing: true,
      }),
    ),
  );
});

it("未选择渠道且未开启上架时仍显示读取入口，选择分组后可读取模型", async () => {
  const user = userEvent.setup();
  const discovered: Task = {
    id: "discovery-without-channel",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status: "succeeded",
    progress: 100,
    message: "读取完成",
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
    result: { dry_run: true, models: ["model-b"], items: [] },
  };
  const enqueue = vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(discovered);
  vi.spyOn(api, "task").mockResolvedValue(discovered);
  renderDialog([], {
    ...schedule,
    request: { ...schedule.request, group_ids: [], publish_missing: false },
  });
  const read = screen.getByRole("button", { name: "读取可用模型" });
  expect(read).toBeDisabled();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  expect(read).toBeEnabled();
  await user.click(read);
  const selector = await screen.findByRole("combobox", { name: "要上架的模型" });
  await waitFor(() => expect(selector).not.toHaveAttribute("aria-disabled", "true"));
  await user.click(selector);
  expect(await screen.findByRole("option", { name: "model-b" })).toHaveAttribute(
    "aria-selected",
    "false",
  );
  expect(enqueue).toHaveBeenCalledWith(
    expect.objectContaining({
      channels: [],
      publish_missing: false,
      price_enabled: false,
      dry_run: true,
    }),
  );
});

it("每天指定时间允许添加和删除时间，保存全部时间点及北京时间", async () => {
  const user = userEvent.setup();
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog([], schedule);
  await user.click(await screen.findByRole("radio", { name: "指定时段" }));
  expect(screen.queryByRole("spinbutton", { name: "运行间隔分钟" })).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("每天运行时间"), { target: { value: "08:30" } });
  await user.click(screen.getByRole("button", { name: "添加时间" }));
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("每天运行时间 2"), { target: { value: "20:00" } });
  await user.click(screen.getByRole("button", { name: "添加时间" }));
  await user.click(screen.getByRole("button", { name: "删除每天运行时间 3" }));
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        schedule_type: "daily",
        daily_times: ["08:30", "20:00"],
        timezone: "Asia/Shanghai",
      }),
    ),
  );
});

it("编辑每天任务回填时间，重复时间禁止保存且切回间隔不提交每日配置", async () => {
  const user = userEvent.setup();
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockResolvedValue(schedule);
  renderDialog([], {
    ...schedule,
    schedule_type: "daily",
    daily_times: ["09:00", "20:00"],
    timezone: "Asia/Shanghai",
  });
  expect(screen.getByRole("radio", { name: "指定时段" })).toBeChecked();
  expect(screen.getByLabelText("每天运行时间 2")).toHaveValue("20:00");
  fireEvent.change(screen.getByLabelText("每天运行时间 2"), { target: { value: "09:00" } });
  expect(screen.getByRole("button", { name: "保存任务" })).toBeDisabled();
  expect(screen.getByLabelText("每天运行时间 2")).toHaveAttribute("aria-invalid", "true");
  await user.click(screen.getByRole("radio", { name: "按间隔" }));
  await user.click(screen.getByRole("button", { name: "保存任务" }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0]).not.toHaveProperty("daily_times");
  expect(save.mock.calls[0][0]).not.toHaveProperty("timezone");
});
