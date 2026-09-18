import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { AccountTrafficSnapshot } from "@/api";
import { account } from "../../__tests__/fixtures";
import { AccountIdentityCell } from "../account-pool-cells";
import { AccountTrafficProvider } from "../account-traffic";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

function renderIdentity(snapshot: Partial<AccountTrafficSnapshot> = {}, long = false) {
  const client = new QueryClient();
  clients.push(client);
  client.setQueryData(["accounts", "live-traffic"], {
    enabled: true,
    observed_at: new Date().toISOString(),
    accounts: [{ account_id: "41", current_requests: 2, waiting_requests: 0, tracked: true }],
    ...snapshot,
  });
  const row = {
    ...account,
    name: long ? "需要保留完整名称的账号".repeat(10) : "测试账号",
    upstream_host: long ? `${"long-host-".repeat(20)}example.test` : null,
    groups: long ? ["主分组".repeat(20), "备用分组"] : [],
  };
  render(
    <QueryClientProvider client={client}>
      <AccountTrafficProvider>
        <AccountIdentityCell account={row} />
      </AccountTrafficProvider>
    </QueryClientProvider>,
  );
  return row;
}

it.each([
  { label: "真实请求 · 2", snapshot: {} },
  { label: "流量监控未开启", snapshot: { enabled: false } },
  { label: "流量读取失败", snapshot: { observed_at: "2000-01-01T00:00:00Z" } },
  { label: "流量未知", snapshot: { accounts: [] } },
])("流量显示为 $label 时与账号名同排且占用固定宽度", (state) => {
  renderIdentity(state.snapshot);
  const status = screen.getByLabelText(`账号 41：${state.label}`);
  expect(status).toHaveClass("w-20", "h-5", "shrink-0", "whitespace-nowrap");
  const heading = status.closest('[data-slot="account-identity-heading"]');
  expect(heading).toHaveClass("flex", "flex-nowrap", "min-w-0");
  expect(heading).toContainElement(screen.getByText("测试账号"));
});

it("账号名、Host 和多分组过长时保持单行省略并可键盘查看完整内容", async () => {
  const user = userEvent.setup();
  const row = renderIdentity({}, true);
  for (const text of [row.name, row.upstream_host!, `分组：${row.groups.join("、")}`]) {
    const field = screen.getByText(text);
    expect(field).toHaveClass("truncate");
    expect(field).toHaveAttribute("tabindex", "0");
    field.focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(text);
    await user.keyboard("{Escape}");
  }
  const group = screen.getByText(`分组：${row.groups.join("、")}`);
  expect(group).toHaveClass("w-full");
  expect(group).not.toHaveClass("border-l", "pl-2");
});

it("Host 缺失时在账号类型同行显示占位，分组独占一行", () => {
  renderIdentity();
  const context = screen.getByText("Host 未记录").closest('[data-slot="account-identity-meta"]');
  expect(context).toHaveClass("flex", "flex-nowrap");
  expect(context).toContainElement(screen.getByText(/#41/));
  expect(context).not.toContainElement(screen.getByText("分组：未分组"));
});
