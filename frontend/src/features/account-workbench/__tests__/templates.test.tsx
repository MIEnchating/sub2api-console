import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { WorkbenchTemplate, WorkbenchTemplateSource } from "@/api";
import { WorkbenchTemplateDialog } from "../components/workbench-template-dialog";
import { defaultConfig } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

const template: WorkbenchTemplate = {
  id: "template-1",
  revision: 4,
  name: "团队模板",
  preferred: true,
  priority: 7,
  target_url: "https://sub2api.example.test",
  source_account_id: "42",
  source_name: "来源账号",
  source_revision: "old",
  match: { plan_type: "team", email_domain: "" },
  config: { ...defaultConfig, extra: { openai_ws_force_http: true } },
};
const source: WorkbenchTemplateSource = {
  account_id: "42",
  account_name: "来源账号",
  target: template.target_url!,
  source_revision: "current",
  synced_at: "2026-09-15T00:00:00Z",
  match: { plan_type: "plus", email_domain: "" },
  priority: 10,
  config: {
    ...defaultConfig,
    concurrency: 20,
    rate_multiplier: "0.123456789012345678901",
    group_ids: ["7"],
    extra: { openai_ws_force_http: true },
  },
};

function mount(
  item?: WorkbenchTemplate,
  fail = false,
): { writes: { path: string; body: unknown }[]; close: ReturnType<typeof vi.fn> } {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const writes: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url, init) => {
      const path = String(url);
      if (path.endsWith("/accounts"))
        return Response.json([
          { id: "42", name: "来源账号", platform: "openai", account_type: "oauth" },
        ]);
      if (path.endsWith("/groups")) return Response.json([{ id: "7", name: "生产组" }]);
      if (path.endsWith("/template-from-account"))
        return fail
          ? Response.json({ detail: "来源暂不可用" }, { status: 503 })
          : Response.json(source);
      writes.push({ path, body: JSON.parse(String(init?.body)) as unknown });
      return Response.json(template);
    }),
  );
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const close = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <WorkbenchTemplateDialog
        item={item}
        refreshSource={!!item?.source_account_id}
        onClose={close}
      />
    </QueryClientProvider>,
  );
  return { writes, close };
}

it("未选择来源时不能保存，选中账号自动读取摘要且保存保留来源版本和精确配置", async () => {
  const state = mount();
  const user = userEvent.setup();
  expect(screen.getByRole("button", { name: "保存并使用" })).toBeDisabled();
  await waitFor(() => expect(screen.getByRole("combobox", { name: "来源账号" })).toBeEnabled());
  await user.click(screen.getByRole("combobox", { name: "来源账号" }));
  await user.click(await screen.findByRole("option", { name: "来源账号（ID 42）" }));
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "模板名称" })).toHaveValue("来源账号 配置"),
  );
  expect(screen.queryByRole("spinbutton")).not.toBeInTheDocument();
  expect(screen.queryByRole("textbox", { name: /JSON|倍率|套餐条件/ })).not.toBeInTheDocument();
  expect(await screen.findByText("生产组")).toBeVisible();
  expect(screen.getByText(source.config.rate_multiplier)).toBeVisible();
  await user.click(screen.getByRole("button", { name: "保存并使用" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  expect(state.writes[0].body).toMatchObject({
    preferred: true,
    source_account_id: "42",
    source_revision: "current",
    config: source.config,
    match: source.match,
    priority: 10,
  });
});

it("刷新来源后保留模板名称，提交原模板版本和新的来源版本", async () => {
  const state = mount(template);
  await waitFor(() => expect(screen.getByRole("button", { name: "保存并使用" })).toBeEnabled());
  expect(screen.getByRole("textbox", { name: "模板名称" })).toHaveValue("团队模板");
  await userEvent.setup().click(screen.getByRole("button", { name: "保存并使用" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  expect(state.writes[0]).toMatchObject({
    path: "/api/account-workbench/templates/template-1",
    body: { revision: 4, source_revision: "current", config: source.config },
  });
});

it("来源读取失败时不能保存旧摘要，保留重试和取消入口", async () => {
  const state = mount(template, true);
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "保存并使用" })).toBeDisabled();
  await userEvent.setup().click(screen.getByRole("button", { name: "取消" }));
  expect(state.close).toHaveBeenCalledOnce();
  expect(state.writes).toHaveLength(0);
});

it("旧手工模板仅修改名称时完整保留原配置与匹配规则", async () => {
  const legacy = { ...template, source_account_id: undefined, source_revision: undefined };
  const state = mount(legacy);
  const user = userEvent.setup();
  await user.clear(screen.getByRole("textbox", { name: "模板名称" }));
  await user.type(screen.getByRole("textbox", { name: "模板名称" }), "旧配置改名");
  await user.click(screen.getByRole("button", { name: "保存名称" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  expect(state.writes[0].body).toMatchObject({
    name: "旧配置改名",
    revision: 4,
    config: legacy.config,
    match: legacy.match,
    priority: 7,
  });
  expect(state.writes[0].body).not.toHaveProperty("source_account_id");
});
