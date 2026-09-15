import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  WorkbenchOAuthBatch as OAuthBatch,
  WorkbenchOAuthBatchPreview,
  WorkbenchSourceProfile,
  WorkbenchSourceProfileIdentity,
} from "@/api";
import { WorkbenchSourceProfileCreate } from "../components/workbench-source-profile-create";
import { WorkbenchSourceProfiles } from "../components/workbench-source-profiles";
import { WorkbenchArtifactProfile } from "../components/workbench-artifact-profile";
import { WorkbenchOAuthBatch } from "../components/workbench-oauth-batch";

const profile: WorkbenchSourceProfile = {
  id: "local-profile-a",
  scope: "local-export",
  revision: 4,
  email: "local@example.test",
  user_id: "official-user-a",
  workspace_id: "workspace-a",
  has_password: true,
  has_totp: true,
  has_proxy: false,
  updated_at: "2026-09-14T00:00:00Z",
};
const source = { artifact_id: "local-file-a", index: 0 };
function identity(): WorkbenchSourceProfileIdentity {
  return {
    scope: "local-export",
    source,
    source_revision: "a".repeat(64),
    email: profile.email,
    user_id: profile.user_id,
    workspace_id: profile.workspace_id,
    expires_at: new Date(Date.now() + 600000).toISOString(),
  };
}
type Request = { path: string; method: string; body: unknown };
let client: QueryClient;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  localStorage.clear();
});
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});
function mount(
  view: ReactElement,
  handler?: (request: Request) => Promise<Response | undefined>,
): Request[] {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const request: Request = {
        path: String(input),
        method: init?.method ?? "GET",
        body: init?.body ? (JSON.parse(String(init.body)) as unknown) : null,
      };
      requests.push(request);
      const response = await handler?.(request);
      if (response) return response;
      if (request.path.endsWith("/source-profiles/source")) return Response.json(identity());
      if (request.method === "DELETE") return Response.json({ deleted: true });
      if (
        request.path === "/api/account-workbench/source-profiles?scope=local-export" ||
        request.path.endsWith("/source-profiles")
      )
        return Response.json(request.method === "GET" ? [profile] : profile);
      return Response.json({ detail: "隔离测试未配置此请求" }, { status: 503 });
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{view}</QueryClientProvider>);
  return requests;
}

