import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it } from "vitest";
import type { DictionaryKind } from "@/api";
import { dictionaryItems } from "@/test/dictionary-fixtures";
import { useDictionaryOrder } from "../use-dictionary-order";

const clients: QueryClient[] = [];
afterEach(() => clients.splice(0).forEach((client) => client.clear()));

it.each<DictionaryKind>([
  "platform",
  "group",
  "account_type",
  "upstream_type",
  "auth_status",
  "scheduling_strategy",
  "task_status",
  "account_status",
  "alert_status",
  "kuma_monitor_type",
])("%s 只展示字典启用的已实现选项，并同步字典名称与顺序", async (kind) => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  clients.push(client);
  const key = ["dictionaries", kind];
  client.setQueryData(key, {
    items: dictionaryItems(kind, [
      { value: "B", name: "字典 B" },
      { value: "A", name: "字典 A" },
      { value: "future", name: "未实现" },
    ]),
  });
  const items = [
    { value: "A", label: "本地 A", metadata: 1 },
    { value: "B", label: "本地 B", metadata: 2 },
    { value: "missing", label: "未登记", metadata: 3 },
  ];
  const view = renderHook(() => useDictionaryOrder(kind, items, (item) => item.value), {
    wrapper: (props: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
    ),
  });
  expect(view.result.current).toEqual([
    { value: "B", label: "字典 B", metadata: 2 },
    { value: "A", label: "字典 A", metadata: 1 },
  ]);

  act(() =>
    client.setQueryData(key, { items: dictionaryItems(kind, [{ value: "A", name: "更新名称" }]) }),
  );

  await waitFor(() =>
    expect(view.result.current).toEqual([{ value: "A", label: "更新名称", metadata: 1 }]),
  );
  expect(items[0].label).toBe("本地 A");
});
