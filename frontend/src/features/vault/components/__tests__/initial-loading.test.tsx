import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { VaultPage } from "../vault-page";

afterEach(() => vi.unstubAllGlobals());

it("现有凭据索引尚未返回时禁止新增，读取完成后允许添加", async () => {
  let resolveIndex!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveIndex = resolve;
        }),
    ),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <VaultPage />
    </QueryClientProvider>,
  );

  expect(screen.getByRole("button", { name: "添加凭据" })).toBeDisabled();
  await act(async () => {
    resolveIndex(new Response(JSON.stringify({ auth_records: [], vault_entries: [] })));
  });
  expect(await screen.findByText("暂无凭据")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "添加凭据" })).toBeEnabled();
});
