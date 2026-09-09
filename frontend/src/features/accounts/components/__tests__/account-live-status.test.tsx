import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AccountLiveStatus } from "../account-live-status";

describe("最近结果连接状态", () => {
  it("已连接时仅显示紧凑状态图标，不在页面额外铺开提示文字", () => {
    render(<AccountLiveStatus status="connected" />);
    const indicator = screen.getByRole("status", { name: "实时请求已连接" });
    expect(indicator).toHaveClass("size-5", "shrink-0");
    expect(indicator).toHaveAttribute("tabindex", "0");
    expect(indicator).toHaveTextContent("");
    expect(screen.queryByText("实时请求已连接")).not.toBeInTheDocument();
  });
  it("连接中断时图标可聚焦并提供原因和下一步的可访问说明", () => {
    render(<AccountLiveStatus status="reconnecting" />);
    const indicator = screen.getByRole("status", { name: "实时请求连接中断" });
    expect(indicator).toHaveAttribute("tabindex", "0");
    expect(indicator).toHaveAccessibleDescription(
      expect.stringContaining("正在自动重连，已有请求结果仍保留"),
    );
  });
  it("没有可订阅账号时隐藏状态图标", () => {
    render(<AccountLiveStatus status="idle" />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
