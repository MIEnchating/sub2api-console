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

it("账号批次启动后切换到记录页再继续，不取消或重复启动原任务", async () => {
  const requests: { path: string; method: string }[] = [];
  const row = {
    index: 0,
    kind: "refresh_token",
    name: "测试账号",
    has_password: false,
    has_totp: false,
    has_proxy: false,
    status: "queued",
    message: "等待处理",
  };
  const base = {
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [row],
    errors: [],
    export_only: false,
    target: "https://sub2api.example.test",
  };
  const run = {
    ...base,
    id: "run-one",
    task_id: "task-one",
    status: "waiting_input",
    available: 0,
    message: "正在准备本批账号",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url, init) => {
      const path = String(url);
      requests.push({ path, method: init?.method ?? "GET" });
      if (path.endsWith("/runs/preview")) return Response.json({ ...base, id: "preview-one" });
      if (path.endsWith("/runs") || path.endsWith("/runs/run-one")) return Response.json(run);
      return Response.json([]);
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["setup-status"], { target_configured: true });
  render(
    <QueryClientProvider client={client}>
      <AccountWorkbenchPage />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.type(await screen.findByRole("textbox", { name: "账号内容" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await user.click(await screen.findByRole("button", { name: "确认处理 1 项" }));
  await user.click(screen.getByRole("button", { name: "开始处理" }));
  await screen.findByRole("heading", { name: "账号处理进度" });
  await user.click(screen.getByRole("tab", { name: "处理记录" }));
  await user.click(await screen.findByRole("button", { name: "继续当前批次" }));
  expect(screen.getByRole("heading", { name: "账号处理进度" })).toBeVisible();
  expect(requests.some((item) => item.method === "DELETE")).toBe(false);
  await waitFor(() =>
    expect(
      requests.filter((item) => item.path.endsWith("/runs") && item.method === "POST"),
    ).toHaveLength(1),
  );
});
