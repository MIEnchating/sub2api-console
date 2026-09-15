import { useState } from "react";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { MultiSelect } from "../../multi-select";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../../ui/dialog";

afterEach(cleanup);

function Selection(): React.ReactElement {
  const [selected, setSelected] = useState(["one", "two"]);
  return (
    <MultiSelect
      options={[
        { value: "one", label: "一组" },
        { value: "two", label: "二组" },
      ]}
      selected={selected}
      onChange={setSelected}
      title="分组"
      maxVisibleChips={1}
    />
  );
}

it.each(["{Enter}", " "])("移除按钮按 %s 时删除选项且保持选择弹层关闭", async (key) => {
  const user = userEvent.setup();
  render(<Selection />);
  screen.getByRole("button", { name: "移除" }).focus();

  await user.keyboard(key);

  expect(screen.queryByText("一组")).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "分组" })).toHaveAttribute("aria-expanded", "false");
});

it("展开剩余选项按钮按 Enter 时显示隐藏标签且保持选择弹层关闭", async () => {
  const user = userEvent.setup();
  render(<Selection />);
  screen.getByRole("button", { name: "另有 1 个" }).focus();

  await user.keyboard("{Enter}");

  expect(screen.getByText("二组")).toBeVisible();
  expect(screen.queryByRole("button", { name: "另有 1 个" })).not.toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "分组" })).toHaveAttribute("aria-expanded", "false");
});

function DialogSelection(props: { searchable?: boolean }): React.ReactElement {
  const [open, setOpen] = useState(true);
  const [selected, setSelected] = useState<string[]>([]);
  const options = [
    { value: "one", label: "一组" },
    { value: "two", label: "二组" },
  ];
  if (props.searchable) {
    options.push(
      { value: "three", label: "三组" },
      { value: "four", label: "四组" },
      { value: "five", label: "五组" },
      { value: "six", label: "六组" },
    );
  }
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogTitle>确认账号</DialogTitle>
        <DialogDescription>核对账号配置</DialogDescription>
        <MultiSelect options={options} selected={selected} onChange={setSelected} title="分组" />
      </DialogContent>
    </Dialog>
  );
}

it.each(["trigger", "chip"])(
  "无搜索多选从 %s 按 Escape 时仅关闭下拉并保留草稿，第二次才关闭弹窗",
  async (focus) => {
    const user = userEvent.setup();
    render(<DialogSelection />);
    const trigger = await screen.findByRole("combobox", { name: "分组" });
    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: "一组" }));
    if (focus === "chip") within(trigger).getByRole("button", { name: "移除" }).focus();
    else trigger.focus();

    await user.keyboard("{Escape}");

    expect(screen.getByRole("dialog", { name: "确认账号" })).toBeVisible();
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(within(trigger).getByText("一组")).toBeVisible();
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "确认账号" })).not.toBeInTheDocument(),
    );
  },
);

it("搜索框聚焦时 Escape 仅关闭多选下拉，重新打开保留已选草稿", async () => {
  const user = userEvent.setup();
  render(<DialogSelection searchable />);
  const trigger = await screen.findByRole("combobox", { name: "分组" });
  await user.click(trigger);
  const search = await screen.findByRole("combobox", { name: "搜索分组" });
  await user.type(search, "一组");
  await user.click(screen.getByRole("option", { name: "一组" }));
  search.focus();

  await user.keyboard("{Escape}");

  expect(screen.getByRole("dialog", { name: "确认账号" })).toBeVisible();
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  await user.click(trigger);
  expect(await screen.findByRole("option", { name: "一组" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
});
