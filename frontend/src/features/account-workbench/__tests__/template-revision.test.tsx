import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toaster, toast } from "sonner";
import { afterEach, expect, it, vi } from "vitest";
import { TemplateList } from "../components/template-list";
import { templateKeys, workbenchKeys } from "../constants";
import type { TemplateLibrary } from "../types";
import { template } from "./template-fixture";

const clients: QueryClient[] = [];
const conflictMessage = "工作台资料已变化，请刷新后重试";
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.unstubAllGlobals();
});

function mount(): {
  client: QueryClient;
  writes: Array<Record<string, unknown>>;
  latest: TemplateLibrary;
} {
  const writes: Array<Record<string, unknown>> = [];
  const latest: TemplateLibrary = {
    revision: 4,
    preferred_id: template.id,
    items: [{ ...template, name: "远端更新模板", revision: 2 }],
  };
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST" || init?.method === "DELETE") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>;
        writes.push(body);
        if (body.revision !== 4) return Response.json({ detail: conflictMessage }, { status: 409 });
        return Response.json({ ...latest, revision: 5 });
      }
      if (String(input).includes("/template-source/"))
        return Response.json({ ...template, source_version: "source-v2" });
      throw new Error(`未配置测试请求：${String(input)}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  clients.push(client);
  client.setQueryData(templateKeys.library, {
    revision: 3,
    preferred_id: template.id,
    items: [template],
  });
  client.setQueryData(workbenchKeys.accounts, [template.summary]);
  render(
    <QueryClientProvider client={client}>
      <TemplateList />
      <Toaster />
    </QueryClientProvider>,
  );
  return { client, writes, latest };
}

it.each([
  { action: "编辑配置", source: false, create: false },
  { action: "创建模板", source: false, create: true },
  { action: "重新同步", source: true, create: false },
  { action: "从线上账号创建", source: true, create: true },
])("$action 期间列表刷新仍提交打开时版本，冲突保留草稿", async (scenario) => {
  const state = mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: scenario.action }));
  const dialog = within(screen.getByRole("dialog"));
  if (scenario.source && scenario.create) {
    dialog.getByRole("combobox", { name: "选择模板来源账号" }).focus();
    await user.keyboard("{ArrowDown}");
    await user.click(await screen.findByRole("option", { name: "来源账号 · #41" }));
    await user.click(dialog.getByRole("button", { name: "读取配置" }));
  }
  await waitFor(() => expect(dialog.getByRole("button", { name: "保存模板" })).toBeEnabled());
  await user.clear(dialog.getByRole("textbox", { name: "模板名称" }));
  await user.type(dialog.getByRole("textbox", { name: "模板名称" }), "未提交草稿");
  await act(async () => {
    state.client.setQueryData(templateKeys.library, state.latest);
  });
  await waitFor(() => expect(screen.getByText("远端更新模板")).toBeInTheDocument());
  await user.click(dialog.getByRole("button", { name: "保存模板" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  expect(state.writes[0]).toMatchObject({ revision: 3, name: "未提交草稿" });
  if (scenario.create) expect(state.writes[0]).not.toHaveProperty("id");
  else expect(state.writes[0]).toHaveProperty("id", template.id);
  if (scenario.source)
    expect(state.writes[0]).toMatchObject({ source_id: "41", source_version: "source-v2" });
  else expect(state.writes[0]).toHaveProperty("config");
  expect(await screen.findByText(conflictMessage)).toBeVisible();
  expect(dialog.getByRole("textbox", { name: "模板名称" })).toHaveValue("未提交草稿");
  expect(dialog.getByRole("button", { name: "取消" })).toBeEnabled();
  expect(screen.getAllByText(conflictMessage)).toHaveLength(1);

  // A retry of this draft must not silently upgrade its optimistic-lock version.
  await user.click(dialog.getByRole("button", { name: "保存模板" }));
  await waitFor(() => expect(state.writes).toHaveLength(2));
  expect(state.writes[1]).toEqual(state.writes[0]);
  await waitFor(() => expect(dialog.getByRole("button", { name: "取消" })).toBeEnabled());
  await user.click(dialog.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "编辑配置" }));
  const reopened = within(screen.getByRole("dialog"));
  expect(reopened.getByRole("textbox", { name: "模板名称" })).toHaveValue("远端更新模板");
  await user.click(reopened.getByRole("button", { name: "保存模板" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(state.writes[2]).toMatchObject({ revision: 4, id: template.id, name: "远端更新模板" });
});

it("删除确认期间模板更新仍提交原版本，重新确认才采用新版本", async () => {
  const state = mount();
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "删除模板 团队模板" }));
  await act(async () => {
    state.client.setQueryData(templateKeys.library, state.latest);
  });
  await waitFor(() => expect(screen.getByText("远端更新模板")).toBeInTheDocument());
  const dialog = within(screen.getByRole("dialog"));
  expect(dialog.getByText(/确定删除“团队模板”/)).toBeVisible();
  await user.click(dialog.getByRole("button", { name: "删除模板" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  expect(state.writes[0]).toEqual({ id: template.id, revision: 3 });
  expect(await screen.findByText(conflictMessage)).toBeVisible();
  await user.click(dialog.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "删除模板 远端更新模板" }));
  const reopened = within(screen.getByRole("dialog"));
  expect(reopened.getByText(/确定删除“远端更新模板”/)).toBeVisible();
  await user.click(reopened.getByRole("button", { name: "删除模板" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(state.writes[1]).toEqual({ id: template.id, revision: 4 });
});
