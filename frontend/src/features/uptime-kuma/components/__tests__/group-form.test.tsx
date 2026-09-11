import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MonitorDialog } from "../monitor-dialog";
import { monitor } from "./fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

it("没有现有分组时可创建顶层分组，提交不包含请求模板或请求参数", async () => {
  const submit = vi.fn();
  const user = userEvent.setup();
  render(
    <MonitorDialog
      monitor={null}
      initialType="group"
      monitors={[]}
      pending={false}
      onClose={vi.fn()}
      onSubmit={submit}
    />,
  );
  await user.type(screen.getByLabelText("分组名称"), "接口服务");
  expect(screen.getByRole("combobox", { name: "所属分组" })).toHaveTextContent("无分组");
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "group",
        name: "接口服务",
        parent: null,
        template_id: "",
        options: undefined,
      }),
    ),
  );
});

it("编辑分组时只显示分组字段并保留原有所属分组", async () => {
  const submit = vi.fn();
  const user = userEvent.setup();
  const parent = { ...monitor, id: 7, type: "group", name: "父分组" };
  const group = { ...monitor, type: "group", parent: 7 };
  render(
    <MonitorDialog
      monitor={group}
      monitors={[parent, group]}
      pending={false}
      onClose={vi.fn()}
      onSubmit={submit}
    />,
  );
  expect(screen.getByRole("dialog", { name: "编辑分组" })).toBeVisible();
  expect(screen.queryByRole("combobox", { name: "功能模板" })).not.toBeInTheDocument();
  expect(screen.queryByRole("region", { name: "检测设置" })).not.toBeInTheDocument();
  await user.clear(screen.getByLabelText("分组名称"));
  await user.type(screen.getByLabelText("分组名称"), "新名称");
  await user.click(screen.getByRole("button", { name: "保存分组" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ type: "group", name: "新名称", parent: 7 }),
    ),
  );
});

it("分组保存中禁用输入和提交并在弹窗底部显示任务进度", () => {
  render(
    <MonitorDialog
      monitor={null}
      initialType="group"
      monitors={[]}
      pending
      task={{ message: "正在创建分组", progress: 50 }}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );
  expect(screen.getByLabelText("分组名称")).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "所属分组" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "正在保存…" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  expect(screen.getByText("正在创建分组").closest('[data-slot="dialog-footer"]')).not.toBeNull();
});
