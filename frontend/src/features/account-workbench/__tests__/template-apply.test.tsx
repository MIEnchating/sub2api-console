import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchMixedForm } from "../components/workbench-mixed-form";
import { defaultConfig, workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("模板页更新当前使用模板后，仍挂载的导入表单同步选择及摘要", async () => {
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  const templates = ["a", "b"].map((id) => ({
    id,
    name: `模板${id}`,
    preferred: id === "a",
    revision: 1,
    priority: 0,
    config: defaultConfig,
    match: { plan_type: "", email_domain: "" },
  }));
  client.setQueryData(workbenchKeys.templates, templates);
  client.setQueryData(["groups"], []);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMixedForm onSubmit={() => undefined} />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("模板a");
  await act(async () => {
    client.setQueryData(
      workbenchKeys.templates,
      templates.map((item) => ({ ...item, preferred: item.id === "b" })),
    );
  });
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("模板b"),
  );
  expect(screen.getByLabelText("已选模板摘要")).toHaveTextContent("并发 10");
});

it("开发模式重挂载使用已缓存的首选模板，不退回自动匹配", () => {
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  client.setQueryData(workbenchKeys.templates, [
    {
      id: "preferred",
      name: "首选账号配置",
      preferred: true,
      revision: 1,
      priority: 0,
      config: defaultConfig,
      match: { plan_type: "", email_domain: "" },
    },
  ]);
  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <WorkbenchMixedForm onSubmit={() => undefined} />
      </QueryClientProvider>
    </StrictMode>,
  );
  expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("首选账号配置");
});

it("导入中读取线上配置并保存使用后，本批预览采用新模板", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const previous = {
    id: "previous",
    name: "原模板",
    revision: 1,
    preferred: true,
    config: defaultConfig,
    priority: 0,
    match: { plan_type: "", email_domain: "" },
  };
  const saved = { ...previous, id: "saved", name: "新账号 配置" };
  let created = false;
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url, init) => {
      if (String(url).endsWith("/accounts"))
        return Response.json([
          { id: "42", name: "新账号", platform: "openai", account_type: "oauth" },
        ]);
      if (String(url).endsWith("/template-from-account"))
        return Response.json({
          account_id: "42",
          account_name: "新账号",
          source_revision: "source-v1",
          config: defaultConfig,
          match: previous.match,
          priority: 0,
        });
      if (String(url).endsWith("/templates")) {
        if (init?.method === "POST") {
          created = true;
          return Response.json(saved);
        }
        return Response.json(created ? [{ ...previous, preferred: false }, saved] : [previous]);
      }
      return Response.json([]);
    }),
  );
  const submit = vi.fn();
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMixedForm onSubmit={submit} />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("原模板"),
  );
  await user.type(screen.getByRole("textbox", { name: "账号内容" }), "rt_fixture");
  await user.click(screen.getByRole("button", { name: "读取线上配置" }));
  await waitFor(() => expect(screen.getByRole("combobox", { name: "来源账号" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "来源账号" }));
  await user.click(await screen.findByRole("option", { name: "新账号（ID 42）" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存并使用" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "保存并使用" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(submit).not.toHaveBeenCalled();
  expect(screen.getByRole("combobox", { name: "配置模板" })).toHaveTextContent("新账号 配置");
  await user.click(screen.getByRole("button", { name: "解析并预览" }));
  expect(submit).toHaveBeenCalledWith(
    expect.objectContaining({ template_id: "saved", content: "rt_fixture" }),
  );
});
