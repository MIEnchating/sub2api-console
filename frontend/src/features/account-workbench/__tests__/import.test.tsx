import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ImportPanel } from "../components/import-panel";
import { importModelKey, templateKeys } from "../constants";
import type { WorkbenchPreview } from "../types";
import { importPreviewFixture } from "./import-preview-fixture";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const preview: WorkbenchPreview = {
  id: "preview-one",
  revision: 1,
  expires_at: "2099-01-01T00:00:00Z",
  action: "import",
  check: true,
  promote: true,
  template: null,
  items: [
    {
      id: "row-one",
      index: 0,
      kind: "refresh_token",
      name: "",
      email: "",
      plan: "",
      identity_source: "pending_refresh",
    },
  ],
  errors: [],
  duplicate_count: 0,
};
function mount(fetcher: typeof fetch, onStarted = vi.fn()): void {
  vi.stubGlobal("fetch", fetcher);
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  client.setQueryData(importModelKey, { model: "gpt-6-astra", revision: 0 });
  client.setQueryData(templateKeys.library, { revision: 0, preferred_id: "", items: [] });
  render(
    <QueryClientProvider client={client}>
      <ImportPanel onStarted={onStarted} />
    </QueryClientProvider>,
  );
}
it("导入设置直接展示糖果题模型且移除输出方式和手动代理", () => {
  mount(vi.fn());
  expect(screen.queryByRole("combobox", { name: "处理方式" })).not.toBeInTheDocument();
  expect(screen.queryByText("独立 JSON 输出")).not.toBeInTheDocument();
  expect(screen.queryByRole("checkbox", { name: "使用登录 / 检测代理" })).not.toBeInTheDocument();
  expect(screen.queryByText("检测设置", { exact: true })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("gpt-6-astra");
  expect(screen.getByRole("checkbox", { name: "导入后执行糖果题检测" })).toBeChecked();
  expect(screen.queryByText(/固定出口或粘性会话/)).not.toBeInTheDocument();
});
it("账号资料为空时完整显示参考项目的凭据、RT 和 JSON 示例及空行", () => {
  mount(vi.fn());
  expect(screen.getByRole("textbox", { name: "账号资料" })).toHaveAttribute(
    "placeholder",
    '邮箱----密码----2FA 密钥\n\nrt_…\n\n{ "accounts": […] }',
  );
});
it("关闭糖果题检测禁用模型，重新开启保留已输入模型", async () => {
  mount(vi.fn());
  const user = userEvent.setup();
  const model = screen.getByRole("textbox", { name: "检测模型" });
  await user.clear(model);
  await user.type(model, "custom-model");
  const check = screen.getByRole("checkbox", { name: "导入后执行糖果题检测" });
  await user.click(check);
  expect(model).toBeDisabled();
  expect(check).not.toBeChecked();
  await user.click(check);
  expect(model).toBeEnabled();
  expect(model).toHaveValue("custom-model");
});
it("粘贴带换行的 RT 自动解析，确认时只提交预览 ID 并清空输入", async () => {
  const requests: Array<{ path: string; body: unknown }> = [];
  const onStarted = vi.fn();
  mount(
    vi.fn(async (input, init) => {
      requests.push({ path: String(input), body: JSON.parse(String(init?.body)) });
      if (String(input).endsWith("/preview")) return Response.json(preview);
      return Response.json({ id: "run-one" });
    }),
    onStarted,
  );
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_private-input\n");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByText("刷新令牌")).toBeVisible();
  expect(requests).toHaveLength(1);
  await user.click(dialog.getByRole("button", { name: "确认并开始" }));
  await waitFor(() => expect(onStarted).toHaveBeenCalledOnce());
  expect(requests[1]).toEqual({
    path: "/api/account-workbench/runs",
    body: { id: "preview-one", revision: 1 },
  });
  expect(screen.getByRole("textbox", { name: "账号资料" })).toHaveValue("");
});
it("预览含无效项时显示行号且禁止启动任务", async () => {
  mount(
    vi.fn(async () =>
      Response.json({ ...preview, errors: [{ index: 1, message: "无法识别该行" }] }),
    ),
  );
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_input\ninvalid");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByText("第 2 项：无法识别该行")).toBeVisible();
  expect(dialog.getByRole("button", { name: "确认并开始" })).toBeDisabled();
  await user.click(dialog.getByRole("button", { name: "返回修改" }));
  expect(screen.getByRole("textbox", { name: "账号资料" })).toHaveValue("rt_input\ninvalid");
});

it("500 项预览翻页后确认仍只提交整批预览 ID 和版本", async () => {
  const requests: Array<{ path: string; body: unknown }> = [];
  const onStarted = vi.fn();
  mount(
    vi.fn(async (input, init) => {
      requests.push({ path: String(input), body: JSON.parse(String(init?.body)) });
      if (String(input).endsWith("/preview")) return Response.json(importPreviewFixture(500));
      return Response.json({ id: "run-batch" });
    }),
    onStarted,
  );
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_batch-fixture");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  const dialog = within(await screen.findByRole("dialog"));
  await user.click(dialog.getByRole("button", { name: "转到下一页" }));
  expect(dialog.getByText("account-21@example.test")).toBeVisible();
  await user.click(dialog.getByRole("button", { name: "确认并开始" }));
  await waitFor(() => expect(onStarted).toHaveBeenCalledOnce());
  expect(requests[1]).toEqual({
    path: "/api/account-workbench/runs",
    body: { id: "preview-one", revision: 1 },
  });
});
it("导入预览固定提交导入方式且不携带手动代理凭据", async () => {
  let body: Record<string, unknown> | undefined;
  mount(
    vi.fn(async (_input, init) => {
      body = JSON.parse(String(init?.body));
      return Response.json(preview);
    }),
  );
  const user = userEvent.setup();
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_input");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await screen.findByRole("dialog");
  expect(body?.action).toBe("import");
  expect(body?.proxy_enabled).toBe(false);
  expect(body?.proxy_url).toBe("");
});

it("修改检测模型后预览请求严格使用输入值且说明检测未通过时保留账号且不开启调度", async () => {
  let body: Record<string, unknown> | undefined;
  mount(
    vi.fn(async (_input, init) => {
      body = JSON.parse(String(init?.body));
      return Response.json(preview);
    }),
  );
  const user = userEvent.setup();
  expect(screen.getByRole("checkbox", { name: "导入后执行糖果题检测" })).toBeChecked();
  expect(screen.queryByText(/检测未通过.*不开启调度/)).not.toBeInTheDocument();

  const model = screen.getByRole("textbox", { name: "检测模型" });
  await user.clear(model);
  await user.type(model, "gpt-5.6-luna");
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_input");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  await screen.findByRole("dialog");
  expect(body?.model).toBe("gpt-5.6-luna");
  expect(screen.queryByRole("checkbox", { name: "导入完成后启用账号" })).not.toBeInTheDocument();
});

it("关闭糖果题检测后预览说明直接开启调度并提交对应设置", async () => {
  let body: Record<string, unknown> | undefined;
  mount(
    vi.fn(async (_input, init) => {
      body = JSON.parse(String(init?.body));
      return Response.json({ ...preview, check: false });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("checkbox", { name: "导入后执行糖果题检测" }));
  await user.type(screen.getByRole("textbox", { name: "账号资料" }), "rt_input");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByText(/不执行糖果题检测，导入后直接开启调度/)).toBeVisible();
  expect(body).toMatchObject({ check: false, promote: true });
});
