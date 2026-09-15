import type { ComponentProps } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { UnifiedLogEntry } from "@/api";
import { LogsTable } from "../logs-table";

const entry: UnifiedLogEntry = {
  id: "task:inspection-1",
  kind: "task",
  occurred_at: "2026-09-15T08:59:33Z",
  title: "automatic-inspection",
  summary: "巡检完成，账号状态已更新",
  status: "succeeded",
  actor: null,
  object_label: null,
  source: "task",
  source_id: "inspection-1",
  related_count: 0,
  details: {},
};

function renderTable(overrides: Partial<ComponentProps<typeof LogsTable>> = {}) {
  return render(
    <LogsTable
      items={[entry]}
      kind="all"
      loading={false}
      unavailable={false}
      refreshing={false}
      filtered={false}
      onRetry={() => undefined}
      onSelect={() => undefined}
      {...overrides}
    />,
  );
}

describe("日志记录内容", () => {
  it("记录缺少对象和执行人时只显示来源，避免重复占位文字", () => {
    renderTable();

    expect(screen.getAllByText("业务任务")).toHaveLength(1);
    expect(screen.queryByText("未记录对象")).not.toBeInTheDocument();
    expect(screen.queryByText(/执行人：/)).not.toBeInTheDocument();
    expect(screen.queryByText(/关联 \d+ 条/)).not.toBeInTheDocument();
  });

  it("记录包含长标题、摘要和对象时保留完整内容供查看", () => {
    const title = "生产环境账号与分组巡检".repeat(8);
    const summary = "上游响应超时，稍后重试并检查服务可用性。".repeat(8);
    const object = "生产环境专用分组".repeat(8);
    renderTable({
      items: [{ ...entry, title, summary, object_label: object, actor: "巡检调度器" }],
    });

    for (const text of [title, summary, object]) {
      expect(screen.getByText(text)).toHaveTextContent(text);
      expect(screen.getByText(text)).not.toHaveAttribute("title");
    }
    expect(screen.getByText("执行人：巡检调度器")).toBeVisible();
  });

  it("摘要过长时关联数量独立展示，不随摘要一起截断", () => {
    const summary = "巡检完成但部分账号需要复核。".repeat(10);
    renderTable({ items: [{ ...entry, summary, related_count: 131 }] });

    expect(screen.getByText("关联 131 条")).toBeVisible();
    expect(screen.getByText(summary)).not.toHaveTextContent("关联 131 条");
  });

  it("键盘打开指定行详情时返回该条完整日志", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    renderTable({ onSelect });

    expect(screen.getByRole("table", { name: "日志记录" })).toHaveAttribute(
      "data-action-column",
      "true",
    );

    await user.tab();
    expect(screen.getByRole("button", { name: "查看日志详情" })).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(onSelect).toHaveBeenCalledWith(entry);
  });

  it("混合记录中任务展示执行状态，事件展示日志级别", () => {
    renderTable({
      items: [entry, { ...entry, id: "event:1", kind: "event", source: "runtime_event" }],
    });

    const rows = screen.getAllByRole("row").slice(1);
    expect(within(rows[0]).getByText("成功")).toBeVisible();
    expect(within(rows[1]).getByText("信息")).toBeVisible();
    expect(within(rows[1]).getByText("信息").closest('[data-slot="status-badge"]')).toHaveClass(
      "text-info",
    );
  });
});

describe("日志读取状态", () => {
  it("首次读取时展示匹配六列表格的骨架，不显示空记录或详情操作", () => {
    renderTable({ items: [], loading: true });

    const rows = screen.getAllByRole("row", { name: "正在加载日志" });
    expect(screen.getByRole("table", { name: "日志记录" })).toHaveAttribute(
      "data-action-column",
      "true",
    );
    expect(rows).toHaveLength(6);
    for (const row of rows) expect(within(row).getAllByRole("cell")).toHaveLength(6);
    expect(screen.queryByText("暂无日志记录")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "查看日志详情" })).not.toBeInTheDocument();
  });

  it.each([
    { filtered: false, title: "暂无日志记录", hint: "任务执行和系统事件将显示在这里" },
    { filtered: true, title: "没有匹配的记录", hint: "试试其他关键词或调整筛选条件" },
  ])("空列表且 filtered=$filtered 时展示对应说明", (scenario) => {
    renderTable({ items: [], filtered: scenario.filtered });

    expect(screen.getByText(scenario.title)).toBeVisible();
    expect(screen.getByText(scenario.hint)).toBeVisible();
  });

  it("首次读取失败时可以重试，重试期间禁用重复提交", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    const view = renderTable({ items: [], unavailable: true, onRetry });

    await user.click(screen.getByRole("button", { name: "重新读取" }));
    expect(onRetry).toHaveBeenCalledOnce();
    expect(screen.queryByText("暂无日志记录")).not.toBeInTheDocument();

    view.rerender(
      <LogsTable
        items={[]}
        kind="all"
        loading={false}
        unavailable
        refreshing
        filtered={false}
        onRetry={onRetry}
        onSelect={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "重新读取" })).toBeDisabled();
  });

  it("后台刷新期间保留已有记录和详情入口", () => {
    renderTable({ refreshing: true });

    expect(screen.getByText("自动巡检")).toBeVisible();
    expect(screen.getByRole("button", { name: "查看日志详情" })).toBeEnabled();
    expect(screen.queryByRole("row", { name: "正在加载日志" })).not.toBeInTheDocument();
  });
});
