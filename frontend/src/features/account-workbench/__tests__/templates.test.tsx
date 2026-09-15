import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkbenchTemplate } from "@/api";
import { WorkbenchTemplateDialog } from "../components/workbench-template-dialog";
import { defaultConfig } from "../constants";

let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const template: WorkbenchTemplate = {
  id: "template-1",
  revision: 4,
  name: "团队模板",
  preferred: true,
  priority: 0,
  target_url: "https://sub2api.example.test",
  source_account_id: "42",
  source_name: "来源账号",
  source_revision: "source-old",
  source_synced_at: "2026-09-14T08:00:00Z",
  match: { plan_type: "", email_domain: "" },
  config: defaultConfig,
};

function mountEditor(props: { refreshSource?: boolean } = {}): void {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchTemplateDialog item={template} {...props} onClose={vi.fn()} />
    </QueryClientProvider>,
  );
}

describe("账号配置模板", () => {
  it("从选中账号的稳定 ID 提取配置时补齐可编辑默认值并保留非敏感扩展配置", async () => {
    const writes: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
        const path = String(input);
        if (path.endsWith("/groups")) return Response.json([{ id: "7", name: "示例分组" }]);
        if (path.endsWith("/accounts"))
          return Response.json([
            { id: "42", name: "同名账号", platform: "openai", account_type: "oauth" },
          ]);
        if (path.endsWith("/template-from-account")) {
          writes.push(JSON.parse(String(init?.body)) as unknown);
          return Response.json({
            account_id: "42",
            account_name: "同名账号",
            target: "https://sub2api.example.test",
            source_revision: "source-current",
            synced_at: "2026-09-14T09:00:00Z",
            match: { plan_type: "plus", email_domain: "" },
            priority: 10,
            config: { group_ids: ["7"], extra: { openai_ws_force_http: true } },
          });
        }
        writes.push(JSON.parse(String(init?.body)) as unknown);
        return Response.json({ id: "saved", revision: 1 });
      }),
    );
    client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={client}>
        <WorkbenchTemplateDialog onClose={vi.fn()} />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(screen.getByRole("combobox", { name: "来源账号" })).toBeEnabled());
    await user.click(screen.getByRole("combobox", { name: "来源账号" }));
    await user.click(await screen.findByRole("option", { name: "同名账号（ID 42）" }));
    await user.click(screen.getByRole("button", { name: "从账号读取配置" }));
    await screen.findByRole("region", { name: "来源配置预览" });
    expect(screen.getByRole("button", { name: "保存模板" })).toBeDisabled();
    expect(screen.getByRole("textbox", { name: "模板名称" })).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "应用来源配置" }));
    await waitFor(() =>
      expect(screen.getByRole("textbox", { name: "模板名称" })).toHaveValue("同名账号 配置"),
    );
    expect(screen.getByRole("spinbutton", { name: "并发数" })).toHaveValue(10);
    expect(screen.getByRole("textbox", { name: "套餐条件" })).toHaveValue("plus");
    expect(screen.getByRole("spinbutton", { name: "匹配优先级" })).toHaveValue(10);
    expect(screen.getByRole("checkbox", { name: "设为首选模板" })).toBeChecked();
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[0]).toEqual({ account_id: "42" });
    expect(writes[1]).toMatchObject({
      preferred: true,
      source_account_id: "42",
      source_revision: "source-current",
      match: { plan_type: "plus", email_domain: "" },
      priority: 10,
      config: { concurrency: 10, group_ids: ["7"], extra: { openai_ws_force_http: true } },
    });
  });

  it("刷新已有来源后取消预览会保留模板配置与来源版本", async () => {
    const writes: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input, init) => {
        const path = String(input);
        if (path.endsWith("/groups")) return Response.json([]);
        if (path.endsWith("/accounts"))
          return Response.json([
            { id: "42", name: "来源账号", platform: "openai", account_type: "oauth" },
          ]);
        if (path.endsWith("/template-from-account")) {
          writes.push(JSON.parse(String(init?.body)) as unknown);
          return Response.json({
            account_id: "42",
            account_name: "来源账号",
            target: template.target_url,
            source_revision: "source-new",
            synced_at: "2026-09-14T09:00:00Z",
            config: { ...defaultConfig, concurrency: 20 },
            match: { plan_type: "team", email_domain: "" },
            priority: 10,
          });
        }
        writes.push(JSON.parse(String(init?.body)) as unknown);
        return Response.json(template);
      }),
    );
    mountEditor({ refreshSource: true });
    const user = userEvent.setup();
    await screen.findByRole("region", { name: "来源配置预览" });
    expect(writes).toEqual([{ account_id: "42" }]);
    expect(screen.getByRole("combobox", { name: "来源账号" })).toHaveTextContent(
      "来源账号（ID 42）",
    );
    await user.click(screen.getByRole("button", { name: "取消来源更新" }));
    expect(screen.getByRole("spinbutton", { name: "并发数" })).toHaveValue(
      defaultConfig.concurrency,
    );
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes[1]).toMatchObject({ name: template.name, revision: 4, config: defaultConfig });
    expect(writes[1]).not.toHaveProperty("source_revision");
    expect(writes[1]).not.toHaveProperty("source_account_id");
  });

  it("应用来源更新后保存原模板ID和新来源版本", async () => {
    const writes: Array<{ path: string; body: unknown }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input, init) => {
        const path = String(input);
        if (path.endsWith("/groups") || path.endsWith("/accounts")) return Response.json([]);
        if (path.endsWith("/template-from-account"))
          return Response.json({
            account_id: "42",
            account_name: "新来源名称",
            target: template.target_url,
            source_revision: "source-new",
            synced_at: "2026-09-14T09:00:00Z",
            config: { ...defaultConfig, concurrency: 20 },
            match: { plan_type: "team", email_domain: "" },
            priority: 10,
          });
        writes.push({ path, body: JSON.parse(String(init?.body)) as unknown });
        return Response.json(template);
      }),
    );
    mountEditor({ refreshSource: true });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "应用来源配置" }));
    expect(screen.getByRole("textbox", { name: "模板名称" })).toHaveValue("团队模板");
    expect(screen.getByRole("spinbutton", { name: "并发数" })).toHaveValue(20);
    await user.click(screen.getByRole("button", { name: "保存模板" }));
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      path: "/api/account-workbench/templates/template-1",
      body: {
        revision: 4,
        source_account_id: "42",
        source_revision: "source-new",
        config: { concurrency: 20 },
      },
    });
  });

  it("来源读取失败时保留原配置并提供重试和取消入口", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn<typeof fetch>(async (input) => {
        if (String(input).endsWith("/template-from-account"))
          return Response.json({ detail: "来源账号暂时不可用" }, { status: 503 });
        return Response.json([]);
      }),
    );
    mountEditor({ refreshSource: true });
    await screen.findByRole("button", { name: "重新读取" });
    expect(screen.getByRole("spinbutton", { name: "并发数" })).toHaveValue(
      defaultConfig.concurrency,
    );
    expect(screen.getByRole("button", { name: /^取消$/ })).toBeEnabled();
    expect(screen.queryByRole("region", { name: "来源配置预览" })).not.toBeInTheDocument();
  });
});
