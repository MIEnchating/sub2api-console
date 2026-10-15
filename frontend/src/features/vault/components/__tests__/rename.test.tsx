import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import { VaultPage } from "../vault-page";

afterEach(() => vi.unstubAllGlobals());

function setup(fail = false): Record<string, unknown>[] {
  const writes: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, init?: RequestInit) => {
      writes.push(JSON.parse(String(init?.body)) as Record<string, unknown>);
      return Response.json(
        fail
          ? { code: "vault_entry_exists", detail: "凭据名称已存在，请使用其他名称" }
          : { configured: true },
        { status: fail ? 409 : 200 },
      );
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["auth-recovery-config"], {
    auth_records: [],
    vault_entries: ["operator", "existing"].map((entry) => ({
      id: `vault-${entry}`,
      entry,
      hosts: ["api.example.test"],
      has_username: true,
      has_password: true,
      username_is_email: true,
      header_names: ["Authorization"],
    })),
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
  fireEvent.click(screen.getAllByRole("button", { name: "编辑凭据" })[0]);
  return writes;
}

it("编辑凭据时允许通过键盘改名，保存时携带原标识且省略未修改的敏感字段", async () => {
  const writes = setup();
  const user = userEvent.setup();
  const name = screen.getByRole("textbox", { name: "凭据名称" });
  expect(name).toBeEnabled();
  await user.clear(name);
  await user.type(name, "新凭据");
  await user.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(writes).toEqual([{ entry: "新凭据", id: "vault-operator" }]));
});

it("改成已存在的名称时显示字段错误且不覆盖其他凭据", async () => {
  const writes = setup();
  fireEvent.change(screen.getByLabelText("凭据名称"), { target: { value: "existing" } });
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  expect(await screen.findByText("凭据名称已存在，请使用其他名称")).toBeVisible();
  expect(screen.getByLabelText("凭据名称")).toHaveAttribute("aria-invalid", "true");
  expect(writes).toHaveLength(0);
});

it("重命名请求冲突时保留输入并允许修改名称后重试", async () => {
  const writes = setup(true);
  fireEvent.change(screen.getByLabelText("凭据名称"), { target: { value: "new-name" } });
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  expect(await screen.findByText("凭据名称已存在，请使用其他名称")).toBeVisible();
  expect(screen.getByLabelText("凭据名称")).toHaveValue("new-name");
  expect(screen.getByRole("button", { name: "保存修改" })).toBeEnabled();
  expect(writes).toHaveLength(1);
});

it("未改名保存时仍使用稳定 ID 定位原凭据", async () => {
  const writes = setup();
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(writes).toEqual([{ id: "vault-operator", entry: "operator" }]));
});
