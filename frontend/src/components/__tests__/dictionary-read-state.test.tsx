import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { dictionaryItems } from "@/test/dictionary-fixtures";
import { FilterMenu } from "../data-table/filter-menu";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

function PlatformFilter() {
  const options = useDictionaryOrder(
    "platform",
    [{ value: "openai", label: "本地名称" }],
    (item) => item.value,
  );
  return (
    <FilterMenu
      dictionaryKind="platform"
      label="平台"
      options={options.map((option) => option.value)}
      optionLabel={(value) => options.find((option) => option.value === value)?.label ?? value}
      value={null}
      onValueChange={() => undefined}
    />
  );
}

function setup(enabled = true): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <PlatformFilter />
    </QueryClientProvider>,
  );
  return client;
}

it("字典首次读取时显示等待，完成空字典后显示空选项且不补本地值", async () => {
  const client = setup(false);
  await userEvent.click(screen.getByRole("button", { name: "平台筛选" }));
  expect(screen.getByText("正在读取字典选项")).toBeVisible();
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
  act(() => client.setQueryData(["dictionaries", "platform"], { items: [] }));
  await screen.findByText("没有匹配项");
  expect(screen.queryByRole("option")).not.toBeInTheDocument();
});

it("字典读取失败时允许重试，恢复后使用字典名称", async () => {
  const request = vi.spyOn(api, "dictionaries").mockRejectedValue(new Error("读取失败"));
  setup();
  await userEvent.click(screen.getByRole("button", { name: "平台筛选" }));
  const retry = await screen.findByRole("button", { name: "重新读取" });
  request.mockResolvedValue({
    items: dictionaryItems("platform", [{ value: "openai", name: "字典名称" }]),
  });
  await userEvent.click(retry);
  expect(await screen.findByRole("option", { name: "字典名称" })).toBeVisible();
  expect(screen.queryByText("本地名称")).not.toBeInTheDocument();
});

it("后台刷新失败时保留缓存中的字典选项", async () => {
  vi.spyOn(api, "dictionaries").mockRejectedValue(new Error("读取失败"));
  const client = setup(false);
  act(() =>
    client.setQueryData(["dictionaries", "platform"], {
      items: dictionaryItems("platform", [{ value: "openai", name: "缓存名称" }]),
    }),
  );
  await userEvent.click(screen.getByRole("button", { name: "平台筛选" }));
  await act(async () => {
    try {
      await client.fetchQuery({
        queryKey: ["dictionaries", "platform"],
        queryFn: () => api.dictionaries("platform"),
      });
    } catch {
      /* Keep cached data after network failure. */
    }
  });
  await waitFor(() => expect(screen.getByRole("option", { name: "缓存名称" })).toBeVisible());
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
