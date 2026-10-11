import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { seedDictionaries } from "@/test/dictionary-fixtures";
import { TrafficRankingPage } from "../components/traffic-ranking-page";
import { rankingFixture } from "./fixtures";

let client: QueryClient | undefined;

afterEach(() => {
  cleanup();
  client?.clear();
  client = undefined;
  vi.unstubAllGlobals();
});

it("选择名称为 all 的分组只查询该分组，清除选择才恢复全部账号", async () => {
  const user = userEvent.setup();
  const scopes: (string | null)[] = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url = new URL(String(input), "http://localhost");
    scopes.push(url.searchParams.get("group"));
    return Promise.resolve(
      Response.json({
        ...rankingFixture,
        accounts:
          url.searchParams.get("group") === "all"
            ? [rankingFixture.accounts[0]]
            : rankingFixture.accounts,
      }),
    );
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  seedDictionaries(client, { group: [{ value: "group-all", name: "all" }] });
  client.setQueryData(["groups"], [{ id: "group-all", name: "all" }]);
  render(
    <QueryClientProvider client={client}>
      <TrafficRankingPage />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("待接入账号")).toBeVisible();

  const filter = screen.getByRole("button", { name: "账号分组筛选" });
  await user.click(filter);
  await user.click(screen.getByRole("option", { name: "all" }));
  await waitFor(() => expect(scopes).toContain("all"));
  expect(screen.getByRole("option", { name: "all" })).toHaveAttribute("aria-selected", "true");
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByText("待接入账号")).not.toBeInTheDocument());
  expect(screen.getByText("稳定主账号")).toBeVisible();

  await user.click(filter);
  await user.click(screen.getByRole("option", { name: "all" }));
  await user.keyboard("{Escape}");
  expect(await screen.findByText("待接入账号")).toBeVisible();
});
