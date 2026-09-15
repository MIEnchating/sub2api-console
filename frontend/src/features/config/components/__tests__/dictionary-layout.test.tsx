import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { DictionaryManagement } from "../dictionary-management";

const clients: QueryClient[] = [];

function renderDictionary(): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <DictionaryManagement />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("首次读取字典时显示表格骨架，读取结束后显示空状态", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  renderDictionary();
  expect(screen.getAllByRole("row", { name: "正在读取字典" }).length).toBeGreaterThan(0);
  expect(screen.queryByText("暂无字典数据")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "刷新字典" })).toBeDisabled();
  await act(async () => resolve(new Response(JSON.stringify({ items: [] }))));
  expect(await screen.findByText("暂无字典数据")).toBeVisible();
  expect(screen.queryByRole("row", { name: "正在读取字典" })).not.toBeInTheDocument();
});

it("字典读取失败时提供重试，重试成功恢复表格空状态", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ detail: "字典服务暂不可用" }), { status: 503 }),
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }))),
  );
  renderDictionary();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByText("暂无字典数据")).not.toBeInTheDocument();
  fireEvent.click(retry);
  await waitFor(() =>
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument(),
  );
  expect(await screen.findByText("暂无字典数据")).toBeVisible();
});
