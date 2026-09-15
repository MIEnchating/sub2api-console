import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AlertPolicyPage } from "../alert-policy-page";
import { cacheAlertPolicyPageData, policy } from "./fixtures";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("无利润流量告警默认开启且可通过键盘单独关闭", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  cacheAlertPolicyPageData(client);
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <AlertPolicyPage onOpenSettings={() => undefined} />
    </QueryClientProvider>,
  );
  const control = screen.getByRole("switch", { name: "无利润／亏损流量" });
  expect(control).toBeChecked();
  control.focus();
  await user.keyboard(" ");
  expect(control).not.toBeChecked();
  expect(screen.getByRole("switch", { name: "倍率上涨通知" })).toBeChecked();
  client.clear();
});

it("关闭告警总开关时成本流量开关保持选择但不可操作", () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  cacheAlertPolicyPageData(client, { ...policy, enabled: false });
  render(
    <QueryClientProvider client={client}>
      <AlertPolicyPage onOpenSettings={() => undefined} />
    </QueryClientProvider>,
  );
  const control = screen.getByRole("switch", { name: "无利润／亏损流量" });
  expect(control).toBeChecked();
  expect(control).toHaveAttribute("aria-disabled", "true");
  client.clear();
});
