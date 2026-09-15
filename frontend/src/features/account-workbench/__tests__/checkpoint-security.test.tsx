import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  WorkbenchOAuthCheckpoint,
  WorkbenchOAuthSession,
  WorkbenchSecuritySession,
} from "@/api";
import { WorkbenchCheckpointSecurity } from "../components/workbench-checkpoint-security";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(options: { rejectIdentity?: boolean; authorize?: () => Promise<Response> } = {}) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const checkpoint: WorkbenchOAuthCheckpoint = {
    id: "paused-checkpoint",
    source_task_id: "old-oauth",
    scope: "local-export",
    revision: 7,
    created_at: "2026-09-14T00:00:00Z",
    checkpoint_revision: 4,
    status: "ready",
    can_restore: true,
    stage: "phone",
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };
  let session: WorkbenchSecuritySession = {
    id: "checkpoint-security",
    task_id: "checkpoint-security",
    source_checkpoint_id: checkpoint.id,
    scope: "local-export",
    operation: "totp",
    email: "",
    status: "waiting",
    message: "等待核对官方身份",
    expires_at: checkpoint.expires_at,
    width: 1100,
    height: 760,
  };
  const oauth: WorkbenchOAuthSession = {
    id: "fresh-oauth",
    task_id: "fresh-oauth",
    scope: "local-export",
    host: "auth.openai.com",
    status: "waiting",
    message: "新授权等待登录",
    expires_at: checkpoint.expires_at,
    width: 1100,
    height: 760,
  };
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input),
        method = init?.method ?? "GET";
      requests.push({
        path,
        method,
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      });
      if (method === "DELETE") return Response.json({ cancelled: true });
      if (path.endsWith("/oauth-checkpoints/paused-checkpoint/security"))
        return Response.json(session);
      if (path.endsWith("/confirm-identity")) {
        if (options.rejectIdentity)
          return Response.json({ detail: "官方身份已变化，请重新读取" }, { status: 409 });
        session = { ...session, identity_confirmed: true, status: "waiting" };
        return Response.json({ accepted: true });
      }
      if (path.endsWith("/continue")) {
        session = session.identity_confirmed
          ? {
              ...session,
              status: "succeeded",
              artifact_id: "private-checkpoint-security",
              message: "安全设置完成",
            }
          : {
              ...session,
              status: "awaiting_confirmation",
              email: "verified@example.test",
              user_id: "verified-user",
              message: "请确认官方身份",
            };
        return Response.json({ accepted: true });
      }
      if (path.endsWith("/security/checkpoint-security/oauth"))
        return options.authorize?.() ?? Response.json(oauth);
      if (path.endsWith("/security/checkpoint-security")) return Response.json(session);
      throw new Error(`未预期请求 ${method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const onOAuth = vi.fn(),
    onClose = vi.fn();
  const result = render(
    <QueryClientProvider client={client}>
      <WorkbenchCheckpointSecurity checkpoint={checkpoint} onClose={onClose} onOAuth={onOAuth} />
    </QueryClientProvider>,
  );
  return { ...result, requests, onOAuth, oauth };
}
async function start(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole("button", { name: "查看操作范围" }));
  expect(
    within(screen.getByRole("region", { name: "确认安全操作" })).getByText(/原授权不能再恢复/),
  ).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认并开始安全设置" }));
  await user.click(await screen.findByRole("button", { name: "验证完成，继续" }));
  await screen.findByRole("region", { name: "确认官方账号身份" });
}
async function complete(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await start(user);
  await user.click(screen.getByRole("button", { name: "确认此官方账号并继续" }));
  await waitFor(() =>
    expect(screen.queryByRole("region", { name: "确认官方账号身份" })).not.toBeInTheDocument(),
  );
  await user.click(screen.getByRole("button", { name: "验证完成，继续" }));
  await screen.findByText("私有结果 ID：private-checkpoint-security");
}

describe("检查点安全流程", () => {
  it("消费检查点后确认真实官方身份，安全设置成功后显式发起新授权", async () => {
    const view = mount();
    const user = userEvent.setup();
    await start(user);
    expect(screen.getByRole("button", { name: "验证完成，继续" })).toBeDisabled();
    expect(view.requests[0].body).toEqual({
      scope: "local-export",
      revision: 7,
      checkpoint_revision: 4,
      operation: "totp",
      confirmed: true,
    });
    expect(screen.queryByRole("combobox", { name: "安全设置账号" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认此官方账号并继续" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "验证完成，继续" })).toBeEnabled(),
    );
    expect(view.requests.find((item) => item.path.endsWith("/confirm-identity"))?.body).toEqual({
      email: "verified@example.test",
      user_id: "verified-user",
      confirmed: true,
    });
    await user.click(screen.getByRole("button", { name: "验证完成，继续" }));
    await user.click(await screen.findByRole("button", { name: "确认发起新的 OAuth 授权" }));
    await waitFor(() => expect(view.onOAuth).toHaveBeenCalledWith(view.oauth));
    expect(view.requests.some((item) => item.path.includes("/restore"))).toBe(false);
  });
  it("身份复核被拒绝时保留确认状态并禁止继续安全操作", async () => {
    const view = mount({ rejectIdentity: true });
    const user = userEvent.setup();
    await start(user);
    await user.click(screen.getByRole("button", { name: "确认此官方账号并继续" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "确认此官方账号并继续" })).toBeEnabled(),
    );
    expect(screen.getByRole("region", { name: "确认官方账号身份" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "验证完成，继续" })).toBeDisabled();
    expect(view.onOAuth).not.toHaveBeenCalled();
  });
  it("等待新授权期间关闭时撤销迟到授权且不交给已关闭页面", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const view = mount({
      authorize: () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    });
    const user = userEvent.setup();
    await complete(user);
    await user.click(screen.getByRole("button", { name: "确认发起新的 OAuth 授权" }));
    await screen.findByRole("button", { name: "正在发起新授权" });
    view.unmount();
    await act(async () => resolve(Response.json(view.oauth)));
    await waitFor(() =>
      expect(
        view.requests.some(
          (item) => item.path.endsWith("/oauth/fresh-oauth") && item.method === "DELETE",
        ),
      ).toBe(true),
    );
    expect(view.onOAuth).not.toHaveBeenCalled();
  });
});
