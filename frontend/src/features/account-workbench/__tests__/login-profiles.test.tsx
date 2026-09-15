import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import type { WorkbenchLoginProfile } from "@/api";
import { WorkbenchLoginProfileEditor } from "../components/workbench-login-profile-editor";
import { WorkbenchLoginProfiles } from "../components/workbench-login-profiles";

const profile: WorkbenchLoginProfile = {
  id: "profile-a",
  account_id: "42",
  user_id: "user-a",
  workspace_id: "workspace-a",
  email: "operator@example.test",
  revision: 4,
  updated_at: "2026-09-14T10:00:00Z",
  has_password: true,
  has_totp: true,
  has_proxy: true,
};
type Request = { path: string; method: string; body: unknown };
let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(
  view: ReactElement,
  handler?: (request: Request) => Promise<Response | undefined>,
): Request[] {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const request = {
        path: String(input),
        method: init?.method ?? "GET",
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      };
      requests.push(request);
      const response = await handler?.(request);
      if (response) return response;
      if (request.path === "/api/accounts")
        return Response.json([
          { id: "42", name: "账号甲", platform: "openai", account_type: "oauth" },
          { id: "43", name: "账号乙", platform: "openai", account_type: "oauth" },
        ]);
      if (request.method === "GET") return Response.json([profile]);
      return Response.json(profile);
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{view}</QueryClientProvider>);
  return requests;
}

