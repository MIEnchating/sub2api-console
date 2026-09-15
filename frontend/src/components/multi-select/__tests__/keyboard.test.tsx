import { useState } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { MultiSelect } from "../../multi-select";

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
