import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";

import { VaultPage } from "../vault-page";

afterEach(() => vi.unstubAllGlobals());

function renderEditor(): Record<string, unknown>[] {
  const writes: Record<string, unknown>[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST")
        writes.push(JSON.parse(String(init.body)) as Record<string, unknown>);
      return new Response(JSON.stringify({ entry: "operator", configured: true }));
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } });
  client.setQueryData(["auth-recovery-config"], {
    auth_records: [],
    vault_entries: [
      {
        entry: "operator",
        hosts: ["api.example.test"],
        has_username: true,
        has_password: true,
        username_is_email: true,
        header_names: ["Authorization"],
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

it.each([
  { field: "password", selector: 'input[type="password"]', value: "temporary-password" },
  {
    field: "username",
    selector: 'input[autocomplete="username"]',
    value: "temporary@example.test",
  },
])("编辑已有凭据的 $field 后重新留空时保留原值", async (field) => {
  const writes = renderEditor();
  const input = screen
    .getAllByPlaceholderText("已配置，留空则不修改")
    .find((element) => element.matches(field.selector));
  if (!input) throw new Error("缺少凭据输入框");

  fireEvent.change(input, { target: { value: field.value } });
  fireEvent.change(input, { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));

  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).not.toHaveProperty(field.field);
});

it("编辑已有 Headers 后重新留空时保留原值", async () => {
  const user = userEvent.setup();
  const writes = renderEditor();
  const headers = await screen.findByRole("textbox", { name: "Headers JSON" }, { timeout: 5000 });
  await user.click(headers);
  await user.paste('{"Authorization":"temporary"}');
  await user.keyboard("{Control>}a{/Control}{Backspace}");
  await user.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).not.toHaveProperty("headers");
});

it("填写 Headers JSON 后提交新值且不更改其他凭据", async () => {
  const user = userEvent.setup();
  const writes = renderEditor();
  await user.click(await screen.findByRole("textbox", { name: "Headers JSON" }, { timeout: 5000 }));
  await user.paste('{"X-Client":"replacement"}');
  await user.click(screen.getByRole("button", { name: "保存修改" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toEqual({ entry: "operator", headers: { "X-Client": "replacement" } });
});

it("明确清除 Headers 时提交空对象并保留其他敏感字段", async () => {
  const writes = renderEditor();
  fireEvent.click(screen.getByRole("button", { name: "清除 Headers" }));
  fireEvent.click(screen.getByRole("button", { name: "保存修改" }));

  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toEqual({ entry: "operator", headers: {} });
});
