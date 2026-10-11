import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";

import type { UpstreamAccountRequestUsage } from "@/api";
import { UpstreamUsageSummary } from "../upstream-usage-summary";

const accounts: UpstreamAccountRequestUsage[] = Array.from({ length: 500 }, (_, index) => ({
  account_id: String(index + 1),
  account_name: `账号 ${index + 1}`,
  request_count: index + 1,
  share: 0.2,
}));

function usage(items: UpstreamAccountRequestUsage[]) {
  return (
    <UpstreamUsageSummary
      name="分页上游"
      requestCount={125250}
      requestWindowDays={30}
      accountRequestUsage={items}
    />
  );
}

it("打开长账号明细时焦点停在标题，避免分页控件将顶部汇总滚出视野", async () => {
  const user = userEvent.setup();
  render(usage(accounts));
  await user.click(screen.getByRole("button", { name: "查看分页上游的账号使用率" }));
  expect(await screen.findByRole("heading", { name: "分页上游 · 使用统计" })).toHaveFocus();
});

it("500 个账号打开明细只渲染首 20 项，翻页展示后续账号且保留全量占比", async () => {
  const user = userEvent.setup();
  render(usage(accounts));
  await user.click(screen.getByRole("button", { name: "查看分页上游的账号使用率" }));
  const table = await screen.findByRole("table", { name: "账号使用率" });
  expect(within(table).getAllByRole("row")).toHaveLength(21);
  expect(within(table).getByText("账号 1")).toBeVisible();
  expect(within(table).queryByText("账号 21")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  expect(within(table).getByText("账号 21")).toBeVisible();
  expect(within(table).queryByText("账号 1")).not.toBeInTheDocument();
  expect(within(table).getAllByText("0.20%")).toHaveLength(20);
});

it("刷新保留当前页，账号减少时回到有效末页，空数据保留关闭入口", async () => {
  const user = userEvent.setup();
  const view = render(usage(accounts));
  await user.click(screen.getByRole("button", { name: "查看分页上游的账号使用率" }));
  await user.click(await screen.findByRole("button", { name: "转到下一页" }));
  view.rerender(usage(accounts.slice(0, 45)));
  expect(screen.getByText("账号 21")).toBeVisible();
  view.rerender(usage(accounts.slice(0, 1)));
  expect(screen.getByText("账号 1")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  view.rerender(usage([]));
  expect(screen.getByText("暂无绑定账号")).toBeVisible();
  expect(screen.queryByRole("navigation", { name: "表格分页" })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("button", { name: "查看分页上游的账号使用率" })).toHaveFocus();
});

it("关闭明细后重新打开从首页显示，焦点回到原入口", async () => {
  const user = userEvent.setup();
  render(usage(accounts));
  const trigger = screen.getByRole("button", { name: "查看分页上游的账号使用率" });
  await user.click(trigger);
  await user.click(await screen.findByRole("button", { name: "转到下一页" }));
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
  await user.keyboard("{Enter}");
  const table = await screen.findByRole("table", { name: "账号使用率" });
  expect(within(table).getByText("账号 1")).toBeVisible();
  expect(screen.getByRole("button", { name: "转到上一页" })).toBeDisabled();
});
