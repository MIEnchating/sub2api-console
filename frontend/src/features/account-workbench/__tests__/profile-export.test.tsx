import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import type { Task, WorkbenchLoginProfile, WorkbenchProfileExportPreview } from "@/api";
import { WorkbenchProfileExport } from "../components/workbench-profile-export";
import { WorkbenchExportArtifacts } from "../components/workbench-export-artifacts";

const profile: WorkbenchLoginProfile = {
  id: "profile-a",
  account_id: "42",
  user_id: "user-a",
  workspace_id: "workspace-a",
  email: "operator@example.test",
  revision: 3,
  has_password: true,
  has_totp: true,
  has_proxy: true,
  updated_at: "2026-09-14T00:00:00Z",
};
function preview(): WorkbenchProfileExportPreview {
  return {
    id: "profile-export-preview",
    kind: "login-profiles",
    target: "https://sub2api.example.test",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    items: [profile],
  };
}
const task: Task = {
  id: "profile-export-task",
  skill: "account-workbench",
  operation: "account-workbench-profile-export",
  status: "queued",
  progress: 0,
  message: "等待生成登录资料文件",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
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
      if (request.method === "DELETE") return Response.json({ deleted: true });
      if (request.path.endsWith("/profiles/preview")) return Response.json(preview());
      return Response.json(task);
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{view}</QueryClientProvider>);
  return requests;
}

describe("登录资料私有导出", () => {
  it("仅按资料ID和版本预览，确认后只提交预览ID并返回任务", async () => {
    const created = vi.fn();
    const requests = mount(<WorkbenchProfileExport profiles={[profile]} onCreated={created} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    const scope = await screen.findByRole("list", { name: "资料导出范围" });
    expect(scope).toHaveTextContent("账号 ID：42");
    expect(scope).toHaveTextContent("版本：3");
    expect(requests).toEqual([
      {
        path: "/api/account-workbench/exports/profiles/preview",
        method: "POST",
        body: { items: [{ id: "profile-a", revision: 3 }] },
      },
    ]);
    expect(screen.getByRole("dialog", { name: "确认导出登录资料" })).toHaveTextContent(
      "密码、TOTP、收码和代理配置",
    );
    await user.click(screen.getByRole("button", { name: "确认生成资料文件" }));
    await waitFor(() => expect(created).toHaveBeenCalledWith(task));
    expect(requests.find((request) => request.path.endsWith("/exports/profiles"))?.body).toEqual({
      preview_id: "profile-export-preview",
      confirmed: true,
    });
  });

  it("取消读取仍保留返回入口并删除迟到预览", async () => {
    let resolve: (response: Response) => void = () => undefined;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const requests = mount(
      <WorkbenchProfileExport profiles={[profile]} onCreated={() => undefined} />,
      async (request) => (request.path.endsWith("/profiles/preview") ? pending : undefined),
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    expect(screen.getByRole("button", { name: "确认生成资料文件" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "返回" }));
    await act(async () => {
      resolve(Response.json(preview()));
    });
    await waitFor(() =>
      expect(
        requests.some(
          (request) =>
            request.method === "DELETE" &&
            request.path.endsWith("/exports/preview/profile-export-preview"),
        ),
      ).toBe(true),
    );
    expect(screen.queryByRole("dialog", { name: "确认导出登录资料" })).not.toBeInTheDocument();
  });

  it("预览失败可重新读取但不会创建导出任务", async () => {
    let fail = true;
    const requests = mount(
      <WorkbenchProfileExport profiles={[profile]} onCreated={() => undefined} />,
      async (request) =>
        request.path.endsWith("/profiles/preview") && fail
          ? Response.json({ detail: "资料读取失败" }, { status: 503 })
          : undefined,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    const retry = await screen.findByRole("button", { name: "重新读取" });
    expect(screen.getByRole("button", { name: "确认生成资料文件" })).toBeDisabled();
    fail = false;
    await user.click(retry);
    await screen.findByRole("list", { name: "资料导出范围" });
    expect(requests.some((request) => request.path.endsWith("/exports/profiles"))).toBe(false);
  });

  it("生成失败丢弃消费过的预览，重新打开再读取", async () => {
    const created = vi.fn();
    const requests = mount(
      <WorkbenchProfileExport profiles={[profile]} onCreated={created} />,
      async (request) =>
        request.path.endsWith("/exports/profiles")
          ? Response.json({ detail: "资料版本已变化" }, { status: 409 })
          : undefined,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    await screen.findByRole("list", { name: "资料导出范围" });
    await user.click(screen.getByRole("button", { name: "确认生成资料文件" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "确认导出登录资料" })).not.toBeInTheDocument(),
    );
    expect(created).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    await screen.findByRole("list", { name: "资料导出范围" });
    expect(requests.filter((request) => request.path.endsWith("/profiles/preview"))).toHaveLength(
      2,
    );
  });

  it("过期预览禁用生成但保留返回入口", async () => {
    mount(
      <WorkbenchProfileExport profiles={[profile]} onCreated={() => undefined} />,
      async (request) =>
        request.path.endsWith("/profiles/preview")
          ? Response.json({ ...preview(), expires_at: "2000-01-01T00:00:00Z" })
          : undefined,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "私有导出资料（1）" }));
    await screen.findByText("资料预览已过期，请关闭后重新预览。");
    expect(screen.getByRole("button", { name: "确认生成资料文件" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "返回" })).toBeEnabled();
  });

  it("空选择或外部禁用时不可创建预览", () => {
    mount(<WorkbenchProfileExport profiles={[]} onCreated={() => undefined} />);
    expect(screen.getByRole("button", { name: "私有导出资料（0）" })).toBeDisabled();
  });

  it("私有文件按账号授权和登录资料区分，删除只提交稳定产物ID", async () => {
    const requests = mount(<WorkbenchExportArtifacts />, async (request) =>
      request.method === "GET"
        ? Response.json([
            {
              id: "accounts-file",
              kind: "accounts",
              count: 2,
              created_at: "2026-09-14T00:00:00Z",
              expires_at: "2026-09-15T00:00:00Z",
            },
            {
              id: "profiles-file",
              kind: "login-profiles",
              count: 1,
              created_at: "2026-09-14T00:00:00Z",
              expires_at: "2026-09-15T00:00:00Z",
            },
          ])
        : undefined,
    );
    const user = userEvent.setup();
    const list = await screen.findByRole("list", { name: "私有文件列表" });
    expect(list).toHaveTextContent("账号授权 2 份");
    expect(list).toHaveTextContent("登录资料 1 份");
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "删除私有文件 profiles-file" }));
    expect(requests.some((request) => request.method === "DELETE")).toBe(false);
    await user.click(
      within(screen.getByRole("dialog", { name: "删除私有文件" })).getByRole("button", {
        name: "删除文件",
      }),
    );
    await waitFor(() =>
      expect(
        requests.some(
          (request) =>
            request.method === "DELETE" && request.path.endsWith("/exports/profiles-file"),
        ),
      ).toBe(true),
    );
  });
});
