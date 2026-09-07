import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AlertPolicyPage } from "../alert-policy-page";
import { cacheAlertPolicyPageData } from "./fixtures";

function renderPolicy(): void {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  cacheAlertPolicyPageData(client);
  render(
    <QueryClientProvider client={client}>
      <AlertPolicyPage onOpenSettings={() => undefined} />
    </QueryClientProvider>,
  );
}

describe("告警策略布局", () => {
  it("读取策略后，将检测总开关和所有检测规则放在同一区域", () => {
    renderPolicy();
    const detection = screen.getByRole("region", { name: "告警检测" });
    expect(within(detection).getByRole("switch", { name: "启用告警检测" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(within(detection).getByRole("switch", { name: "配置异常" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(within(detection).getByRole("switch", { name: "账号降级" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });

  it("按单列阅读或键盘导航时，依次经过检测、阈值和通知区域", () => {
    renderPolicy();
    const regions = screen.getAllByRole("region");
    expect(regions.map((region) => region.getAttribute("aria-label"))).toEqual([
      "告警检测",
      "阈值与范围",
      "通知发送",
    ]);
  });

  it("设置发送行为时，频率字段位于较长的恢复类型列表之前", () => {
    renderPolicy();
    const notification = screen.getByRole("region", { name: "通知发送" });
    const frequency = within(notification).getByRole("spinbutton", {
      name: "重复提醒间隔（分钟）",
    });
    const recovery = within(notification).getByRole("switch", {
      name: "配置异常恢复",
    });
    expect(
      frequency.compareDocumentPosition(recovery) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
