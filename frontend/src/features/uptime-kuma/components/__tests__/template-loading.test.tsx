import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { KumaTemplate, KumaTemplateDetail } from "@/api";
import { TemplateDialog } from "../template-dialog";

const template: KumaTemplate = {
  id: "template-1",
  revision: 1,
  name: "API 探活",
  method: "POST",
  auth_method: "none",
  headers_configured: true,
  body_configured: true,
  auth_configured: false,
};
const detail: KumaTemplateDetail = {
  ...template,
  headers: '{"X-Test":"value"}',
  body: '{"model":"test-model"}',
};
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
function renderEditor(): ReturnType<typeof render> {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TemplateDialog
        item={template}
        pending={false}
        error={null}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />
    </QueryClientProvider>,
  );
}
it("编辑模板读取中显示轻量反馈，详情返回后回填并允许保存", async () => {
  let resolve!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    ),
  );
  const view = renderEditor();
  const status = screen.getByRole("status", { name: "正在读取模板内容…" });
  expect(status).toHaveTextContent("正在读取模板内容…");
  expect(screen.getByRole("dialog").querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.getByRole("button", { name: "保存模板" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  await act(async () => resolve(Response.json(detail)));
  expect(await screen.findByLabelText("模板名称")).toHaveValue("API 探活");
  expect(screen.getByRole("button", { name: "保存模板" })).toBeEnabled();
  view.unmount();
  await waitFor(() =>
    expect(client.getQueryData(["uptime-kuma", "template-editor", template.id])).toBeUndefined(),
  );
});
it("模板读取失败保留重试入口，重试期间禁用保存直至读取成功", async () => {
  let resolve!: (value: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValueOnce(Response.json({ detail: "读取失败" }, { status: 502 }))
      .mockImplementationOnce(
        () =>
          new Promise<Response>((done) => {
            resolve = done;
          }),
      ),
  );
  renderEditor();
  fireEvent.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(screen.getByRole("button", { name: "保存模板" })).toBeDisabled();
  expect(await screen.findByRole("status", { name: "正在读取模板内容…" })).toBeVisible();
  await act(async () => resolve(Response.json(detail)));
  expect(await screen.findByLabelText("模板名称")).toHaveValue("API 探活");
});
