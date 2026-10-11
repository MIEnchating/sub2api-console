import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { FileText } from "lucide-react";
import { WorkbenchEmptyState, WorkbenchToolbar } from "../components/workbench-section";

afterEach(() => cleanup());

it("工作台操作面板使用统一卡片圆角并保留操作入口", () => {
  render(<WorkbenchToolbar description="说明" actions={<button type="button">刷新</button>} />);

  const panel = screen.getByRole("group", { name: "工作台操作" });
  expect(panel).toHaveClass("rounded-lg");
  expect(panel).not.toHaveClass("rounded-xl");
  expect(screen.getByRole("button", { name: "刷新" })).toBeVisible();
});

it("工作台空状态使用统一卡片和图标圆角", () => {
  render(<WorkbenchEmptyState icon={FileText} title="暂无内容" description="请先添加内容" />);

  const card = screen.getByText("暂无内容").parentElement;
  expect(card).toHaveClass("rounded-lg");
  expect(card).not.toHaveClass("rounded-xl");
  expect(card?.querySelector("span")).toHaveClass("rounded-lg");
  expect(screen.getByText("请先添加内容")).toBeVisible();
});