describe("登录资料管理", () => {
  it("新增资料必须确认服务器保存且使用稳定账号ID，凭据不进入mutation变量", async () => {
    const close = vi.fn();
    const requests = mount(<WorkbenchLoginProfileEditor accountID="42" onClose={close} />);
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("登录邮箱"), profile.email);
    await user.type(screen.getByLabelText("登录密码"), "private-password");
    await user.type(
      screen.getByLabelText("登录代理"),
      "https://user:proxy-secret@proxy.example.test:8443",
    );
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    expect(requests).toHaveLength(0);
    expect(screen.getByRole("dialog", { name: "确认保存登录资料" })).toHaveTextContent(
      "账号 ID 42",
    );
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(requests[0]?.body).toEqual({
      account_id: "42",
      revision: 0,
      confirmed: true,
      login: {
        email: profile.email,
        password: "private-password",
        proxy_url: "https://user:proxy-secret@proxy.example.test:8443",
      },
    });
    expect(screen.getByLabelText("登录密码")).toHaveValue("");
    expect(screen.getByLabelText("登录代理")).toHaveValue("");
    expect(
      JSON.stringify(
        client
          .getMutationCache()
          .getAll()
          .map((mutation) => mutation.state.variables),
      ),
    ).not.toMatch(/private-password|proxy-secret/);
  });

  it("替换已有资料只回显身份摘要，确认完整替换并携带当前版本", async () => {
    const close = vi.fn();
    const requests = mount(
      <WorkbenchLoginProfileEditor accountID="42" profile={profile} onClose={close} />,
    );
    const user = userEvent.setup();
    expect(screen.getByLabelText("登录邮箱")).toHaveValue(profile.email);
    expect(screen.getByLabelText("登录邮箱")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("登录密码")).toHaveValue("");
    expect(screen.getByLabelText("TOTP 密钥")).toHaveValue("");
    expect(screen.getByLabelText("登录代理")).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    expect(screen.getByRole("dialog", { name: "确认保存登录资料" })).toHaveTextContent(
      "完整替换原资料",
    );
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(requests[0]?.body).toEqual({
      id: "profile-a",
      account_id: "42",
      revision: 4,
      confirmed: true,
      login: { email: profile.email, workspace_id: "workspace-a" },
    });
  });

  it("保存请求失败后清除已提交凭据并保留重新填写入口", async () => {
    const close = vi.fn();
    mount(
      <WorkbenchLoginProfileEditor accountID="42" profile={profile} onClose={close} />,
      async () => Response.json({ detail: "登录资料暂时无法保存" }, { status: 503 }),
    );
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("登录密码"), "old-secret");
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "确认保存登录资料" })).not.toBeInTheDocument(),
    );
    expect(close).not.toHaveBeenCalled();
    expect(screen.getByLabelText("登录密码")).toHaveValue("");
    expect(screen.getByRole("button", { name: "保存登录资料" })).toBeEnabled();
  });

  it("保存请求等待时禁止重复提交并在成功后关闭资料", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const response = new Promise<Response>((done) => {
      resolve = done;
    });
    const close = vi.fn();
    mount(
      <WorkbenchLoginProfileEditor accountID="42" profile={profile} onClose={close} />,
      async () => response,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    expect(await screen.findByRole("button", { name: "处理中…" })).toBeDisabled();
    await act(async () => {
      resolve(Response.json(profile));
    });
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
  });

  it("输入无效邮箱时展示字段错误且不发出保存请求", async () => {
    const requests = mount(
      <WorkbenchLoginProfileEditor accountID="42" onClose={() => undefined} />,
    );
    const user = userEvent.setup();
    await user.type(screen.getByLabelText("登录邮箱"), "invalid");
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    expect(await screen.findByText("请填写有效的登录邮箱")).toBeInTheDocument();
    expect(screen.getByLabelText("登录邮箱")).toHaveAttribute("aria-invalid", "true");
    expect(requests).toHaveLength(0);
  });

  it("选中资料后仅提交稳定账号ID，取消删除不会调用接口", async () => {
    const authorize = vi.fn();
    const requests = mount(<WorkbenchLoginProfiles onAuthorize={authorize} />);
    const user = userEvent.setup();
    expect(await screen.findByRole("button", { name: "预览重新授权（0）" })).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: `选择 ${profile.email}（ID 42）` }));
    await user.click(screen.getByRole("button", { name: "预览重新授权（1）" }));
    expect(authorize).toHaveBeenCalledWith(["42"]);
    await user.click(screen.getByRole("button", { name: `删除 ${profile.email} 的登录资料` }));
    await user.click(
      within(screen.getByRole("dialog", { name: "删除登录资料" })).getByRole("button", {
        name: "取消",
      }),
    );
    expect(requests.some((request) => request.method === "DELETE")).toBe(false);
  });

  it("确认删除携带资料版本且成功后清除选择", async () => {
    let removed = false;
    const requests = mount(
      <WorkbenchLoginProfiles onAuthorize={() => undefined} />,
      async (request) => {
        if (request.method === "DELETE") {
          removed = true;
          return Response.json({ deleted: true });
        }
        if (request.path.endsWith("/login-profiles") && removed) return Response.json([]);
      },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: `选择 ${profile.email}（ID 42）` }),
    );
    await user.click(screen.getByRole("button", { name: `删除 ${profile.email} 的登录资料` }));
    await user.click(screen.getByRole("button", { name: "确认删除登录资料" }));
    expect(await screen.findByText("暂无登录资料")).toBeInTheDocument();
    expect(requests.find((request) => request.method === "DELETE")).toEqual({
      path: "/api/account-workbench/login-profiles/profile-a",
      method: "DELETE",
      body: { revision: 4, confirmed: true },
    });
    expect(screen.getByRole("button", { name: "预览重新授权（0）" })).toBeDisabled();
  });

  it("保存版本冲突时关闭过期编辑并刷新资料，下次替换使用新版本", async () => {
    let conflicted = false;
    mount(<WorkbenchLoginProfiles onAuthorize={() => undefined} />, async (request) => {
      if (request.path.endsWith("/login-profiles") && request.method === "POST") {
        conflicted = true;
        return Response.json({ detail: "登录资料版本已变化" }, { status: 409 });
      }
      if (request.path.endsWith("/login-profiles") && conflicted)
        return Response.json([{ ...profile, revision: 5 }]);
    });
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: `替换 ${profile.email} 的登录资料` }),
    );
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "替换登录资料" })).not.toBeInTheDocument(),
    );
    await user.click(
      await screen.findByRole("button", { name: `替换 ${profile.email} 的登录资料` }),
    );
    expect(screen.getByRole("dialog", { name: "替换登录资料" })).toHaveTextContent("资料版本：5");
  });

  it("删除版本冲突时关闭过期确认并刷新资料，下次删除使用新版本", async () => {
    let conflicted = false;
    const requests = mount(
      <WorkbenchLoginProfiles onAuthorize={() => undefined} />,
      async (request) => {
        if (request.method === "DELETE") {
          conflicted = true;
          return Response.json({ detail: "登录资料版本已变化" }, { status: 409 });
        }
        if (request.path.endsWith("/login-profiles") && conflicted)
          return Response.json([{ ...profile, revision: 5 }]);
      },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: `删除 ${profile.email} 的登录资料` }),
    );
    await user.click(screen.getByRole("button", { name: "确认删除登录资料" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "删除登录资料" })).not.toBeInTheDocument(),
    );
    await user.click(
      await screen.findByRole("button", { name: `删除 ${profile.email} 的登录资料` }),
    );
    await user.click(screen.getByRole("button", { name: "确认删除登录资料" }));
    await waitFor(() =>
      expect(requests.filter((request) => request.method === "DELETE").at(-1)?.body).toEqual({
        revision: 5,
        confirmed: true,
      }),
    );
  });

  it("登录资料首次读取使用骨架，失败后可重试恢复空列表", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    let retry = false;
    mount(<WorkbenchLoginProfiles onAuthorize={() => undefined} />, async (request) => {
      if (request.path.endsWith("/login-profiles")) return retry ? Response.json([]) : pending;
    });
    expect(screen.getByRole("status", { name: "正在读取登录资料" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    await act(async () => {
      resolve(Response.json({ detail: "暂时不可读" }, { status: 503 }));
    });
    const button = await screen.findByRole("button", { name: "重新读取" });
    retry = true;
    await userEvent.setup().click(button);
    expect(await screen.findByText("暂无登录资料")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "预览重新授权（0）" })).toBeDisabled();
  });
});
