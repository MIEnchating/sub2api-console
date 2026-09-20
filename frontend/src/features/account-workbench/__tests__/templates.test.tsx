import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { TemplateList } from "../components/template-list";
import { templateKeys, workbenchKeys } from "../constants";
import type { TemplateLibrary } from "../types";
import { template } from "./template-fixture";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

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
  expect(dialog.getByRole("row", { name: "gpt-5 gpt-5.6" })).toBeVisible();
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

it("来源模板可直接编辑配置，保存沿用模板 ID 和版本且不读取线上账号", async () => {
  const requests: Array<{ url: string; body: Record<string, unknown> }> = [];
  mount(
    vi.fn(async (url, init) => {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      requests.push({ url: String(url), body });
      return Response.json({
        revision: 2,
        preferred_id: template.id,
        items: [{ ...template, name: body.name, config: body.config }],
      });
    }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "编辑配置" }));
  const dialog = within(screen.getByRole("dialog", { name: "编辑配置模板" }));
  expect(dialog.getByRole("textbox", { name: "模板名称" })).toHaveValue(template.name);
  expect(dialog.getByRole("textbox", { name: "计费倍率" })).toHaveValue(
    template.config.rate_multiplier,
  );
  await user.clear(dialog.getByRole("textbox", { name: "模板名称" }));
  await user.type(dialog.getByRole("textbox", { name: "模板名称" }), "调整后模板");
  await user.clear(dialog.getByRole("spinbutton", { name: "并发数" }));
  await user.type(dialog.getByRole("spinbutton", { name: "并发数" }), "12");
  await user.click(dialog.getByRole("button", { name: "保存模板" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({
    url: "/api/account-workbench/templates",
    body: {
      id: template.id,
      revision: 1,
      name: "调整后模板",
      config: { ...template.config, concurrency: 12 },
    },
  });
  expect(requests[0].body).not.toHaveProperty("source_id");
  expect(screen.getByRole("article", { name: "调整后模板" })).toBeVisible();
});
