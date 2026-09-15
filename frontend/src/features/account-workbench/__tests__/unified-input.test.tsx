import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountWorkbenchPage } from "../components/account-workbench-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it.each([
  ["JSON", '{"access_token":"fixture-access"}'],
  ["RT", "rt_fixture_refresh"],
  ["邮箱登录资料", "operator@example.test----fixture-password"],
  [
    "混合资料",
    '{"access_token":"fixture-access"}\nrt_fixture_refresh\noperator@example.test----fixture-password',
  ],
])("默认入口粘贴%s后无需选择格式，原文交给统一预览且确认前不启动任务", async (_kind, content) => {
  const requests: { path: string; method: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url, init) => {
      const path = String(url);
      requests.push({
        path,
        method: init?.method ?? "GET",
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (path.endsWith("/templates")) return Response.json([]);
      if (path.endsWith("/runs/preview"))
        return Response.json({
          id: "unified-preview",
          target: "https://console.example.test",
          export_only: false,
          expires_at: new Date(Date.now() + 600000).toISOString(),
          items: [],
          errors: [{ index: 0, message: "测试范围待核对" }],
        });
      return Response.json([]);
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(["setup-status"], { target_configured: true });
  render(
    <QueryClientProvider client={client}>
      <AccountWorkbenchPage />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  const input = await screen.findByRole("textbox", { name: "账号内容" });
  expect(screen.queryByRole("combobox", { name: "输入格式" })).not.toBeInTheDocument();
  await user.click(input);
  await user.paste(content);
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await waitFor(() =>
    expect(requests.find((request) => request.path.endsWith("/runs/preview"))?.body).toMatchObject({
      content,
      export_only: false,
      recovery_enabled: false,
    }),
  );
  expect(requests.filter((request) => request.method === "POST")).toHaveLength(1);
  expect(screen.queryByRole("textbox", { name: "账号内容" })).not.toBeInTheDocument();
});
