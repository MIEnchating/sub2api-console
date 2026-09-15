import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkbenchExportArtifacts } from "../components/workbench-export-artifacts";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(options: { failed?: boolean; expired?: boolean } = {}) {
  const requests: Array<{ path: string; method: string; body: unknown }> = [];
  const expires = new Date(Date.now() + 600000).toISOString();
  const task = {
    id: "local-regeneration",
    skill: "account-workbench",
    operation: "account-workbench-regenerate",
    status: "queued",
    progress: 0,
    message: "等待刷新本地授权",
    result: {},
    created_at: expires,
    updated_at: expires,
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
      if (path.endsWith("/local-exports"))
        return Response.json([
          {
            id: "local-file",
            kind: "accounts",
            count: 1,
            created_at: expires,
            expires_at: expires,
          },
          {
            id: "profile-file",
            kind: "login-profiles",
            count: 1,
            created_at: expires,
            expires_at: expires,
          },
        ]);
      if (path.endsWith("/regenerate/preview")) {
        if (options.failed)
          return Response.json({ detail: "来源已变化，请重新选择" }, { status: 409 });
        return Response.json({
          id: "local-regeneration-preview",
          scope: "local-export",
          artifact_id: "local-file",
          target: "",
          expires_at: options.expired ? "2000-01-01T00:00:00Z" : expires,
          items: [
            {
              index: 0,
              name: "独立账号",
              email: "local@example.test",
              user_id: "local-user",
              workspace_id: "local-workspace",
              revision: "private-version",
            },
          ],
        });
      }
      if (path.endsWith("/regenerate") || path.includes("/tasks/")) return Response.json(task);
      if (method === "DELETE") return Response.json({ deleted: true });
      throw new Error(`未预期请求 ${method} ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchExportArtifacts scope="local-export" />
    </QueryClientProvider>,
  );
  return requests;
}
describe("本地私有文件再生", () => {
  it("账号文件按稳定产物ID预览并确认后刷新，不请求管理目标", async () => {
    const requests = mount();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "重新生成私有文件 local-file" }));
    await screen.findByText("范围：本地私有文件");
    expect(screen.getByRole("list", { name: "重新生成授权范围" })).toHaveTextContent(
      "local-workspace",
    );
    expect(requests.find((row) => row.path.endsWith("/regenerate/preview"))?.body).toEqual({
      scope: "local-export",
      artifact_id: "local-file",
    });
    expect(requests.some((row) => row.path.endsWith("/regenerate"))).toBe(false);
    await user.click(screen.getByRole("button", { name: "确认刷新并生成文件" }));
    await screen.findByRole("status", { name: "等待刷新本地授权" });
    expect(requests.find((row) => row.path.endsWith("/regenerate"))?.body).toEqual({
      preview_id: "local-regeneration-preview",
      confirmed: true,
    });
    expect(requests.some((row) => row.path === "/api/accounts" || row.path === "/api/config")).toBe(
      false,
    );
  });
  it("登录资料文件不显示账号RT再生操作", async () => {
    mount();
    await screen.findByText("profile-file");
    expect(
      screen.queryByRole("button", { name: "重新生成私有文件 profile-file" }),
    ).not.toBeInTheDocument();
  });
  it("私有来源已失效时只提供重读和关闭，禁止提交", async () => {
    const requests = mount({ failed: true });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "重新生成私有文件 local-file" }));
    await screen.findByRole("button", { name: "重新读取" });
    expect(screen.getByRole("button", { name: "确认刷新并生成文件" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "返回" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(requests.some((row) => row.path.endsWith("/regenerate"))).toBe(false);
  });
});
