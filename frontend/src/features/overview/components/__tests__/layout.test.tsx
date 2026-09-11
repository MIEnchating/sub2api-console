import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { OverviewPage } from "../overview-page";
import { OverviewActivity } from "../overview-activity";

it("读取事件或渠道失败时不在页面重复展示查询错误", () => {
  render(
    <OverviewActivity
      attention={[]}
      events={[]}
      attentionLoading={false}
      eventsLoading={false}
      attentionError={new Error("账号读取失败")}
      eventsError={new Error("事件读取失败")}
      onOpenAccounts={() => {}}
      onOpenEvents={() => {}}
    />,
  );
  expect(screen.queryByText("账号读取失败")).not.toBeInTheDocument();
  expect(screen.queryByText("事件读取失败")).not.toBeInTheDocument();
  expect(screen.queryByText("所有受管渠道都健康")).not.toBeInTheDocument();
});

it("四个核心指标的说明允许换行而不隐藏风险数量", () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["accounts"], []);
  client.setQueryData(["groups"], []);
  render(
    <QueryClientProvider client={client}>
      <OverviewPage onOpenAccounts={() => {}} onOpenGroups={() => {}} onOpenEvents={() => {}} />
    </QueryClientProvider>,
  );
  expect(screen.getByText(/0 健康 · 0 降级/)).not.toHaveClass("truncate");
  expect(screen.getByRole("button", { name: "刷新运营总览" })).toBeEnabled();
});
