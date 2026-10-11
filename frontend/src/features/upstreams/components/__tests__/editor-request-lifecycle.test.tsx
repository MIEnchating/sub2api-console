import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, type UpstreamAllocationSetting, type UpstreamConfiguration } from "@/api";
import { seedDictionaries } from "@/test/dictionary-fixtures";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

const configuration: UpstreamConfiguration = {
  upstream_id: "up_first",
  host: "first.example.test",
  name: "第一上游",
  base_url: "https://first.example.test",
  account_base_url: "https://first.example.test/v1",
  upstream_type: "sub2api",
  auth_mode: "sub2api_user_token",
  recharge_rate: "1",
  raw_balance: null,
  balance: null,
  has_access_token: true,
  has_refresh_token: false,
  has_admin_key: false,
  has_user_id: false,
  headers: {},
  header_names: [],
  cookie_names: [],
  groups: [],
};
const second: UpstreamConfiguration = {
  ...configuration,
  upstream_id: "up_second",
  host: "second.example.test",
  name: "第二上游",
  base_url: "https://second.example.test",
  account_base_url: "https://second.example.test/v1",
};
const setting: UpstreamAllocationSetting = {
  revision: "v1",
  target_id: "up_first",
  upstream_id: "up_first",
  override: null,
  selected: true,
  effective: true,
  global_enabled: true,
  source: "policy",
};
let client: QueryClient;
let onSaved: ReturnType<typeof vi.fn>;
let onOpenChange: ReturnType<typeof vi.fn>;
beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  seedDictionaries(client);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  for (const data of [configuration, second]) {
    client.setQueryData(["upstream-configuration", data.host], data);
    client.setQueryData(["upstream-allocation", "upstreams", data.upstream_id], {
      ...setting,
      upstream_id: data.upstream_id,
      target_id: data.upstream_id,
    });
  }
  onSaved = vi.fn();
  onOpenChange = vi.fn();
  vi.stubGlobal("PointerEvent", MouseEvent);
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function editor(host: string | null = configuration.host) {
  return (
    <QueryClientProvider client={client}>
      <UpstreamEditDialog host={host} onSaved={onSaved} onOpenChange={onOpenChange} />
    </QueryClientProvider>
  );
}