describe("本地登录资料", () => {
  it("从已验证本地来源保存时二次确认，凭据不写入查询和mutation缓存", async () => {
    const close = vi.fn();
    const requests = mount(<WorkbenchSourceProfileCreate source={source} onClose={close} />);
    const user = userEvent.setup();
    expect(await screen.findByLabelText("登录邮箱")).toHaveValue(profile.email);
    expect(screen.getByLabelText("登录邮箱")).toHaveAttribute("readonly");
    await user.type(screen.getByLabelText("登录密码"), "local-private-password");
    await user.type(screen.getByLabelText("TOTP 密钥"), "JBSWY3DPEHPK3PXP");
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    expect(requests.filter((item) => item.path.endsWith("/source-profiles"))).toHaveLength(0);
    expect(screen.getByRole("dialog", { name: "确认保存登录资料" })).toHaveTextContent(
      profile.user_id,
    );
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(requests.find((item) => item.path.endsWith("/source-profiles"))?.body).toEqual({
      scope: "local-export",
      source,
      source_revision: "a".repeat(64),
      confirmed: true,
      login: {
        email: profile.email,
        workspace_id: profile.workspace_id,
        password: "local-private-password",
        totp_secret: "JBSWY3DPEHPK3PXP",
      },
    });
    expect(screen.getByLabelText("登录密码")).toHaveValue("");
    expect(screen.getByLabelText("TOTP 密钥")).toHaveValue("");
    const cache = JSON.stringify({
      queries: client
        .getQueryCache()
        .getAll()
        .map((query) => query.state),
      mutations: client
        .getMutationCache()
        .getAll()
        .map((mutation) => mutation.state),
    });
    expect(cache).not.toMatch(/local-private-password|JBSWY3DPEHPK3PXP/);
    expect(localStorage.length).toBe(0);
  });

  it("替换发生版本冲突时关闭旧编辑并重新读取最新版本", async () => {
    let conflict = false;
    const requests = mount(
      <WorkbenchSourceProfiles onAuthorize={() => undefined} />,
      async (request) => {
        if (request.path.endsWith("/source-profiles") && request.method === "POST") {
          conflict = true;
          return Response.json({ detail: "本地登录资料版本已变化" }, { status: 409 });
        }
        if (
          request.path === "/api/account-workbench/source-profiles?scope=local-export" &&
          conflict
        )
          return Response.json([{ ...profile, revision: 5 }]);
      },
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: `替换本地资料 ${profile.email}` }));
    expect(screen.getByLabelText("登录密码")).toHaveValue("");
    await user.type(screen.getByLabelText("登录密码"), "replacement-private");
    await user.click(screen.getByRole("button", { name: "保存登录资料" }));
    expect(screen.getByRole("dialog", { name: "确认保存登录资料" })).toHaveTextContent(
      "完整替换原资料",
    );
    await user.click(screen.getByRole("button", { name: "确认保存到服务器" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "替换登录资料" })).not.toBeInTheDocument(),
    );
    expect(requests.find((item) => item.method === "POST")?.body).toMatchObject({
      scope: "local-export",
      id: profile.id,
      revision: 4,
    });
    await user.click(await screen.findByRole("button", { name: `替换本地资料 ${profile.email}` }));
    expect(screen.getByRole("dialog", { name: "替换登录资料" })).toHaveTextContent("资料版本：5");
    expect(screen.getByLabelText("登录密码")).toHaveValue("");
  });

  it("本地资料导出仅提交本地稳定ID和版本，预览版本冲突后刷新本地列表", async () => {
    let conflict = false;
    const requests = mount(
      <WorkbenchSourceProfiles onAuthorize={() => undefined} />,
      async (request) => {
        if (request.path.endsWith("/source-profiles/exports/preview")) {
          conflict = true;
          return Response.json({ detail: "本地登录资料版本已变化" }, { status: 409 });
        }
        if (
          request.path === "/api/account-workbench/source-profiles?scope=local-export" &&
          conflict
        )
          return Response.json([{ ...profile, revision: 5 }]);
      },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: `选择本地资料 ${profile.email}` }),
    );
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    await screen.findByRole("button", { name: "重新读取" });
    await user.click(screen.getByRole("button", { name: "返回" }));
    await waitFor(() =>
      expect(screen.getByRole("list", { name: "已保存本地登录资料" })).toHaveTextContent("版本 5"),
    );
    expect(
      requests.find((item) => item.path.endsWith("/source-profiles/exports/preview"))?.body,
    ).toEqual({ scope: "local-export", items: [{ id: profile.id, revision: 4 }] });
    expect(requests.some((item) => /\/login-profiles|\/exports\/profiles/.test(item.path))).toBe(
      false,
    );
  });

  it("本地资料导出确认范围与任务均使用本地接口", async () => {
    const requests = mount(
      <WorkbenchSourceProfiles onAuthorize={() => undefined} />,
      async (request) => {
        if (request.path.endsWith("/source-profiles/exports/preview"))
          return Response.json({
            id: "local-export-preview",
            scope: "local-export",
            target: "",
            kind: "login-profiles",
            items: [profile],
            expires_at: new Date(Date.now() + 600000).toISOString(),
          });
        if (
          request.path.endsWith("/source-profiles/exports") ||
          request.path === "/api/tasks/local-export-task"
        )
          return Response.json({
            id: "local-export-task",
            skill: "account-workbench",
            operation: "account-workbench-profile-export",
            status: "succeeded",
            progress: 100,
            message: "本地资料文件已生成",
            result: {},
            created_at: "2026-09-14T00:00:00Z",
            updated_at: "2026-09-14T00:00:00Z",
          });
      },
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: `选择本地资料 ${profile.email}` }),
    );
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    const dialog = await screen.findByRole("dialog", { name: "确认导出登录资料" });
    await within(dialog).findByRole("list", { name: "资料导出范围" });
    expect(dialog).toHaveTextContent("范围：本地登录资料");
    expect(dialog).not.toHaveTextContent("管理目标：");
    await user.click(within(dialog).getByRole("button", { name: "确认生成资料文件" }));
    await waitFor(() =>
      expect(requests.some((item) => item.path.endsWith("/source-profiles/exports"))).toBe(true),
    );
    expect(requests.find((item) => item.path.endsWith("/source-profiles/exports"))?.body).toEqual({
      preview_id: "local-export-preview",
      confirmed: true,
    });
  });

  it("取消读取身份保留关闭入口，迟到响应不会保存凭据", async () => {
    let resolve: (value: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const close = vi.fn();
    const requests = mount(
      <WorkbenchSourceProfileCreate source={source} onClose={close} />,
      async () => pending,
    );
    const user = userEvent.setup();
    expect(screen.getByText("正在核对本地账号身份")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "保存登录资料" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(close).toHaveBeenCalledOnce();
    cleanup();
    await act(async () => {
      resolve(Response.json(identity()));
    });
    await waitFor(() => expect(client.getQueryCache().getAll()).toHaveLength(0));
    expect(requests).toHaveLength(1);
  });

  it("身份来源已过期时禁止保存但允许取消", async () => {
    const close = vi.fn();
    const requests = mount(
      <WorkbenchSourceProfileCreate source={source} onClose={close} />,
      async () => Response.json({ ...identity(), expires_at: "2000-01-01T00:00:00Z" }),
    );
    const user = userEvent.setup();
    await screen.findByText("身份来源已过期，请重新选择来源");
    expect(screen.getByRole("button", { name: "保存登录资料" })).toBeDisabled();
    expect(screen.getByLabelText("登录密码")).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "取消" }));
    expect(close).toHaveBeenCalledOnce();
    expect(requests).toHaveLength(1);
  });

  it("多账号私有文件选择无效序号时禁用核对，有效序号转换为零基稳定索引", async () => {
    const requests = mount(
      <WorkbenchArtifactProfile
        artifact={{
          id: source.artifact_id,
          kind: "accounts",
          count: 2,
          created_at: "2026-09-14T00:00:00Z",
          expires_at: new Date(Date.now() + 600000).toISOString(),
        }}
        onClose={() => undefined}
      />,
    );
    const user = userEvent.setup();
    await user.clear(screen.getByLabelText("账号序号"));
    expect(screen.getByRole("button", { name: "核对所选账号身份" })).toBeDisabled();
    await user.type(screen.getByLabelText("账号序号"), "3");
    expect(screen.getByRole("button", { name: "核对所选账号身份" })).toBeDisabled();
    await user.clear(screen.getByLabelText("账号序号"));
    await user.type(screen.getByLabelText("账号序号"), "2");
    await user.click(screen.getByRole("button", { name: "核对所选账号身份" }));
    await screen.findByLabelText("登录邮箱");
    expect(requests[0]?.body).toEqual({
      scope: "local-export",
      source: { artifact_id: source.artifact_id, index: 1 },
    });
  });

  it("本地重新登录只提交所选资料ID和版本，失败项重试不包含成功资料", async () => {
    const second: WorkbenchSourceProfile = {
      ...profile,
      id: "local-profile-b",
      email: "second@example.test",
      revision: 7,
    };
    const rows = [profile, second].map((item, index) => ({
      index,
      email: item.email,
      user_id: item.user_id,
      workspace_id: item.workspace_id,
      profile_id: item.id,
      profile_revision: item.revision,
      has_password: true,
      has_totp: false,
      status: "queued" as const,
      message: "等待授权",
    }));
    const batchPreview: WorkbenchOAuthBatchPreview = {
      id: "local-batch-preview",
      scope: "local-export",
      fresh_login: true,
      target: "",
      expires_at: new Date(Date.now() + 600000).toISOString(),
      items: rows,
      errors: [],
    };
    const batch: OAuthBatch = {
      id: "local-batch",
      task_id: "local-batch",
      scope: "local-export",
      fresh_login: true,
      status: "authorized",
      message: "本批授权已结束",
      available: 1,
      expires_at: batchPreview.expires_at,
      items: [
        { ...rows[0]!, status: "succeeded" },
        { ...rows[1]!, status: "failed" },
      ],
    };
    const requests = mount(
      <WorkbenchOAuthBatch scope="local-export" localProfiles />,
      async (request) => {
        if (request.path === "/api/account-workbench/source-profiles?scope=local-export")
          return Response.json([profile, second]);
        if (request.path.endsWith("/reauthorization/preview")) return Response.json(batchPreview);
        if (
          request.path === "/api/account-workbench/oauth-batches" ||
          request.path === "/api/account-workbench/oauth-batches/local-batch"
        )
          return Response.json(batch);
        if (request.path.endsWith("/templates")) return Response.json([]);
      },
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("checkbox", { name: "选择当前资料" }));
    await user.click(screen.getByRole("button", { name: "预览重新登录（2）" }));
    await user.click(await screen.findByRole("button", { name: "确认授权 2 个账号" }));
    await user.click(screen.getByRole("button", { name: "开始批量授权" }));
    await user.click(await screen.findByRole("button", { name: "重新授权失败项" }));
    await screen.findByRole("region", { name: "批量授权预览" });
    expect(
      requests
        .filter((item) => item.path.endsWith("/reauthorization/preview"))
        .map((item) => item.body),
    ).toEqual([
      {
        scope: "local-export",
        items: [
          { id: profile.id, revision: 4 },
          { id: second.id, revision: 7 },
        ],
        fresh_login: true,
      },
      {
        scope: "local-export",
        items: [{ id: second.id, revision: 7 }],
        failed_batch_id: batch.id,
        fresh_login: true,
      },
    ]);
  });
});
