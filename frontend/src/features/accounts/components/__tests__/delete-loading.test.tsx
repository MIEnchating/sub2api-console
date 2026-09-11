import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AccountDeleteDialog } from "../account-delete-dialog";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("账号删除范围读取中显示轻量提示，不允许提前确认删除", () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountDeleteDialog
        accountId="41"
        open
        pending={false}
        activeAction={null}
        task={undefined}
        taskError={null}
        onOpenChange={vi.fn()}
        onConfirm={vi.fn()}
      />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("status", { name: "正在读取账号删除范围" })).toHaveTextContent(
    "正在读取账号删除范围",
  );
  expect(screen.getByRole("dialog").querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.queryByRole("button", { name: "确认删除" })).not.toBeInTheDocument();
});
