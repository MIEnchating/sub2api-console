import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { TemplateList } from "../components/template-list";
import { templateKeys, workbenchKeys } from "../constants";
import type { TemplateLibrary, WorkbenchTemplate } from "../types";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const template: WorkbenchTemplate = {
  id: "template-one",
  name: "团队模板",
  source_id: "41",
  source_name: "来源账号",
  source_version: "source-v1",
  synced_at: "2026-09-17T10:00:00Z",
  revision: 1,
  config: {
    concurrency: 0,
    priority: 50,
    rate_multiplier: "0.1234567890123456789",
    load_factor: null,
    proxy_id: null,
    group_ids: ["7"],
    auto_pause_on_expired: true,
    expires_at: null,
    notes: "",
    credential_extras: { plan_type: "prolite", model_mapping: { "gpt-5": "gpt-5.6" } },
    extra: { codex_fingerprint_mode: "session" },
  },
  summary: {
    id: "41",
    name: "来源账号",
    email: "owner@example.test",
    status: "active",
    schedulable: true,
    plan: "prolite",
    groups: [{ id: "7", name: "团队" }],
    proxy_name: "",
    concurrency: "0",
    load_factor: "",
    rate_multiplier: "0.1234567890123456789",
    model_mapping: { "gpt-5": "gpt-5.6" },
    fingerprint: "session",
  },
};
function mount(
  fetcher: typeof fetch,
  library: TemplateLibrary = { revision: 1, preferred_id: template.id, items: [template] },
): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  client.setQueryData(templateKeys.library, library);
  client.setQueryData(workbenchKeys.accounts, [template.summary]);
  render(
    <QueryClientProvider client={client}>
      <TemplateList />
    </QueryClientProvider>,
  );
}
it("重新同步使用模板自身来源并保存到同一模板", async () => {
  const writes: Array<Record<string, unknown>> = [];
  const reads: string[] = [];
  mount(
    vi.fn(async (input, init) => {
      const url = String(input);
      if (init?.method === "POST") {
        writes.push(JSON.parse(String(init.body)));
        return Response.json({
          revision: 2,
          preferred_id: template.id,
          items: [{ ...template, revision: 2 }],
        });
      }
      reads.push(url);
      return Response.json({ ...template, source_version: "source-v2" });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "重新同步" }));
  await waitFor(() => expect(reads).toEqual(["/api/account-workbench/template-source/41"]));
  await user.click(await screen.findByRole("button", { name: "保存模板" }));
  await waitFor(() =>
    expect(writes).toEqual([
      {
        id: "template-one",
        name: "团队模板",
        source_id: "41",
        source_version: "source-v2",
        revision: 1,
      },
    ]),
  );
});
it("删除模板先显示确认，取消不会发请求，确认携带版本", async () => {
  const requests: Array<{ method: string | undefined; body: unknown }> = [];
  mount(
    vi.fn(async (_input, init) => {
      requests.push({ method: init?.method, body: JSON.parse(String(init?.body)) });
      return Response.json({ revision: 2, preferred_id: "", items: [] });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除模板 团队模板" }));
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "取消" }));
  expect(requests).toEqual([]);
  await user.click(screen.getByRole("button", { name: "删除模板 团队模板" }));
  await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "删除模板" }));
  await waitFor(() =>
    expect(requests).toEqual([{ method: "DELETE", body: { id: "template-one", revision: 1 } }]),
  );
  expect(await screen.findByText("还没有模板，可手动创建或从线上账号读取。")).toBeVisible();
});

it("新建模板读取成功前禁止保存，空名称提交显示字段校验", async () => {
  let complete: (response: Response) => void = () => undefined;
  mount(
    vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          complete = resolve;
        }),
    ),
    { revision: 0, preferred_id: "", items: [] },
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "从线上账号创建" }));
  const dialog = within(screen.getByRole("dialog"));
  expect(dialog.getByRole("button", { name: "保存模板" })).toBeDisabled();
  dialog.getByRole("combobox", { name: "选择模板来源账号" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: "来源账号 · #41" }));
  await user.click(dialog.getByRole("button", { name: "读取配置" }));
  expect(dialog.getByRole("status", { name: "正在读取来源配置" })).toBeVisible();
  expect(dialog.getByRole("button", { name: "保存模板" })).toBeDisabled();
  complete(Response.json(template));
  await waitFor(() => expect(dialog.getByRole("button", { name: "保存模板" })).toBeEnabled());
  expect(dialog.getByText("gpt-5 → gpt-5.6")).toBeVisible();
  expect(dialog.getByText("0.1234567890123456789")).toBeVisible();
  await user.click(dialog.getByRole("button", { name: "保存模板" }));
  expect(await dialog.findByRole("alert")).toHaveTextContent("请输入模板名称");
  expect(dialog.getByRole("textbox", { name: "模板名称" })).toHaveAttribute("aria-invalid", "true");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it("来源读取失败保留取消与重试入口，重试成功后才允许保存", async () => {
  let reads = 0;
  mount(
    vi.fn(async () => {
      reads++;
      if (reads === 1) return Response.json({ detail: "来源读取失败" }, { status: 503 });
      return Response.json(template);
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "重新同步" }));
  const dialog = within(screen.getByRole("dialog"));
  await waitFor(() => expect(dialog.getByRole("button", { name: "重新读取" })).toBeEnabled());
  expect(dialog.getByRole("button", { name: "保存模板" })).toBeDisabled();
  expect(dialog.getByRole("button", { name: "取消" })).toBeEnabled();
  await user.click(dialog.getByRole("button", { name: "重新读取" }));
  await waitFor(() => expect(dialog.getByRole("button", { name: "保存模板" })).toBeEnabled());
});
