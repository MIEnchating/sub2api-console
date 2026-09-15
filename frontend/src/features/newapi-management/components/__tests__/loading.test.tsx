import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { RemoteModelPricesTable } from "../model-prices";
import { NewAPIRemoteLoading } from "../newapi-management-page";
it("远端表格读取时使用能填满工作区的共享骨架，不固定窄屏列宽", () => {
  render(<NewAPIRemoteLoading label="正在读取远端数据" />);
  const loading = screen.getByRole("status");
  expect(loading).toHaveAttribute("data-slot", "page-loading-skeleton");
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading).toHaveClass("h-full", "min-w-0");
});
it("远程价格目录首次读取时展示骨架屏，不把数据尚未返回显示为空目录", () => {
  render(<RemoteModelPricesTable prices={[]} pending error="" />);
  const loading = screen.getByRole("status", { name: "正在获取远程模型价格" });
  expect(loading.querySelector('[data-slot="skeleton"]')).not.toBeNull();
  expect(screen.queryByText("远程价卡未返回模型价格")).not.toBeInTheDocument();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(loading.querySelector(".border")).toBeNull();
});

it("渠道首次读取时显示一张包含步骤和凭据分栏的卡片", () => {
  render(<NewAPIRemoteLoading label="正在加载渠道管理" view="channels" />);
  const loading = screen.getByRole("status", { name: "正在加载渠道管理" });
  expect(loading.querySelectorAll('[data-slot="card"]')).toHaveLength(1);
  expect(loading.querySelector("[data-channel-credentials-layout]")).toHaveClass("grid-cols-1");
  expect(loading.querySelector('[data-slot="skeleton-pagination"]')).toBeNull();
  expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
});

it("平台配置首次读取时显示单张详情卡内的两列信息", () => {
  render(<NewAPIRemoteLoading label="正在加载平台配置" view="platform" />);
  const loading = screen.getByRole("status", { name: "正在加载平台配置" });
  expect(loading.querySelectorAll('[data-slot="card"]')).toHaveLength(1);
  expect(loading.querySelector('[data-slot="platform-details-grid"]')).toHaveClass(
    "sm:grid-cols-2",
  );
  expect(loading.querySelector('[data-slot="skeleton-pagination"]')).toBeNull();
});
