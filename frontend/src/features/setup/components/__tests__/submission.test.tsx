import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SetupPage } from "@/app-shell";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function setup(targetConfigured = true): ReturnType<typeof vi.fn> {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const complete = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <SetupPage
        status={{
          initialized: false,
          target_configured: targetConfigured,
          setup_token_required: false,
        }}
        onComplete={complete}
      />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByLabelText("控制台账号", { exact: true }), {
    target: { value: "operator" },
  });
  for (const label of ["控制台密码", "确认控制台密码"])
    fireEvent.change(screen.getByLabelText(label, { exact: true }), {
      target: { value: "private-password" },
    });
  return complete;
}

it("初始化提交期间锁定全部凭据，失败后保留输入并恢复提交", async () => {
  let finish!: (response: Response) => void;
  vi.stubGlobal("fetch", (path: string) =>
    path.endsWith("/status")
      ? Promise.resolve(Response.json({ initialized: false }))
      : new Promise<Response>((resolve) => {
          finish = resolve;
        }),
  );
  setup();
  fireEvent.click(screen.getByRole("button", { name: "完成初始化" }));
  await waitFor(() => expect(finish).toBeDefined());
  expect(screen.getByLabelText("控制台账号", { exact: true })).toBeDisabled();
  expect(screen.getByLabelText("控制台密码", { exact: true })).toBeDisabled();
  await act(async () => finish(Response.json({ detail: "配置库不可用" }, { status: 503 })));
  await waitFor(() => expect(screen.getByRole("button", { name: "完成初始化" })).toBeEnabled());
  expect(screen.getByLabelText("控制台密码", { exact: true })).toHaveValue("private-password");
});

it("初始化响应丢失但状态已完成时确认状态后离开表单，不重复提交", async () => {
  const fetch = vi.fn(async (path: string) => {
    if (path.endsWith("/status"))
      return Response.json({ initialized: true, target_configured: true });
    throw new TypeError("连接中断");
  });
  vi.stubGlobal("fetch", fetch);
  const complete = setup();
  fireEvent.click(screen.getByRole("button", { name: "完成初始化" }));
  await waitFor(() => expect(complete).toHaveBeenCalledOnce());
  expect(fetch.mock.calls.filter(([path]) => path.endsWith("/initialize"))).toHaveLength(1);
  expect(screen.getByLabelText("控制台密码", { exact: true })).toHaveValue("");
});

it.each([
  {
    name: "空管理目标",
    username: "operator",
    url: "",
    key: "",
    error: "请填写管理地址和管理密钥，或选择仅使用本地账号工作台",
  },
  {
    name: "非HTTP地址",
    username: "operator",
    url: "ftp://isolated.example",
    key: "test-key",
    error: "请输入完整的 http/https 地址",
  },
  {
    name: "单个Unicode字符账号",
    username: "😀",
    url: "https://isolated.example",
    key: "test-key",
    error: "账号至少 2 个字符",
  },
])("$name 在提交前给出字段提示", async (fixture) => {
  const fetch = vi.fn(async () => Response.json({ initialized: true }));
  vi.stubGlobal("fetch", fetch);
  setup(false);
  fireEvent.change(screen.getByLabelText("控制台账号", { exact: true }), {
    target: { value: fixture.username },
  });
  fireEvent.change(screen.getByLabelText("Admin Base URL"), { target: { value: fixture.url } });
  fireEvent.change(screen.getByLabelText("Admin Key"), { target: { value: fixture.key } });
  fireEvent.click(screen.getByRole("button", { name: "完成初始化" }));
  expect(await screen.findByText(fixture.error)).toBeVisible();
  expect(fetch).not.toHaveBeenCalled();
});
