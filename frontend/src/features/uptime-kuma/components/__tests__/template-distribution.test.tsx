import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { KumaTemplate, KumaTemplateDistributionPreview } from "@/api";
import { TemplateDistributionDialog } from "../template-distribution-dialog";
import { KumaTemplatesPage } from "../templates-page";
import { task } from "./fixtures";

const template: KumaTemplate = {
  id: "a".repeat(48),
  revision: 2,
  name: "接口健康模板",
  method: "POST",
  auth_method: "none",
  headers_configured: true,
  body_configured: true,
  auth_configured: false,
};
const preview: KumaTemplateDistributionPreview = {
  template_id: template.id,
  revision: 2,
  config_revision: 1,
  scope: "b".repeat(64),
  items: [
    { monitor_id: 11, name: "健康项甲", template_revision: 1, reason: "" },
    { monitor_id: 12, name: "健康项乙", template_revision: 1, reason: "类型不同，跳过" },
  ],
};
let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function renderDialog(onClose = vi.fn()): ReturnType<typeof render> {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TemplateDistributionDialog item={template} onClose={onClose} />
    </QueryClientProvider>,
  );
}

it("读取影响范围期间禁用确认并保留键盘关闭入口", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const close = vi.fn();
  renderDialog(close);
  expect(screen.getByRole("status", { name: "正在读取参数下发影响范围…" })).toBeVisible();
  expect(screen.getByRole("button", { name: "确认下发" })).toBeDisabled();
  const cancel = screen.getByRole("button", { name: "取消" });
  cancel.focus();
  await userEvent.keyboard("{Enter}");
  expect(close).toHaveBeenCalledOnce();
  await act(async () => resolve(Response.json(preview)));
  expect(await screen.findByRole("list", { name: "参数下发影响范围" })).toHaveTextContent(
    "健康项甲",
  );
  expect(screen.getByRole("button", { name: "确认下发" })).toBeEnabled();
});

it.each([
  { items: [] },
  { items: [{ monitor_id: 12, name: "不可下发", template_revision: 1, reason: "类型不同" }] },
])("没有可下发监控项时禁止提交：$items", async (fixture) => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(Response.json({ ...preview, items: fixture.items })),
  );
  renderDialog();
  await screen.findByRole("list", { name: "参数下发影响范围" });
  expect(screen.getByRole("button", { name: "确认下发" })).toBeDisabled();
  if (!fixture.items.length) expect(screen.getByText("暂无使用此模板的监控项。")).toBeVisible();
});

it("影响范围读取失败后可重试，成功前不允许下发", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "读取失败" }, { status: 502 }))
      .mockResolvedValueOnce(Response.json(preview)),
  );
  renderDialog();
  await userEvent.click(await screen.findByRole("button", { name: "重新读取" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "确认下发" })).toBeEnabled());
});

it("确认影响范围后仅提交一次并展示逐项结果，完成后刷新监控缓存", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === "POST")
      return Response.json({ ...task, status: "queued", result: { phase: "queued" } });
    if (path.includes("/api/tasks/"))
      return Response.json({
        ...task,
        result: {
          items: [
            { monitor_id: 11, name: "健康项甲", status: "succeeded", message: "参数已下发" },
            { monitor_id: 12, name: "健康项乙", status: "skipped", message: "类型不同" },
          ],
        },
      });
    return Response.json(preview);
  });
  vi.stubGlobal("fetch", fetch);
  renderDialog();
  client.setQueryData(["uptime-kuma", "monitors"], { monitors: [] });
  await screen.findByRole("list", { name: "参数下发影响范围" });
  expect(fetch.mock.calls.filter((call) => call[1]?.method === "POST")).toHaveLength(0);
  await userEvent.click(screen.getByRole("button", { name: "确认下发" }));
  const results = await screen.findByRole("list", { name: "参数下发结果" });
  expect(results).toHaveTextContent("下发成功");
  expect(results).toHaveTextContent("已跳过");
  const writes = fetch.mock.calls.filter((call) => call[1]?.method === "POST");
  expect(writes).toHaveLength(1);
  expect(JSON.parse(String(writes[0][1]?.body))).toEqual({
    revision: 2,
    scope: preview.scope,
    monitor_ids: [11],
  });
  expect(client.getQueryState(["uptime-kuma", "monitors"])?.isInvalidated).toBe(true);
  expect(screen.queryByRole("button", { name: "确认下发" })).not.toBeInTheDocument();
});

