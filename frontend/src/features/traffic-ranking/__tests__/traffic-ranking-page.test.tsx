import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TrafficRanking } from "@/api";

import { TrafficRankingPage } from "../components/traffic-ranking-page";
import { rankingFixture } from "./fixtures";

const clients: QueryClient[] = [];

afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
});

function renderRanking(data?: TrafficRanking): QueryClient {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  });
  clients.push(client);
  client.setQueryData(["groups"], []);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  client.setQueryData(["dictionaries", "platform"], { items: [] });
  if (data) client.setQueryData(["traffic-ranking", "24h", "all", "traffic"], data);
  render(
    <QueryClientProvider client={client}>
      <TrafficRankingPage />
    </QueryClientProvider>,
  );
  return client;
}

describe("流量排行筛选", () => {
  it("键盘选择平台时只显示匹配账号，并保留原始名次和占比", async () => {
    const user = userEvent.setup();
    renderRanking({
      ...rankingFixture,
      accounts: [
        { ...rankingFixture.accounts[0], rank: 4, traffic_share: 25 },
        rankingFixture.accounts[1],
      ],
    });
    const trigger = screen.getByRole("button", { name: "平台筛选" });
    trigger.focus();
    await user.keyboard("{Enter}");
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    await user.type(screen.getByRole("combobox", { name: "搜索平台" }), "anthropic{Enter}");
    expect(screen.getByRole("option")).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Escape}");

    expect(trigger).toHaveFocus();
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("待接入账号")).not.toBeInTheDocument();
    const row = screen.getByRole("row", { name: /稳定主账号/ });
    expect(within(row).getByLabelText("第 4 名")).toBeInTheDocument();
    expect(within(row).getByText("25.00%")).toBeInTheDocument();
  });

  it("切换为延迟排行时在延迟列展示升序状态", async () => {
    const user = userEvent.setup();
    const client = renderRanking(rankingFixture);
    client.setQueryData(["traffic-ranking", "24h", "all", "latency"], {
      ...rankingFixture,
      sort_by: "latency",
    });
    await user.click(screen.getByRole("button", { name: "排行维度筛选" }));
    await user.click(screen.getByRole("option", { name: "按 P95 延迟" }));
    await user.keyboard("{Escape}");

    expect(screen.getByRole("columnheader", { name: /响应延迟/ })).toHaveAttribute(
      "aria-sort",
      "ascending",
    );
    expect(screen.getByRole("columnheader", { name: /请求数/ })).not.toHaveAttribute("aria-sort");
  });

  it("切换到按天统计的时间范围时活跃时段使用天数单位", async () => {
    const user = userEvent.setup();
    const client = renderRanking(rankingFixture);
    client.setQueryData(["traffic-ranking", "7d", "all", "traffic"], {
      ...rankingFixture,
      bucket: "day",
      accounts: [{ ...rankingFixture.accounts[0], active_buckets: 5, total_buckets: 7 }],
    });
    await user.click(screen.getByRole("button", { name: "时间范围筛选" }));
    await user.click(screen.getByRole("option", { name: "最近 7 天" }));
    await user.keyboard("{Escape}");

    expect(screen.getByText("5 / 7 天")).toBeInTheDocument();
    expect(screen.queryByText("22 / 24 小时")).not.toBeInTheDocument();
  });

  it("搜索没有结果时展示空状态，清空搜索后恢复账号", async () => {
    const user = userEvent.setup();
    renderRanking(rankingFixture);
    const search = screen.getByRole("textbox", { name: "搜索账号" });
    await user.type(search, "missing");
    expect(screen.getByText("当前范围没有匹配的账号流量")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();

    await user.clear(search);
    expect(screen.queryByText("当前范围没有匹配的账号流量")).not.toBeInTheDocument();
    expect(screen.getByText("稳定主账号")).toBeInTheDocument();
    expect(screen.getByText("待接入账号")).toBeInTheDocument();
  });
});

describe("流量排行读取状态", () => {
  it("首次请求未完成时展示骨架且禁用刷新，完成后展示排行", async () => {
    let releaseResponse: (response: Response) => void = () => {};
    const response = new Promise<Response>((resolve) => {
      releaseResponse = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(() => response),
    );
    renderRanking();
    try {
      expect(screen.getByRole("status", { name: "流量排行加载中" })).toHaveAttribute(
        "aria-busy",
        "true",
      );
      expect(screen.getByRole("button", { name: "刷新流量排行" })).toBeDisabled();
      expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    } finally {
      releaseResponse(new Response(JSON.stringify(rankingFixture)));
    }
    expect(await screen.findByText("稳定主账号")).toBeInTheDocument();
    expect(screen.queryByRole("status", { name: "流量排行加载中" })).not.toBeInTheDocument();
  });

  it("首次请求失败时提供重试且不显示空结果，重试成功恢复排行", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ detail: "排行服务暂不可用" }), { status: 503 }),
        )
        .mockResolvedValueOnce(new Response(JSON.stringify(rankingFixture))),
    );
    renderRanking();
    const retry = await screen.findByRole("button", { name: "重新读取" });
    expect(screen.queryByText("当前范围没有匹配的账号流量")).not.toBeInTheDocument();
    expect(screen.queryByText("排行服务暂不可用")).not.toBeInTheDocument();

    await user.click(retry);
    expect(await screen.findByText("稳定主账号")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  });
});

it("字典顺序变更时流量排行分组筛选立即更新", async () => {
  const client = renderRanking(rankingFixture);
  act(() => {
    client.setQueryData(
      ["groups"],
      [
        { id: "1", name: "A" },
        { id: "2", name: "B" },
      ],
    );
    client.setQueryData(["dictionaries", "group"], {
      items: [
        { value: "2", enabled: true },
        { value: "1", enabled: true },
      ],
    });
  });
  fireEvent.click(screen.getByRole("button", { name: "账号分组筛选" }));
  expect((await screen.findAllByRole("option")).map((item) => item.textContent)).toEqual([
    "B",
    "A",
  ]);
});
