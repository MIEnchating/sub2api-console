import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { MonitorWorkspace } from "../monitor-workspace";
import { monitor } from "./fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

describe("Uptime Kuma 监控表格", () => {
  it("操作位于对应监控项右侧，点击恢复传递该行稳定 ID", () => {
    const action = vi.fn();
    const paused = { ...monitor, id: 20, key: "id:20", name: "备用", active: false };
    render(
      <MonitorWorkspace
        monitors={[monitor, paused]}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={action}
      />,
    );
    const row = screen.getByRole("row", { name: /备用/ });
    const button = within(row).getByRole("button", { name: "恢复" });
    expect(button.closest("td")).toBe(row.lastElementChild);
    fireEvent.click(button);
    expect(action).toHaveBeenCalledWith(paused, "resume");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("搜索名称或地址无匹配时显示空状态", async () => {
    render(
      <MonitorWorkspace
        monitors={[monitor]}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "搜索监控项或地址" }), "不存在");
    expect(screen.getByText("没有匹配的监控项")).toBeVisible();
    await user.clear(screen.getByRole("textbox", { name: "搜索监控项或地址" }));
    await user.type(screen.getByRole("textbox", { name: "搜索监控项或地址" }), "monitor.example");
    expect(screen.getByRole("button", { name: "查看 智谱 详情" })).toBeVisible();
  });
  it("仅配置密钥时没有操作列且无状态不伪装健康", () => {
    render(
      <MonitorWorkspace
        monitors={[{ ...monitor, status: null }]}
        management={false}
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    expect(screen.queryByRole("columnheader", { name: "操作" })).not.toBeInTheDocument();
    expect(screen.getByText("暂无状态")).toBeVisible();
  });
  it("刷新或写入中禁用全部写操作", () => {
    render(
      <MonitorWorkspace
        monitors={[monitor]}
        management
        disabled
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    for (const name of ["暂停", "编辑", "删除"])
      expect(screen.getByRole("button", { name })).toBeDisabled();
  });
  it("空列表显示空状态且分页不可前进", () => {
    render(
      <MonitorWorkspace
        monitors={[]}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    expect(screen.getByText("暂无监控项")).toBeVisible();
    expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  });
  it("多页数据切换后展示下一页，搜索时重置为第一页", async () => {
    const monitors = Array.from({ length: 21 }, (_, i) => ({
      ...monitor,
      id: i + 1,
      key: `id:${i + 1}`,
      name: `监控 ${i + 1}`,
    }));
    render(
      <MonitorWorkspace
        monitors={monitors}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    const user = userEvent.setup({ skipHover: true });
    fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));
    expect(screen.getByRole("button", { name: "查看 监控 21 详情" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "查看 监控 1 详情" })).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "搜索监控项或地址" }), "监控 1");
    expect(screen.getByRole("button", { name: "查看 监控 1 详情" })).toBeVisible();
    expect(screen.getByRole("button", { name: "转到第 1 页" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
  it("长名称不扩大表格，内容区域负责滚动且列表填满剩余空间", () => {
    const name = "超长监控名称".repeat(20);
    render(
      <MonitorWorkspace
        monitors={[{ ...monitor, name }]}
        management
        disabled={false}
        onEdit={vi.fn()}
        onAction={vi.fn()}
      />,
    );
    expect(screen.getByText(name)).toHaveClass("truncate");
    expect(screen.getByRole("table").parentElement).toHaveClass(
      "min-h-0",
      "flex-1",
      "overflow-auto",
    );
    expect(screen.getByRole("region", { name: "监控项列表" })).toHaveClass("flex-1");
  });
});
