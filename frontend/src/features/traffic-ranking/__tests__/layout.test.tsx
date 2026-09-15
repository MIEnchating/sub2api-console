import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { TrafficRankingPage } from "../components/traffic-ranking-page";
import { rankingFixture } from "./fixtures";

const clients: QueryClient[] = [];

afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
});

function renderRanking(): void {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  clients.push(client);
  client.setQueryData(["groups"], []);
  client.setQueryData(["dictionaries", "platform"], { items: [] });
  client.setQueryData(["traffic-ranking", "24h", "all", "traffic"], rankingFixture);
  render(
    <QueryClientProvider client={client}>
      <TrafficRankingPage />
    </QueryClientProvider>,
  );
}

describe("流量排行布局", () => {
  it("有流量时合并账号排名与活跃信息，保留所有指标和清晰标签", () => {
    renderRanking();

    const table = screen.getByRole("table", { name: "账号流量排行" });
    expect(within(table).getAllByRole("columnheader")).toHaveLength(8);
    const row = screen.getByRole("row", { name: /稳定主账号/ });
    expect(within(row).getByLabelText("第 1 名")).toBeInTheDocument();
    expect(within(row).getByText("1,250")).toBeInTheDocument();
    expect(within(row).getByText("100.00%")).toBeInTheDocument();
    expect(within(row).getByText("99.08%")).toBeInTheDocument();
    expect(within(row).getByText("820 ms")).toBeInTheDocument();
    expect(within(row).getByText("1.45 s")).toBeInTheDocument();
    expect(within(row).getByText("22 / 24 小时")).toBeInTheDocument();
    expect(within(row).getByText("0.12 M")).toBeInTheDocument();
    expect(within(row).getByText("0.05 M")).toBeInTheDocument();
    expect(within(row).getByText("6.19%")).toBeInTheDocument();
    expect(within(row).getByText("0.93%")).toBeInTheDocument();
    expect(within(table).getByRole("columnheader", { name: /缓存占比/ })).toHaveAttribute(
      "title",
      "读取、写入分别占输入侧用量的比例；输入侧用量 = 输入 + 缓存读取 + 缓存写入，不含输出。",
    );
    expect(within(row).getByText("输入")).toBeInTheDocument();
    expect(within(row).getByText("输出")).toBeInTheDocument();
    expect(within(row).getByText("读取")).toBeInTheDocument();
    expect(within(row).getByText("写入")).toBeInTheDocument();
  });

  it("表格横向滚动时账号列固定，辅助文字保留独立字号", () => {
    renderRanking();

    const identity = screen.getByRole("row", { name: /稳定主账号/ }).children[0];
    expect(identity).toHaveClass("sticky", "left-0", "bg-card");
    expect(screen.getByRole("columnheader", { name: "排名 / 账号" })).toHaveClass("left-0");
    expect(screen.getByRole("table", { name: "账号流量排行" })).not.toHaveClass("[&_td_*]:text-sm");
    expect(screen.getByText("#41 · codex")).toHaveClass("text-xs", "truncate");
    expect(screen.getByText("api.example")).toHaveAttribute("title", "api.example");
  });

  it("账号没有流量或用量时保留账号，并明确显示缺失值", () => {
    renderRanking();

    const row = screen.getByRole("row", { name: /待接入账号/ });
    expect(within(row).getByText("无样本")).toBeInTheDocument();
    expect(within(row).getAllByText("未提供")).toHaveLength(2);
    expect(within(row).getByText("0 / 24 小时")).toBeInTheDocument();
    expect(within(row).queryByRole("time")).not.toBeInTheDocument();
  });
});
