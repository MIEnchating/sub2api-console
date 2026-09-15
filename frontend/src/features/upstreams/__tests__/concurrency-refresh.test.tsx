import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { UpstreamsPage } from "@/App";
import type { UpstreamSummary } from "@/api";
import { router } from "@/router";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
});

it("刷新列表取得最新并发后按上游分别更新额度和调度目标并保留选择", async () => {
  const upstream: UpstreamSummary["hosts"][number] = {
    upstream_id: "upstream-1",
    host: "limited.example.test",
    hosts: ["limited.example.test"],
    base_url: "https://limited.example.test",
    name: "有限并发上游",
    upstream_type: "sub2api",
    account_count: 2,
    group_count: 1,
    auth_status: "已鉴权",
    raw_balance: "10",
    balance: "10",
    recharge_rate: "1",
    balance_status: "available",
    checked_at: null,
    concurrency_limit: 10,
    concurrency_status: "stale",
    allocated_concurrency: 12,
    target_concurrency: 10,
  };
  const unchanged: UpstreamSummary["hosts"][number] = {
    ...upstream,
    upstream_id: "upstream-2",
    host: "unlimited.example.test",
    hosts: ["unlimited.example.test"],
    base_url: "https://unlimited.example.test",
    name: "不限并发上游",
    concurrency_limit: 0,
    concurrency_status: "unlimited",
    allocated_concurrency: 5,
    target_concurrency: null,
  };
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("fetch", async () =>
    Response.json({
      hosts: [
        { ...upstream, concurrency_limit: 20, concurrency_status: "known", target_concurrency: 18 },
        unchanged,
      ],
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["upstreams"], { hosts: [upstream, unchanged] });
  client.setQueryData(["config"], { mode: "监控模式" });
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <UpstreamsPage />
      </RouterContextProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "选择上游 有限并发上游" }));
  expect(screen.getByText("缓存额度")).toBeVisible();

  await user.click(screen.getByRole("button", { name: "刷新上游列表" }));

  const updatedRow = within(screen.getByRole("row", { name: /有限并发上游/ }));
  await waitFor(() => expect(updatedRow.getByLabelText("用户上限")).toHaveTextContent("20"));
  expect(updatedRow.getByLabelText("已配置并发")).toHaveTextContent("12");
  expect(updatedRow.getByLabelText("目标并发")).toHaveTextContent("18");
  expect(updatedRow.getByRole("checkbox")).toBeChecked();
  expect(screen.queryByText("缓存额度")).not.toBeInTheDocument();
  expect(screen.queryByText("已超上限")).not.toBeInTheDocument();
  const unchangedRow = within(screen.getByRole("row", { name: /不限并发上游/ }));
  expect(unchangedRow.getByLabelText("用户上限")).toHaveTextContent("不限");
  expect(unchangedRow.getByLabelText("已配置并发")).toHaveTextContent("5");
  expect(unchangedRow.queryByLabelText("目标并发")).not.toBeInTheDocument();
});
