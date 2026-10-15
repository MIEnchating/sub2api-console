import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import { VaultPage } from "../vault-page";

afterEach(() => vi.unstubAllGlobals());

function setup(): Record<string, unknown>[] {
  const writes: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      writes.push(JSON.parse(String(init?.body)) as Record<string, unknown>);
      return Response.json({ configured: true });
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["auth-recovery-config"], {
    auth_records: [],
    vault_entries: [
      {
        id: "operator",
        entry: "operator",
        hosts: [],
        has_username: true,
        has_password: true,
        username_is_email: false,
        header_names: ["X-Private"],
      },
    ],
  });
  const view = render(
    <QueryClientProvider client={client}>
      <VaultPage />
    </QueryClientProvider>,
  );
  onTestFinished(() => {
    view.unmount();
    client.clear();
  });
  fireEvent.click(screen.getByRole("button", { name: "编辑凭据" }));
  return writes;
}

it("打开编辑弹窗时按用途分区，登录字段窄屏单列且请求头编辑器高度受限", async () => {
  setup();
  expect(screen.getByRole("group", { name: "基本信息" })).toBeVisible();
  expect(screen.getByRole("group", { name: "登录凭据" })).toHaveClass("sm:grid-cols-2");
  const headers = document.querySelector('[data-slot="json-editor"]');
  expect(headers).toBeTruthy();
  expect(headers).toHaveClass("h-36");
  const dialog = screen.getByRole("dialog", { name: "编辑凭据" });
  expect(dialog.querySelector('[data-slot="dialog-body"]')).toHaveClass("overflow-y-auto");
  expect(dialog.querySelector('[data-slot="dialog-body"]')).not.toContainElement(
    screen.getByRole("button", { name: "保存修改" }),
  );
});

it("清除密码后显示待保存状态，撤销后保存保留原密码", async () => {
  const writes = setup();
  fireEvent.click(screen.getByRole("button", { name: "清除密码" }));
  expect(screen.getByText("保存后将移除此密码")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "撤销清除密码" }));
  expect(screen.queryByText("保存后将移除此密码")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(writes).toEqual([{ id: "operator", entry: "operator" }]));
});

it("明确清除密码并保存时只清除该字段", async () => {
  const writes = setup();
  fireEvent.click(screen.getByRole("button", { name: "清除密码" }));
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() =>
    expect(writes).toEqual([{ id: "operator", entry: "operator", password: null }]),
  );
});
