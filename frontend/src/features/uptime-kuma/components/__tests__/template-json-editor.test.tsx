import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { KumaTemplateDetail } from "@/api";
import { TemplateEditorForm } from "../template-editor-form";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
});

function renderTemplate() {
  const item: KumaTemplateDetail = {
    id: "template-json",
    revision: 1,
    name: "JSON 探活",
    method: "POST",
    auth_method: "none",
    headers_configured: true,
    body_configured: true,
    auth_configured: false,
    headers: '{"X-Client":"original"}',
    body: '{"model":"original-model","messages":[{"role":"user","content":"ping"}]}',
    body_encoding: "json",
    request_profile: "openai-chat",
    model: "original-model",
  };
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const submit = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <TemplateEditorForm item={item} pending={false} error={null} onSubmit={submit} />
      <button type="submit" form="kuma-template">
        保存模板
      </button>
    </QueryClientProvider>,
  );
  return submit;
}

it("编辑 JSON 请求体同步模型字段并提交新的请求头与请求体", async () => {
  const user = userEvent.setup();
  const submit = renderTemplate();
  await user.click(screen.getByRole("button", { name: "查看请求体" }));
  const body = await screen.findByRole("textbox", { name: "请求体" });
  await user.click(body);
  await user.keyboard("{Control>}a{/Control}");
  await user.paste('{"model":"new-model","messages":[{"role":"user","content":"check"}]}');
  expect(screen.getByLabelText("请求模型")).toHaveValue("new-model");
  const headers = screen.getByRole("textbox", { name: "请求头（JSON）" });
  await user.click(headers);
  await user.keyboard("{Control>}a{/Control}");
  await user.paste('{"X-Client":"replacement"}');
  await user.click(screen.getByRole("button", { name: "保存模板" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        model: "new-model",
        headers: '{"X-Client":"replacement"}',
        body: '{"model":"new-model","messages":[{"role":"user","content":"check"}]}',
      }),
    ),
  );
});

it("切换为 XML 编码保留请求体并允许纯文本提交", async () => {
  const user = userEvent.setup();
  const submit = renderTemplate();
  await user.click(screen.getByRole("combobox", { name: "请求体编码" }));
  await user.click(screen.getByRole("option", { name: "XML" }));
  await user.click(screen.getByRole("button", { name: "查看请求体" }));
  const body = screen.getByRole("textbox", { name: "请求体" });
  expect(body).toHaveValue(
    '{"model":"original-model","messages":[{"role":"user","content":"ping"}]}',
  );
  await user.clear(body);
  await user.paste("<ping>check</ping>");
  await user.click(screen.getByRole("button", { name: "保存模板" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ body_encoding: "xml", body: "<ping>check</ping>" }),
    ),
  );
});
