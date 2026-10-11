import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, type AccountDetail, type Task } from "@/api";
import { account, task } from "../../__tests__/fixtures";
import { AccountSettingsPanel } from "../account-settings-panel";

let client: QueryClient;
const detail: AccountDetail = {
  ...account,
  metadata: {},
  group_rates: {},
  group_ids: {},
  bindings: [],
  test_models: ["current-model"],
};
beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } },
  });
  vi.stubGlobal("PointerEvent", MouseEvent);
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function panel(id = "41", data: AccountDetail = { ...detail, id }, onSaved = vi.fn()) {
  return (
    <QueryClientProvider client={client}>
      <AccountSettingsPanel
        accountId={id}
        query={{ data, isLoading: false, isError: false, error: null }}
        onCancel={vi.fn()}
        onSaved={onSaved}
      />
    </QueryClientProvider>
  );
}
it("账号切换后取消旧模型读取，迟到模型不进入新账号", async () => {
  let finish!: (value: { models: string[] }) => void;
  let signal: AbortSignal | undefined;
  vi.spyOn(api, "accountModels").mockImplementation((_id, incoming) => {
    signal = incoming;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  const success = vi.spyOn(toast, "success");
  const view = render(panel());
  await userEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(finish).toBeDefined());
  view.rerender(panel("42"));
  expect(signal?.aborted).toBe(true);
  await act(async () => finish({ models: ["old-account-model"] }));
  expect(screen.getByRole("textbox", { name: "探测模型" })).toHaveValue("current-model");
  expect(screen.queryByRole("combobox", { name: "选择探测模型" })).not.toBeInTheDocument();
  expect(success).not.toHaveBeenCalled();
});
it("账号详情与当前账号不匹配时禁止提交旧详情", () => {
  render(panel("42", detail));
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  expect(screen.queryByRole("spinbutton", { name: "优先级" })).not.toBeInTheDocument();
});
it("保存期间禁用所有可编辑字段，失败后保留输入并恢复编辑", async () => {
  let reject!: (error: Error) => void;
  vi.spyOn(api, "saveAccountSettings").mockReturnValue(
    new Promise((_resolve, fail) => {
      reject = fail;
    }),
  );
  vi.spyOn(toast, "error").mockReturnValue("error");
  render(panel());
  const concurrency = screen.getByRole("spinbutton", { name: "并发上限" });
  await userEvent.clear(concurrency);
  await userEvent.type(concurrency, "23");
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(reject).toBeDefined());
  expect(concurrency).toBeDisabled();
  expect(screen.getByRole("textbox", { name: "探测模型" })).toBeDisabled();
  expect(screen.getByRole("switch", { name: "暂停调度" })).toHaveAttribute("aria-disabled", "true");
  expect(screen.getByRole("button", { name: "获取模型" })).toBeDisabled();
  await act(async () => reject(new Error("设置写入失败")));
  await waitFor(() => expect(concurrency).toBeEnabled());
  expect(concurrency).toHaveValue(23);
});
it("保存成功不等待无关的日志缓存失效", async () => {
  vi.spyOn(api, "saveAccountSettings").mockResolvedValue(task("saved"));
  let releaseLogs!: () => void;
  const invalidations: string[] = [];
  vi.spyOn(client, "invalidateQueries").mockImplementation((filters) => {
    const scope = String(filters?.queryKey?.[0] ?? "");
    invalidations.push(scope);
    if (scope === "logs") {
      return new Promise<void>((resolve) => {
        releaseLogs = resolve;
      });
    }
    return Promise.resolve();
  });
  const onSaved = vi.fn();
  render(panel("41", detail, onSaved));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(invalidations).toContain("logs"));
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
  releaseLogs();
});
it("离开编辑后成本墙提交完成，不再继续启动设置任务或关闭新编辑", async () => {
  let finish!: (value: { ignore_cost_wall: boolean }) => void;
  vi.spyOn(api, "setAccountIgnoreCostWall").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const save = vi.spyOn(api, "saveAccountSettings").mockResolvedValue(task("saved"));
  const onSaved = vi.fn();
  const view = render(panel("41", detail, onSaved));
  await userEvent.click(screen.getByRole("switch", { name: "无视成本墙" }));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(finish).toBeDefined());
  view.rerender(panel("42", { ...detail, id: "42" }, onSaved));
  await act(async () => finish({ ignore_cost_wall: true }));
  expect(save).not.toHaveBeenCalled();
  expect(onSaved).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "保存" })).toBeEnabled();
});
it("关闭编辑取消任务状态读取，已提交任务不被自动取消", async () => {
  vi.spyOn(api, "saveAccountSettings").mockResolvedValue(task("running", "running"));
  let signal: AbortSignal | undefined;
  let finish!: (value: Task) => void;
  vi.spyOn(api, "task").mockImplementation((_id, incoming) => {
    signal = incoming;
    return new Promise((resolve) => {
      finish = resolve;
    });
  });
  const cancel = vi.spyOn(api, "cancelTask");
  const onSaved = vi.fn();
  const view = render(panel("41", detail, onSaved));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(finish).toBeDefined());
  view.unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => finish(task("running")));
  expect(onSaved).not.toHaveBeenCalled();
  expect(cancel).not.toHaveBeenCalled();
});

it("模型读取失败保留手动输入，点击重试后才重新读取", async () => {
  const read = vi
    .spyOn(api, "accountModels")
    .mockRejectedValueOnce(new Error("模型目录读取失败"))
    .mockResolvedValue({ models: [] });
  const notify = vi.spyOn(toast, "error").mockReturnValue("error");
  render(panel());
  await userEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(notify).toHaveBeenCalledOnce());
  expect(screen.getByRole("textbox", { name: "探测模型" })).toHaveValue("current-model");
  expect(read).toHaveBeenCalledTimes(1);
  await userEvent.click(screen.getByRole("button", { name: "获取模型" }));
  expect(await screen.findByRole("combobox", { name: "选择探测模型" })).toHaveTextContent(
    "current-model",
  );
  expect(screen.getByRole("status")).toHaveTextContent("上游未返回可用模型");
  expect(read).toHaveBeenCalledTimes(2);
});

it("旧账号设置任务迟到返回时不关闭新账号，只失效旧账号详情", async () => {
  let finish!: (value: Task) => void;
  vi.spyOn(api, "saveAccountSettings").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const success = vi.spyOn(toast, "success");
  const onSaved = vi.fn();
  client.setQueryData(["account-detail", "41"], detail);
  client.setQueryData(["account-detail", "42"], { ...detail, id: "42" });
  const view = render(panel("41", detail, onSaved));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(finish).toBeDefined());
  view.rerender(panel("42", { ...detail, id: "42" }, onSaved));
  await act(async () => finish(task("old-save")));
  expect(onSaved).not.toHaveBeenCalled();
  expect(success).not.toHaveBeenCalled();
  expect(client.getQueryState(["account-detail", "41"])?.isInvalidated).toBe(true);
  expect(client.getQueryState(["account-detail", "42"])?.isInvalidated).toBe(false);
});
