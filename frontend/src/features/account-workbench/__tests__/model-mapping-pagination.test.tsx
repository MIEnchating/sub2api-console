import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { TemplateModelMapping } from "../components/template-model-mapping";

function mapping(count: number): Record<string, string> {
  return Object.fromEntries(
    Array.from({ length: count }, (_, index) => [`source-${index + 1}`, `target-${index + 1}`]),
  );
}

it("1000条映射默认只展示20条，键盘翻页可以读取剩余内容", async () => {
  render(<TemplateModelMapping mapping={mapping(1000)} />);
  const table = screen.getByRole("table", { name: "模型映射" });
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  expect(screen.queryByRole("cell", { name: "source-21" })).not.toBeInTheDocument();
  screen.getByRole("button", { name: "转到下一页" }).focus();
  await userEvent.keyboard("{Enter}");
  expect(screen.getByRole("row", { name: "source-21 target-21" })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "转到最后一页" }));
  expect(screen.getByRole("row", { name: "source-1000 target-1000" })).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
});

it("紧凑预览展开后分页，收起再展开从第一页展示", async () => {
  render(<TemplateModelMapping mapping={mapping(41)} compact />);
  const table = screen.getByRole("table", { name: "模型映射" });
  expect(within(table).getAllByRole("row")).toHaveLength(4);
  expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  const toggle = screen.getByRole("button", { name: /展开.*41.*映射/ });
  expect(toggle).toHaveAttribute("aria-controls", table.id);
  await userEvent.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  await userEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  await userEvent.click(screen.getByRole("button", { name: "收起映射" }));
  expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  await userEvent.click(toggle);
  expect(screen.getByRole("row", { name: "source-1 target-1" })).toBeVisible();
});

it("映射刷新保留有效页，缩减或清空后页码收敛且不会恢复旧末页", async () => {
  const view = render(<TemplateModelMapping mapping={mapping(41)} />);
  await userEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<TemplateModelMapping mapping={{ ...mapping(42), "source-21": "updated" }} />);
  expect(screen.getByRole("row", { name: "source-21 updated" })).toBeVisible();
  view.rerender(<TemplateModelMapping mapping={mapping(1)} />);
  expect(screen.getByRole("row", { name: "source-1 target-1" })).toBeVisible();
  expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  view.rerender(<TemplateModelMapping mapping={{}} />);
  expect(screen.getByText("不限制")).toBeVisible();
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  view.rerender(<TemplateModelMapping mapping={mapping(41)} />);
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
});

it("修改每页行数后回到首页，页数减少时仍能调整行数", async () => {
  render(<TemplateModelMapping mapping={mapping(41)} />);
  await userEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  await userEvent.click(screen.getByRole("combobox", { name: "每页行数" }));
  await userEvent.click(await screen.findByRole("option", { name: "50" }));
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(42);
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "每页行数" })).toBeEnabled();
});

it("末页超长来源和目标保持完整文字及换行", async () => {
  const source = "source-".repeat(100);
  const target = "target-".repeat(100);
  render(<TemplateModelMapping mapping={{ ...mapping(20), [source]: target }} />);
  await userEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("cell", { name: source })).toHaveClass("wrap-anywhere");
  expect(screen.getByRole("cell", { name: target })).toHaveClass("wrap-anywhere");
});

it("选择10行后目录缩到15条，仍可翻页查看余下映射", async () => {
  const view = render(<TemplateModelMapping mapping={mapping(41)} />);
  await userEvent.click(screen.getByRole("combobox", { name: "每页行数" }));
  await userEvent.click(await screen.findByRole("option", { name: "10" }));
  view.rerender(<TemplateModelMapping mapping={mapping(15)} />);
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(11);
  await userEvent.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(screen.getByRole("row", { name: "source-15 target-15" })).toBeVisible();
});