it("任务部分失败时保留已完成和未执行结果并禁止直接重复提交", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") return Response.json({ ...task, status: "queued" });
      if (String(input).includes("/api/tasks/"))
        return Response.json({
          ...task,
          status: "failed",
          message: "下发停止，请重新预览",
          result: {
            items: [
              { monitor_id: 11, name: "健康项甲", status: "succeeded", message: "参数已下发" },
              { monitor_id: 12, name: "健康项乙", status: "not_run", message: "尚未下发" },
            ],
          },
        });
      return Response.json(preview);
    }),
  );
  renderDialog();
  await screen.findByRole("list", { name: "参数下发影响范围" });
  await userEvent.click(screen.getByRole("button", { name: "确认下发" }));
  const results = await screen.findByRole("list", { name: "参数下发结果" });
  expect(results).toHaveTextContent("下发成功");
  expect(results).toHaveTextContent("未执行");
  expect(screen.queryByRole("button", { name: "确认下发" })).not.toBeInTheDocument();
});

it("模板列表的参数下发入口打开当前模板影响范围", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) =>
      Response.json(String(input).includes("/distribution") ? preview : [template]),
    ),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <KumaTemplatesPage />
    </QueryClientProvider>,
  );
  const actions = await screen.findByRole("group", { name: "接口健康模板 的操作" });
  await userEvent.click(within(actions).getByRole("button", { name: "参数下发" }));
  expect(await screen.findByRole("dialog")).toHaveTextContent("第 2 版参数");
  expect(await screen.findByRole("list", { name: "参数下发影响范围" })).toHaveTextContent(
    "健康项甲",
  );
});

it("默认全选可下发项，取消单项后只提交剩余选择", async () => {
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") return Response.json({ ...task, status: "queued" });
    if (String(input).includes("/api/tasks/")) return Response.json(task);
    return Response.json({
      ...preview,
      items: [
        ...preview.items,
        { monitor_id: 13, name: "健康项丙", template_revision: 1, reason: "" },
      ],
    });
  });
  vi.stubGlobal("fetch", fetch);
  renderDialog();
  const first = await screen.findByRole("checkbox", { name: "选择 健康项甲（#11）" });
  expect(first).toBeChecked();
  expect(screen.getByRole("checkbox", { name: "选择 健康项丙（#13）" })).toBeChecked();
  const unavailable = screen.getByRole("checkbox", { name: "选择 健康项乙（#12）" });
  expect(unavailable).not.toBeChecked();
  expect(unavailable).toHaveAttribute("aria-disabled", "true");
  first.focus();
  await userEvent.keyboard(" ");
  expect(first).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: "全选可下发项" })).toBePartiallyChecked();
  await userEvent.click(screen.getByRole("button", { name: "确认下发" }));
  await waitFor(() =>
    expect(fetch.mock.calls.some((call) => call[1]?.method === "POST")).toBe(true),
  );
  const write = fetch.mock.calls.find((call) => call[1]?.method === "POST");
  expect(JSON.parse(String(write?.[1]?.body))).toEqual({
    revision: 2,
    scope: preview.scope,
    monitor_ids: [13],
  });
});

it("取消全选后禁止下发，再次全选恢复可下发项", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(preview)),
  );
  renderDialog();
  const all = await screen.findByRole("checkbox", { name: "全选可下发项" });
  expect(all).toBeChecked();
  await userEvent.click(all);
  expect(screen.getByRole("button", { name: "确认下发" })).toBeDisabled();
  expect(screen.getByText(/已选 0 项/)).toBeVisible();
  await userEvent.click(all);
  expect(screen.getByRole("checkbox", { name: "选择 健康项甲（#11）" })).toBeChecked();
  expect(screen.getByRole("button", { name: "确认下发" })).toBeEnabled();
});
