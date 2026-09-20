import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { RunItems } from "../components/run-items";
import type { WorkbenchRun } from "../types";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function mount(kind = "email_code"): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const run: WorkbenchRun = {
    id: "run-one",
    revision: 1,
    task_id: "task-one",
    action: "export",
    status: "running",
    created_at: "2026-09-20T00:00:00Z",
    updated_at: "2026-09-20T00:00:00Z",
    expires_at: "2099-01-01T00:00:00Z",
    duplicate_count: 0,
    items: [
      {
        id: "item-one",
        index: 0,
        kind: "login",
        name: "",
        email: "login@example.test",
        plan: "",
        identity_source: "login",
        status: "waiting_input",
        message: "请输入验证码",
        template_name: "默认配置",
        login_prompt: { id: "prompt-one", kind: kind as "email_code" },
      },
    ],
  };
  render(
    <QueryClientProvider client={client}>
      <RunItems run={run} pending={true} onEnable={() => {}} />
    </QueryClientProvider>,
  );
}
it("协议要求邮箱验证码时提供输入框，提交绑定当前验证步骤且不请求浏览器画面", async () => {
  const fetcher = vi.fn<typeof fetch>(async () => Response.json({ accepted: true }));
  vi.stubGlobal("fetch", fetcher);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "输入验证码" }));
  expect(screen.getByRole("textbox", { name: "邮箱验证码" })).toBeVisible();
  await user.type(screen.getByRole("textbox", { name: "邮箱验证码" }), "123456");
  await user.click(screen.getByRole("button", { name: "提交验证" }));
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
  expect(String(fetcher.mock.calls[0]?.[0])).toBe(
    "/api/account-workbench/runs/run-one/login-input/item-one",
  );
  expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
    prompt_id: "prompt-one",
    value: "123456",
  });
  expect(screen.queryByText("完成验证")).not.toBeInTheDocument();
});
it("验证码不足六位时展示字段提示并且不发送请求", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "输入验证码" }));
  await user.type(screen.getByRole("textbox", { name: "邮箱验证码" }), "123");
  await user.click(screen.getByRole("button", { name: "提交验证" }));
  expect(await screen.findByText("请输入六位数字验证码")).toBeVisible();
  expect(fetcher).not.toHaveBeenCalled();
});

it("提交验证码期间防止重复提交，网络失败后保留输入并允许关闭", async () => {
  let rejectRequest: (error: Error) => void = () => {};
  const fetcher = vi.fn<typeof fetch>(
    () =>
      new Promise<Response>((_resolve, reject) => {
        rejectRequest = reject;
      }),
  );
  vi.stubGlobal("fetch", fetcher);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "输入验证码" }));
  await user.type(screen.getByRole("textbox", { name: "邮箱验证码" }), "234567");
  await user.keyboard("{Enter}");
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
  expect(screen.getByRole("button", { name: "正在提交…" })).toBeDisabled();
  expect(screen.getByRole("textbox", { name: "邮箱验证码" })).toBeDisabled();
  rejectRequest(new Error("isolated connection failed"));
  await waitFor(() => expect(screen.getByRole("button", { name: "提交验证" })).toBeEnabled());
  expect(screen.getByRole("textbox", { name: "邮箱验证码" })).toHaveValue("234567");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("邮箱验证码支持无需填写验证码直接重发，提交当前验证步骤", async () => {
  const fetcher = vi.fn<typeof fetch>(async () => Response.json({ accepted: true }));
  vi.stubGlobal("fetch", fetcher);
  mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "输入验证码" }));
  await user.click(screen.getByRole("button", { name: "重发邮箱验证码" }));
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetcher.mock.calls[0]?.[1]?.body))).toEqual({
    prompt_id: "prompt-one",
    value: "",
    action: "resend_email",
  });
});

it("2FA 验证码输入不提供邮箱重发操作", async () => {
  mount("totp_code");
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "输入验证码" }));
  expect(screen.queryByRole("button", { name: "重发邮箱验证码" })).not.toBeInTheDocument();
});
