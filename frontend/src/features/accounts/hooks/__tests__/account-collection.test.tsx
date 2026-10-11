import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { useAccountCollection } from "../use-account-collection";

afterEach(() => vi.restoreAllMocks());

it("模型同步打开期间保留账号数据且暂停失效查询，关闭后恢复读取", async () => {
  const accounts = vi.spyOn(api, "accounts").mockResolvedValue([]);
  const checks = vi.spyOn(api, "modelCheckAccountStatuses").mockResolvedValue([]);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = (props: PropsWithChildren) => (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  );
  const view = renderHook((paused: boolean) => useAccountCollection(paused), {
    initialProps: false,
    wrapper,
  });
  await waitFor(() => expect(accounts).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(checks).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(view.result.current.accounts.data).toEqual([]));

  view.rerender(true);
  await client.invalidateQueries({ queryKey: ["accounts"] });
  await client.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
  expect(view.result.current.accounts.data).toEqual([]);
  expect(accounts).toHaveBeenCalledTimes(1);
  expect(checks).toHaveBeenCalledTimes(1);

  view.rerender(false);
  await waitFor(() => expect(accounts).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(checks).toHaveBeenCalledTimes(2));
  view.unmount();
  client.clear();
});
