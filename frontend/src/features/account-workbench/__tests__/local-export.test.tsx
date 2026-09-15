import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Task, WorkbenchOAuthSession, WorkbenchPreview } from "@/api";
import { WorkbenchLocalExport } from "../components/workbench-local-export";
import { workbenchKeys } from "../constants";

type RecordedRequest = { path: string; method: string; body: unknown };
let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const inputJSON =
  '{"credentials":{"access_token":"isolated-local-access","chatgpt_user_id":"user-1","chatgpt_account_id":"workspace-1"}}';
function localPreview(): WorkbenchPreview {
  return {
    id: "local-preview",
    scope: "local-export",
    export_only: true,
    target: "",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    check_after_import: false,
    model: "",
    errors: [],
    items: [
      {
        id: "0",
        index: 0,
        name: "本地账号",
        email: "local@example.test",
        plan_type: "plus",
        template_id: "",
        template_name: "",
        template_revision: 0,
        group_ids: [],
        duplicate: false,
      },
    ],
  };
}
const task: Task = {
  id: "local-task",
  skill: "account-workbench",
  operation: "account-workbench-convert",
  status: "queued",
  progress: 0,
  message: "等待生成私有账号文件",
  result: {},
  created_at: "2026-09-14T00:00:00Z",
  updated_at: "2026-09-14T00:00:00Z",
};

function mountLocal(handler: (request: RecordedRequest) => Response): RecordedRequest[] {
  const requests: RecordedRequest[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url, options) => {
      const request = {
        path: String(url),
        method: options?.method ?? "GET",
        body: options?.body ? (JSON.parse(String(options.body)) as unknown) : null,
      };
      requests.push(request);
      return handler(request);
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryDefaults(workbenchKeys.localExports, { staleTime: Infinity });
  client.setQueryData(workbenchKeys.localExports, []);
  client.setQueryData(workbenchKeys.exports, [
    {
      id: "managed-secret-file",
      kind: "accounts",
      count: 1,
      created_at: "2026-09-14T00:00:00Z",
      expires_at: "2026-09-15T00:00:00Z",
    },
  ]);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchLocalExport />
    </QueryClientProvider>,
  );
  return requests;
}

it("本地JSON转换不读取线上模板，成功后清空输入并更新本地文件列表", async () => {
  let removed = false;
  const requests = mountLocal((request) => {
    if (request.path === "/api/account-workbench/preview") return Response.json(localPreview());
    if (
      request.path === "/api/account-workbench/local-exports/local-file" &&
      request.method === "DELETE"
    ) {
      removed = true;
      return Response.json({ deleted: true });
    }
    if (request.path === "/api/account-workbench/local-exports")
      return Response.json(
        removed
          ? []
          : [
              {
                id: "local-file",
                kind: "accounts",
                count: 1,
                created_at: "2026-09-14T00:00:00Z",
                expires_at: "2026-09-15T00:00:00Z",
              },
            ],
      );
    return Response.json(task);
  });
  const user = userEvent.setup();
  const editor = await screen.findByRole("textbox", { name: "账号内容" }, { timeout: 5000 });
  await user.click(editor);
  await user.paste(inputJSON);
  expect(screen.queryByRole("combobox", { name: "配置模板" })).not.toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await screen.findByRole("table", { name: "账号预览" });
  expect(screen.getByRole("region", { name: "账号私有转换预览" })).not.toHaveTextContent(
    "模板所属目标",
  );
  expect(requests.find((request) => request.path.endsWith("/preview"))?.body).toEqual({
    content: inputJSON,
    scope: "local-export",
    export_only: true,
    check_after_import: false,
    model: "",
  });
  expect(screen.queryByRole("button", { name: "确认导入 1 个账号" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "生成私有 JSON 文件" }));
  expect(screen.getByRole("dialog", { name: "确认生成私有账号文件" })).not.toHaveTextContent(
    "模板配置",
  );
  await user.click(
    within(screen.getByRole("dialog", { name: "确认生成私有账号文件" })).getByRole("button", {
      name: "创建私有转换任务",
    }),
  );
  await waitFor(() => expect(editor).toHaveTextContent(/^账号 JSON 或 rt_ 刷新令牌$/));
  await user.click(screen.getByRole("tab", { name: "私有文件" }));
  expect(await screen.findByText("local-file")).toBeVisible();
  expect(screen.queryByText("managed-secret-file")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "删除私有文件 local-file" }));
  expect(requests.some((request) => request.method === "DELETE")).toBe(false);
  await user.click(
    within(screen.getByRole("dialog", { name: "删除私有文件" })).getByRole("button", {
      name: "删除文件",
    }),
  );
  await screen.findByText("暂无私有导出文件");
  expect(
    requests.some(
      (request) =>
        request.path.endsWith("/templates") ||
        request.path.endsWith("/import") ||
        request.path === "/api/account-workbench/exports",
    ),
  ).toBe(false);
});

it("本地官方授权的启动和结果预览保持独立范围且不展示导入设置", async () => {
  const session: WorkbenchOAuthSession = {
    id: "local-oauth",
    task_id: "local-oauth",
    scope: "local-export",
    host: "auth.openai.com",
    status: "authorized",
    message: "授权完成",
    expires_at: new Date(Date.now() + 900000).toISOString(),
    width: 1100,
    height: 760,
  };
  const requests = mountLocal((request) => {
    if (request.path === "/api/account-workbench/oauth-checkpoints?scope=local-export")
      return Response.json([]);
    if (request.path.endsWith("/preview")) return Response.json(localPreview());
    return Response.json(session);
  });
  const user = userEvent.setup();
  await user.click(screen.getByRole("tab", { name: "单个授权" }));
  await user.click(screen.getByRole("button", { name: "开始授权登录" }));
  await user.click(await screen.findByRole("button", { name: "预览授权账号" }));
  await screen.findByRole("table", { name: "账号预览" });
  expect(
    requests.find((request) => request.path === "/api/account-workbench/oauth")?.body,
  ).toMatchObject({ scope: "local-export" });
  expect(requests.find((request) => request.path.endsWith("/local-oauth/preview"))?.body).toEqual({
    scope: "local-export",
    export_only: true,
    check_after_import: false,
    model: "",
  });
  expect(screen.queryByRole("combobox", { name: "配置模板" })).not.toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: "检测模型" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "生成私有 JSON 文件" })).toBeEnabled();
  expect(
    requests.some(
      (request) => request.path.endsWith("/templates") || request.path.endsWith("/import"),
    ),
  ).toBe(false);
});
