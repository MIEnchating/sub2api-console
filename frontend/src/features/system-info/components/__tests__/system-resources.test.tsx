import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { SystemMetrics } from "@/api";

import { SystemResources } from "../system-resources";

const metrics: SystemMetrics = {
  sampled_at: "2026-09-15T00:00:00Z",
  cpu: { usage_percent: 37.5, logical_cores: 8 },
  memory: {
    total_bytes: 16 * 1024 ** 3,
    used_bytes: 10 * 1024 ** 3,
    available_bytes: 6 * 1024 ** 3,
    usage_percent: 62.5,
  },
  disk: {
    total_bytes: 200 * 1024 ** 3,
    used_bytes: 80 * 1024 ** 3,
    available_bytes: 120 * 1024 ** 3,
    usage_percent: 40,
  },
};

const baseProps = { loading: false, refreshing: false, failed: false, onRetry: (): void => {} };

describe("服务器资源布局", () => {
  it("窄屏仍并排展示三项资源，并保留百分比和容量信息", () => {
    render(<SystemResources {...baseProps} metrics={metrics} />);

    const resources = screen.getByRole("region", { name: "服务器资源占用" });
    expect(resources).toHaveClass("grid-cols-3");
    expect(within(resources).getByText("CPU 占用")).toBeVisible();
    expect(within(resources).getByText("8 个逻辑核心")).toBeVisible();
    expect(within(resources).getByText("10 GB / 16 GB")).toBeVisible();
    expect(within(resources).getByText("80 GB / 200 GB")).toBeVisible();
    expect(within(resources).getByRole("progressbar", { name: "CPU 占用 37.5%" })).toHaveAttribute(
      "aria-valuenow",
      "37.5",
    );
    expect(within(resources).getByRole("progressbar", { name: "内存占用 62.5%" })).toHaveAttribute(
      "aria-valuenow",
      "62.5",
    );
    expect(within(resources).getByRole("progressbar", { name: "硬盘占用 40%" })).toHaveAttribute(
      "aria-valuenow",
      "40",
    );
  });

  it("容量明细过长时允许在资源卡片内换行", () => {
    render(
      <SystemResources
        {...baseProps}
        metrics={{ ...metrics, cpu: { usage_percent: 100, logical_cores: 1024 } }}
      />,
    );

    expect(screen.getByText("1024 个逻辑核心")).toHaveClass("min-w-0", "wrap-anywhere");
    expect(screen.getByText("10 GB / 16 GB")).toHaveClass("min-w-0", "wrap-anywhere");
    expect(screen.getByText("100%")).toHaveClass("tabular-nums", "whitespace-nowrap");
  });

  it("首次读取资源时使用相同三列布局的骨架，不显示确定进度", () => {
    render(<SystemResources {...baseProps} loading refreshing />);

    expect(screen.getByRole("region", { name: "服务器资源占用" })).toHaveClass("grid-cols-3");
    expect(screen.getAllByRole("status", { name: "正在读取资源占用" })).toHaveLength(3);
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  });

  it("后台刷新失败时保留已读取资源，不替换为骨架或重试占位", () => {
    const view = render(<SystemResources {...baseProps} metrics={metrics} />);

    view.rerender(<SystemResources {...baseProps} metrics={metrics} refreshing failed />);

    expect(screen.getByText("37.5%")).toBeVisible();
    expect(screen.getByText("10 GB / 16 GB")).toBeVisible();
    expect(screen.queryByRole("status", { name: "正在读取资源占用" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  });

  it("首次读取失败时提供重试，重试进行期间禁用入口", async () => {
    const onRetry = vi.fn();
    const view = render(<SystemResources {...baseProps} failed onRetry={onRetry} />);
    const retry = screen.getByRole("button", { name: "重新读取" });

    expect(screen.queryByRole("status", { name: "正在读取资源占用" })).not.toBeInTheDocument();
    await userEvent.setup().click(retry);
    expect(onRetry).toHaveBeenCalledOnce();

    view.rerender(<SystemResources {...baseProps} failed refreshing onRetry={onRetry} />);
    expect(retry).toBeDisabled();
  });
});
