import { onlineManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

function setup(options: { fail?: boolean; hold?: boolean; holdReadback?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity },
      mutations: { retry: 2, retryDelay: 0 },
    },
  });
  clients.push(client);
  let patterns = ["original"];
  let failure = options.fail;
  let readFailure = false;
  let release: () => void = () => undefined;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  const writes: { blocked_patterns: string[]; expected_patterns?: string[] }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const payload = JSON.parse(String(init.body)) as {
          blocked_patterns: string[];
          expected_patterns?: string[];
        };
        writes.push(payload);
        if (options.hold) await gate;
        if (failure)
          return Response.json(
            { code: "unavailable", detail: "保存失败，请重试" },
            { status: 503 },
          );
        if (JSON.stringify(payload.expected_patterns) !== JSON.stringify(patterns))
          return Response.json(
            { code: "model_sync_settings_changed", detail: "规则已更新，请重新读取后再保存" },
            { status: 409 },
          );
        patterns = payload.blocked_patterns;
      }
      if (init?.method !== "PUT" && options.holdReadback && writes.length > 0) await gate;
      if (init?.method !== "PUT" && readFailure)
        return Response.json({ code: "unavailable", detail: "读取失败，请重试" }, { status: 503 });
      return Response.json({ blocked_patterns: patterns });
    }),
  );
  const view = () => (
    <QueryClientProvider client={client}>
      <ModelSyncSettingsCard />
    </QueryClientProvider>
  );
  const rendered = render(view());
  return {
    client,
    writes,
    release,
    failRead: (value: boolean) => {
      readFailure = value;
    },
    recover: () => {
      failure = false;
    },
    concurrent: () => {
      patterns = ["concurrent", "original"];
    },
    unmount: rendered.unmount,
    reopen: () => {
      rendered.unmount();
      return render(view());
    },
  };
}

it("编辑期间后台刷新后保存仍携带原草稿基准，冲突保留草稿", async () => {
  const state = setup();
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "draft" } });
  state.concurrent();
  await act(async () => {
    await state.client.invalidateQueries({ queryKey: ["account-model-sync-settings"] });
  });
  expect(field).toHaveValue("draft");
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.writes).toEqual([{ blocked_patterns: ["draft"], expected_patterns: ["original"] }]);
  expect(field).toHaveValue("draft");
  expect(screen.getByRole("button", { name: "保存屏蔽规则" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "加载最新规则并替换草稿" }));
  await waitFor(() => expect(field).toHaveValue("concurrent\noriginal"));
  fireEvent.change(field, { target: { value: "concurrent\noriginal\nnew" } });
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.writes).toHaveLength(2));
  expect(state.writes[1].expected_patterns).toEqual(["concurrent", "original"]);
});

it("冲突后加载最新规则失败保留草稿，恢复读取后才能重新编辑保存", async () => {
  const state = setup();
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "draft" } });
  state.concurrent();
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  const reload = await screen.findByRole("button", { name: "加载最新规则并替换草稿" });
  await waitFor(() => expect(state.client.isFetching()).toBe(0));
  state.failRead(true);
  await userEvent.click(reload);
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(field).toHaveValue("draft");
  expect(screen.getByRole("button", { name: "保存屏蔽规则" })).toBeDisabled();
  state.failRead(false);
  await userEvent.click(screen.getByRole("button", { name: "加载最新规则并替换草稿" }));
  await waitFor(() => expect(field).toHaveValue("concurrent\noriginal"));
  expect(state.writes).toHaveLength(1);
});

it("同轮重复提交只发送一次保存", async () => {
  const state = setup({ hold: true });
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "draft" } });
  const form = field.closest("form")!;
  act(() => {
    fireEvent.submit(form);
    fireEvent.submit(form);
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "保存中…" })).toBeDisabled());
  expect(state.writes).toHaveLength(1);
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
});

it("离线标记下保存立即尝试，不延迟到恢复联网后发送", async () => {
  const state = setup({ fail: true });
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "draft" } });
  onlineManager.setOnline(false);
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(field).toHaveValue("draft");
});

it("保存失败不继承自动重试，草稿可手动重试", async () => {
  const state = setup({ fail: true });
  fireEvent.change(await screen.findByRole("textbox", { name: "屏蔽规则" }), {
    target: { value: "draft" },
  });
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(state.writes).toHaveLength(1);
  state.recover();
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.writes).toHaveLength(2));
});

it("关闭后保存返回不覆盖新窗口草稿", async () => {
  const state = setup({ hold: true });
  fireEvent.change(await screen.findByRole("textbox", { name: "屏蔽规则" }), {
    target: { value: "first-draft" },
  });
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.writes).toHaveLength(1));
  state.reopen();
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "new-draft" } });
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(field).toHaveValue("new-draft");
});

it("保存成功但后续读取失败时仍显示已保存的规则", async () => {
  const state = setup();
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "saved-draft" } });
  state.failRead(true);
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.client.isMutating()).toBe(0));
  expect(field).toHaveValue("saved-draft");
});

it("保存成功后后台刷新仍在等待时恢复编辑并保留已保存规则", async () => {
  const state = setup({ holdReadback: true });
  const field = await screen.findByRole("textbox", { name: "屏蔽规则" });
  fireEvent.change(field, { target: { value: "saved-draft" } });
  await userEvent.click(screen.getByRole("button", { name: "保存屏蔽规则" }));
  await waitFor(() => expect(state.client.isFetching()).toBe(1));
  await waitFor(() => expect(field).toBeEnabled());
  expect(field).toHaveValue("saved-draft");
  fireEvent.change(field, { target: { value: "next-draft" } });
  await act(async () => state.release());
  await waitFor(() => expect(state.client.isFetching()).toBe(0));
  expect(field).toHaveValue("next-draft");
});
