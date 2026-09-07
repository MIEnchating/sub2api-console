import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AlertPolicyPage } from "../alert-policy-page";
import { cacheAlertPolicyPageData, policy } from "./fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => vi.unstubAllGlobals());

function renderPolicy(): void {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  cacheAlertPolicyPageData(client);
  render(
    <QueryClientProvider client={client}>
      <AlertPolicyPage onOpenSettings={() => undefined} />
    </QueryClientProvider>,
  );
}

describe("告警策略开关联动", () => {
  it("关闭检测总开关后，禁用其他设置并在重新开启时保留选择", async () => {
    const user = userEvent.setup();
    renderPolicy();
    const master = screen.getByRole("switch", { name: "启用告警检测" });
    await user.click(master);
    for (const control of screen.getAllByRole("switch").filter((item) => item !== master)) {
      expect(control).toHaveAttribute("aria-disabled", "true");
    }
    expect(screen.getByRole("textbox", { name: "余额告警阈值 1" })).toBeDisabled();
    await user.click(master);
    expect(screen.getByRole("switch", { name: "配置异常" })).toBeChecked();
    expect(screen.getByRole("textbox", { name: "余额告警阈值 1" })).toHaveValue("20");
  });

  it("关闭通知发送后，禁用通知设置但仍可编辑检测规则", async () => {
    const user = userEvent.setup();
    renderPolicy();
    await user.click(screen.getByRole("switch", { name: "启用通知发送" }));
    expect(screen.getByRole("switch", { name: "发送恢复通知" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("switch", { name: "鉴权恢复" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("spinbutton", { name: "重复提醒间隔（分钟）" })).toBeDisabled();
    expect(screen.getByRole("switch", { name: "鉴权失效" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });

  it("通过键盘启用恢复类型并保存后，提交新的选择且保留其他策略字段", async () => {
    const user = userEvent.setup();
    const fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit): Promise<Response> =>
      Response.json(JSON.parse(String(init?.body))),
    );
    vi.stubGlobal("fetch", fetch);
    renderPolicy();
    await user.click(screen.getByRole("switch", { name: "发送恢复通知" }));
    const recovery = screen.getByRole("switch", { name: "配置异常恢复" });
    recovery.focus();
    await user.keyboard(" ");
    expect(recovery).toBeChecked();
    expect(recovery).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "保存策略" }));
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({
      ...policy,
      notify_recovery: true,
      recovery_notification_types: [...policy.recovery_notification_types, "configuration"],
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled());
  });

  it("关闭账号降级后，禁用降级来源并保留原先选中的来源", async () => {
    const user = userEvent.setup();
    renderPolicy();
    await user.click(screen.getByRole("switch", { name: "账号降级" }));
    const source = screen.getByRole("switch", { name: "健康分过低" });
    expect(source).toHaveAttribute("aria-disabled", "true");
    expect(source).toBeChecked();
    expect(screen.getByRole("switch", { name: "账号熔断判定" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });
});
