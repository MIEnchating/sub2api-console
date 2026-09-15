import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { UpstreamConcurrency, UpstreamConcurrencyHeading } from "../upstream-concurrency";

describe("上游并发额度", () => {
  it("读取到有限额度时主行显示已配置与上限，仅额外显示不同的目标", () => {
    render(<UpstreamConcurrency limit={20} status="known" allocated={12} target={16} />);

    expect(screen.getByLabelText("用户上限")).toHaveTextContent("20");
    expect(screen.getByLabelText("已配置并发")).toHaveTextContent("12");
    expect(screen.getByLabelText("目标并发")).toHaveTextContent("16");
    expect(screen.getByLabelText("当前并发与用户上限")).toHaveTextContent("12 / 20");
    expect(screen.queryByText("用户上限")).not.toBeInTheDocument();
    expect(screen.queryByText("已配置")).not.toBeInTheDocument();
    expect(screen.queryByText("已超上限")).not.toBeInTheDocument();
  });

  it("用户上限为零时显示不限，已配置为零时仍显示零", () => {
    render(<UpstreamConcurrency limit={0} status="unlimited" allocated={0} target={null} />);

    expect(screen.getByLabelText("用户上限")).toHaveTextContent("不限");
    expect(screen.getByLabelText("已配置并发")).toHaveTextContent("0");
    expect(screen.queryByLabelText("目标并发")).not.toBeInTheDocument();
  });

  it("尚未读取额度且账号配置未知时不以零冒充可分配容量", () => {
    render(<UpstreamConcurrency limit={null} status="unknown" allocated={null} target={null} />);

    expect(screen.getByLabelText("用户上限")).toHaveTextContent("未读取");
    expect(screen.queryByLabelText("已配置并发")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("目标并发")).not.toBeInTheDocument();
  });

  it("旧响应没有并发字段时保留未读取状态", () => {
    render(<UpstreamConcurrency />);

    expect(screen.getByLabelText("用户上限")).toHaveTextContent("未读取");
    expect(screen.queryByLabelText("已配置并发")).not.toBeInTheDocument();
  });

  it("未读取用户上限时只显示短状态，不重复展示配置和目标数字", () => {
    render(<UpstreamConcurrency status="unknown" allocated={800} target={800} />);

    expect(screen.getByRole("group", { name: "上游并发" })).toHaveTextContent(/^未读取$/);
    expect(screen.queryByLabelText("目标并发")).not.toBeInTheDocument();
  });

  it("调度目标与已配置相同时只保留一行容量", () => {
    render(<UpstreamConcurrency limit={1000} status="known" allocated={800} target={800} />);

    expect(screen.getByLabelText("当前并发与用户上限")).toHaveTextContent("800 / 1000");
    expect(screen.queryByLabelText("目标并发")).not.toBeInTheDocument();
  });

  it.each([
    [24, "24"],
    [0, "不限"],
  ])("最新读取失败但留有额度 %i 时展示缓存额度", (limit, expected) => {
    render(<UpstreamConcurrency limit={limit} status="stale" allocated={10} target={12} />);

    expect(screen.getByLabelText("用户上限")).toHaveTextContent(expected);
    expect(screen.getByText("缓存额度")).toBeVisible();
  });

  it("已配置并发超过有限额度时明确标记超限", () => {
    render(<UpstreamConcurrency limit={10} status="known" allocated={14} target={10} />);

    expect(screen.getByText("已超上限")).toBeVisible();
    expect(screen.getByLabelText("已配置并发")).toHaveTextContent("14");
  });

  it("目标为零时展示零且长数值允许换行以保留完整信息", () => {
    render(<UpstreamConcurrency limit={123456789012345} status="known" allocated={1} target={0} />);

    expect(screen.getByLabelText("目标并发")).toHaveTextContent("0");
    expect(screen.getByLabelText("用户上限")).toHaveTextContent("123456789012345");
    expect(screen.getByLabelText("用户上限")).toHaveClass("break-all");
    expect(screen.getByRole("group", { name: "上游并发" })).toHaveClass(
      "min-w-0",
      "whitespace-normal",
    );
  });

  it("重新读取到新额度后更新数值并移除缓存和超限标记", () => {
    const view = render(
      <UpstreamConcurrency limit={10} status="stale" allocated={12} target={10} />,
    );
    expect(screen.getByText("缓存额度")).toBeVisible();
    expect(screen.getByText("已超上限")).toBeVisible();

    view.rerender(<UpstreamConcurrency limit={20} status="known" allocated={12} target={18} />);

    expect(screen.getByLabelText("用户上限")).toHaveTextContent("20");
    expect(screen.getByLabelText("目标并发")).toHaveTextContent("18");
    expect(screen.queryByText("缓存额度")).not.toBeInTheDocument();
    expect(screen.queryByText("已超上限")).not.toBeInTheDocument();
  });

  it("键盘聚焦并发说明时展示刷新入口和自动分配所需开关", async () => {
    const user = userEvent.setup();
    render(<UpstreamConcurrencyHeading />);

    await user.tab();

    expect(screen.getByRole("button", { name: "上游并发说明" })).toHaveFocus();
    const tooltip = await screen.findByText(/Sub2API 用户并发上限通过/);
    expect(tooltip).toBeVisible();
    expect(tooltip).toHaveTextContent("同步余额");
    expect(tooltip).toHaveTextContent("同步上游");
    expect(tooltip).toHaveTextContent("智能扩容");
    expect(tooltip).toHaveTextContent("上游超额自动下调");
    expect(tooltip).toHaveTextContent("不自动扩容或恢复账号");
    expect(tooltip).toHaveTextContent("并发上限自动执行");
    expect(tooltip).toHaveTextContent("调度状态自动执行");
    expect(tooltip).toHaveTextContent("完全模式");
    expect(tooltip).toHaveTextContent("额度恢复后自动评估");
    expect(tooltip).toHaveTextContent("配置容量");
  });
});