it("切换上游时取消原配置读取，旧响应不进入新表单", async () => {
  client.removeQueries({ queryKey: ["upstream-configuration", configuration.host] });
  let signal: AbortSignal | undefined;
  let finish!: (value: UpstreamConfiguration) => void;
  vi.spyOn(api, "upstreamConfiguration").mockImplementation((_host, incoming) => {
    signal = incoming;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  const view = render(editor());
  await waitFor(() => expect(finish).toBeDefined());
  view.rerender(editor(second.host));
  expect(signal?.aborted).toBe(true);
  await act(async () => finish(configuration));
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue(second.name);
});

it("保存中锁定字段和页签，失败后保留输入并恢复编辑", async () => {
  let reject!: (error: Error) => void;
  const update = vi.spyOn(api, "updateUpstreamConfiguration").mockReturnValue(
    new Promise((_resolve, fail) => {
      reject = fail;
    }),
  );
  vi.spyOn(toast, "error").mockReturnValue("error");
  render(editor());
  const name = screen.getByRole("textbox", { name: "名称" });
  await userEvent.clear(name);
  await userEvent.type(name, "待保存名称");
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(update).toHaveBeenCalledOnce());
  expect(name).toBeDisabled();
  expect(screen.getByLabelText("Token")).toBeDisabled();
  expect(screen.getByRole("combobox", { name: "平台" })).toBeDisabled();
  expect(screen.getByRole("switch", { name: "自定义 Headers" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("tab", { name: "关联账号" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  await act(async () => reject(new Error("保存失败")));
  await waitFor(() => expect(name).toBeEnabled());
  expect(name).toHaveValue("待保存名称");
});

it("关闭后共享额度保存才返回，不再继续提交上游配置", async () => {
  let finish!: (value: UpstreamAllocationSetting) => void;
  const allocation = vi.spyOn(api, "setUpstreamAllocationSetting").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const update = vi.spyOn(api, "updateUpstreamConfiguration").mockResolvedValue(configuration);
  const view = render(editor());
  await userEvent.click(screen.getByRole("switch", { name: "上游共享并发分配" }));
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(allocation).toHaveBeenCalledOnce());
  view.rerender(editor(null));
  await act(async () => finish({ ...setting, revision: "v2", override: false, effective: false }));
  await waitFor(() => expect(client.isMutating()).toBe(0));
  expect(update).not.toHaveBeenCalled();
  expect(onSaved).not.toHaveBeenCalled();
});

it("同一上游关闭重开后旧保存返回，保留新草稿且不提示旧成功", async () => {
  let finish!: (value: UpstreamConfiguration) => void;
  const update = vi.spyOn(api, "updateUpstreamConfiguration").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const notify = vi.spyOn(toast, "success");
  const view = render(editor());
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(update).toHaveBeenCalledOnce());
  view.rerender(editor(null));
  view.rerender(editor());
  const name = screen.getByRole("textbox", { name: "名称" });
  await userEvent.clear(name);
  await userEvent.type(name, "重新打开的草稿");
  await act(async () => finish({ ...configuration, name: "旧保存的名称" }));
  await waitFor(() => expect(client.isMutating()).toBe(0));
  expect(name).toHaveValue("重新打开的草稿");
  expect(notify).not.toHaveBeenCalled();
  expect(onSaved).not.toHaveBeenCalled();
  expect(client.getQueryData(["upstream-configuration", configuration.host])).toMatchObject({
    name: "旧保存的名称",
  });
});

it("切换上游后旧地址迁移成功，不关闭当前编辑或调用旧保存回调", async () => {
  let finish!: (value: UpstreamConfiguration) => void;
  const update = vi.spyOn(api, "updateUpstreamConfiguration").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const view = render(editor());
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(update).toHaveBeenCalledOnce());
  view.rerender(editor(second.host));
  const renamed = {
    ...configuration,
    host: "moved.example.test",
    base_url: "https://moved.example.test",
  };
  await act(async () => finish(renamed));
  await waitFor(() => expect(client.isMutating()).toBe(0));
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue(second.name);
  expect(onOpenChange).not.toHaveBeenCalled();
  expect(onSaved).not.toHaveBeenCalled();
  expect(client.getQueryData(["upstream-configuration", renamed.host])).toEqual(renamed);
});

it("关闭后旧保存失败不提示到新编辑，当前上游仍可保存", async () => {
  let reject!: (error: Error) => void;
  const update = vi.spyOn(api, "updateUpstreamConfiguration").mockReturnValue(
    new Promise((_resolve, fail) => {
      reject = fail;
    }),
  );
  const notify = vi.spyOn(toast, "error").mockReturnValue("error");
  const view = render(editor());
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(update).toHaveBeenCalledOnce());
  view.rerender(editor(second.host));
  await act(async () => reject(new Error("旧上游写入失败")));
  await waitFor(() => expect(client.isMutating()).toBe(0));
  expect(notify).not.toHaveBeenCalled();
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue(second.name);
  expect(screen.getByRole("button", { name: "保存并重算" })).toBeEnabled();
});

it("后台配置读取迟到时不覆盖已经确认的保存结果", async () => {
  let finishRead!: (value: UpstreamConfiguration) => void;
  let readSignal: AbortSignal | undefined;
  vi.spyOn(api, "upstreamConfiguration").mockImplementation((_host, signal) => {
    readSignal = signal;
    return new Promise((resolve) => {
      finishRead = resolve;
    });
  });
  let finishSave!: (value: UpstreamConfiguration) => void;
  const update = vi.spyOn(api, "updateUpstreamConfiguration").mockReturnValue(
    new Promise((resolve) => {
      finishSave = resolve;
    }),
  );
  render(editor());
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(update).toHaveBeenCalledOnce());
  let reading!: Promise<void>;
  act(() => {
    reading = client.refetchQueries({
      queryKey: ["upstream-configuration", configuration.host],
      exact: true,
    });
  });
  await waitFor(() => expect(finishRead).toBeDefined());
  await act(async () => finishSave({ ...configuration, name: "已确认名称" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
  expect(readSignal?.aborted).toBe(true);
  await act(async () => {
    finishRead(configuration);
    await reading;
  });
  expect(screen.getByRole("textbox", { name: "名称" })).toHaveValue("已确认名称");
  expect(client.getQueryData(["upstream-configuration", configuration.host])).toMatchObject({
    name: "已确认名称",
  });
});

it("配置读取失败时保留取消入口，重新读取成功后才允许保存", async () => {
  client.removeQueries({ queryKey: ["upstream-configuration", configuration.host] });
  const read = vi
    .spyOn(api, "upstreamConfiguration")
    .mockRejectedValueOnce(new Error("配置读取失败"))
    .mockResolvedValue(configuration);
  vi.spyOn(toast, "error").mockReturnValue("error");
  render(editor());
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "保存并重算" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  expect(screen.queryByRole("textbox", { name: "名称" })).not.toBeInTheDocument();
  await userEvent.click(retry);
  expect(await screen.findByRole("textbox", { name: "名称" })).toHaveValue(configuration.name);
  expect(screen.getByRole("button", { name: "保存并重算" })).toBeEnabled();
  expect(read).toHaveBeenCalledTimes(2);
});
