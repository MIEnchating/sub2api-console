import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import type { AccountModelAutomationSchedule } from "@/api";
import { AccountModelAutomationTaskGroupDialog } from "../account-model-automation-task-group-dialog";

const schedules: AccountModelAutomationSchedule[] = [
  {
    id: "a".repeat(32),
    name: "第一项任务",
    request: {
      group_ids: ["7"],
      platform_id: "platform",
      channels: [],
      selected_models: ["model-a"],
      publish_missing: true,
      price_enabled: false,
      pricing_source: "official",
    },
    enabled: true,
    schedule_type: "interval",
    daily_times: [],
    timezone: "",
    interval_minutes: 60,
    last_run_at: null,
    next_run_at: null,
  },
  {
    id: "b".repeat(32),
    name: "第二项任务",
    request: {
      group_ids: ["7"],
      platform_id: "platform",
      channels: [],
      selected_models: ["model-b"],
      publish_missing: true,
      price_enabled: false,
      pricing_source: "official",
    },
    enabled: false,
    schedule_type: "interval",
    daily_times: [],
    timezone: "",
    interval_minutes: 60,
    last_run_at: null,
    next_run_at: null,
  },
];

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

it("选择两个运营任务并保存时按任务列表顺序提交", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn();
  render(
    <AccountModelAutomationTaskGroupDialog
      open
      schedules={schedules}
      pending={false}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />,
  );

  await user.type(screen.getByRole("textbox", { name: "运营任务组名称" }), "连续执行");
  await user.click(screen.getByRole("checkbox", { name: /第二项任务/ }));
  await user.click(screen.getByRole("checkbox", { name: /第一项任务/ }));
  await user.click(screen.getByRole("button", { name: "保存任务组" }));

  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0]?.[0]).toEqual({
    name: "连续执行",
    schedule_ids: [schedules[0]!.id, schedules[1]!.id],
  });
});

it("任务组名称恰好 128 个 Unicode 字符时允许保存", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn();
  render(
    <AccountModelAutomationTaskGroupDialog
      open
      schedules={schedules}
      pending={false}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />,
  );

  fireEvent.change(screen.getByRole("textbox", { name: "运营任务组名称" }), {
    target: { value: "😀".repeat(128) },
  });
  await user.click(screen.getByRole("checkbox", { name: /第一项任务/ }));
  await user.click(screen.getByRole("checkbox", { name: /第二项任务/ }));
  await user.click(screen.getByRole("button", { name: "保存任务组" }));

  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0]?.[0]).toEqual({
    name: "😀".repeat(128),
    schedule_ids: [schedules[0]!.id, schedules[1]!.id],
  });
});

it("任务组名称达到 129 个 Unicode 字符时显示字段错误且不提交", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn();
  render(
    <AccountModelAutomationTaskGroupDialog
      open
      schedules={schedules}
      pending={false}
      onOpenChange={vi.fn()}
      onSave={onSave}
    />,
  );

  fireEvent.change(screen.getByRole("textbox", { name: "运营任务组名称" }), {
    target: { value: "😀".repeat(129) },
  });
  await user.click(screen.getByRole("checkbox", { name: /第一项任务/ }));
  await user.click(screen.getByRole("checkbox", { name: /第二项任务/ }));
  await user.click(screen.getByRole("button", { name: "保存任务组" }));

  expect(screen.getByRole("textbox", { name: "运营任务组名称" })).toHaveAttribute(
    "aria-invalid",
    "true",
  );
  expect(screen.getByText("任务组名称不能超过 128 个字符")).toBeVisible();
  expect(onSave).not.toHaveBeenCalled();
});

it("后台更新任务名称和运行时间时保留任务组名称与勾选草稿", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn();
  const props = { open: true, schedules, pending: false, onOpenChange: vi.fn(), onSave };
  const view = render(<AccountModelAutomationTaskGroupDialog {...props} />);
  await user.type(screen.getByRole("textbox", { name: "运营任务组名称" }), "未保存草稿");
  await user.click(screen.getByRole("checkbox", { name: /第一项任务/ }));
  await user.click(screen.getByRole("checkbox", { name: /第二项任务/ }));

  view.rerender(
    <AccountModelAutomationTaskGroupDialog
      {...props}
      schedules={schedules.map((schedule) => ({ ...schedule, name: `${schedule.name}已更新` }))}
    />,
  );

  expect(screen.getByRole("textbox", { name: "运营任务组名称" })).toHaveValue("未保存草稿");
  expect(screen.getByRole("checkbox", { name: /第一项任务已更新/ })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: /第二项任务已更新/ })).toBeChecked();
  await user.click(screen.getByRole("button", { name: "保存任务组" }));
  await waitFor(() => expect(onSave).toHaveBeenCalled());
  expect(onSave.mock.calls[0]?.[0]).toEqual({
    name: "未保存草稿",
    schedule_ids: schedules.map((schedule) => schedule.id),
  });
});

it("后台移除已选任务时保留名称草稿并禁止提交不足两项的任务组", async () => {
  const user = userEvent.setup();
  const onSave = vi.fn();
  const props = { open: true, schedules, pending: false, onOpenChange: vi.fn(), onSave };
  const view = render(<AccountModelAutomationTaskGroupDialog {...props} />);
  await user.type(screen.getByRole("textbox", { name: "运营任务组名称" }), "保留名称");
  await user.click(screen.getByRole("checkbox", { name: /第一项任务/ }));
  await user.click(screen.getByRole("checkbox", { name: /第二项任务/ }));

  view.rerender(<AccountModelAutomationTaskGroupDialog {...props} schedules={[schedules[1]!]} />);

  expect(screen.getByRole("textbox", { name: "运营任务组名称" })).toHaveValue("保留名称");
  expect(screen.getByRole("checkbox", { name: /第二项任务/ })).toBeChecked();
  expect(screen.queryByRole("checkbox", { name: /第一项任务/ })).not.toBeInTheDocument();
  expect(screen.getByText("已选择 1 个")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "保存任务组" }));
  expect(await screen.findByText("请至少选择两个运营任务")).toBeVisible();
  expect(onSave).not.toHaveBeenCalled();
});
