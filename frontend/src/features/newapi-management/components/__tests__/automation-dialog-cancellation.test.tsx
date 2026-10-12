import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, type Task } from "@/api";
import { AccountModelAutomationDialog } from "../account-model-automation-dialog";

let client: QueryClient;

function task(status: Task["status"] = "running", dryRun = true): Task {
  return {
    id: dryRun ? "temporary-task" : "write-task",
    skill: "sub2api-account-model-automation",
    operation: "account-model-automation",
    status,
    progress: status === "succeeded" ? 100 : 0,
    message: "正在读取模型",
    result: { dry_run: dryRun, models: ["model-a"], items: [] },
    created_at: "2026-10-08T00:00:00Z",
    updated_at: "2026-10-08T00:00:00Z",
  };
}

function Editor() {
  const [open, setOpen] = useState(true);
  if (!open) return <p>弹窗已关闭</p>;
  return (
    <AccountModelAutomationDialog open onOpenChange={setOpen} platformId="platform" channels={[]} />
  );
}

function renderEditor() {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <Editor />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(api, "groupMembershipOptions").mockResolvedValue([
    {
      id: "7",
      name: "生产分组",
      account_count: 1,
    },
  ]);
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [],
    local_groups: [],
    sub2api_base_url: "",
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "生产分组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
  });
  vi.spyOn(api, "enqueueAccountModelAutomation").mockResolvedValue(task());
  vi.spyOn(api, "task").mockResolvedValue(task());
  vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
});

afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("关闭分组绑定读取中的编辑弹窗时取消平台工作区请求", async () => {
  const user = userEvent.setup();
  vi.mocked(api.newAPIWorkspace).mockImplementation(
    (_platformId, signal) =>
      new Promise((_resolve, reject) => {
        signal?.addEventListener("abort", () => reject(new DOMException("已取消", "AbortError")));
      }),
  );
  renderEditor();
  await waitFor(() => expect(api.newAPIWorkspace).toHaveBeenCalled());
  expect(api.newAPIWorkspace).toHaveBeenCalledWith("platform", expect.any(AbortSignal));
  const signal = vi.mocked(api.newAPIWorkspace).mock.calls[0]?.[1];
  await user.click(screen.getByRole("button", { name: "取消" }));
  await screen.findByText("弹窗已关闭");
  expect(signal?.aborted).toBe(true);
});

it.each(["取消", "Escape"])("模型读取运行中通过 %s 关闭弹窗时取消临时任务", async (action) => {
  const user = userEvent.setup();
  renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await screen.findByRole("status", { name: "正在读取模型" });
  if (action === "Escape") await user.keyboard("{Escape}");
  else await user.click(screen.getByRole("button", { name: "取消" }));
  await screen.findByText("弹窗已关闭");
  await waitFor(() => expect(api.cancelTask).toHaveBeenCalledWith("temporary-task"));
  expect(api.cancelTask).toHaveBeenCalledTimes(1);
});

it("预览运行中离开页面时取消弹窗创建的临时任务", async () => {
  const user = userEvent.setup();
  const view = renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "预览" }));
  await waitFor(() =>
    expect(api.task).toHaveBeenCalledWith("temporary-task", expect.any(AbortSignal)),
  );
  view.unmount();
  await waitFor(() => expect(api.cancelTask).toHaveBeenCalledWith("temporary-task"));
});

it("任务创建尚未返回就关闭弹窗时在获得任务 ID 后取消", async () => {
  const user = userEvent.setup();
  let resolveTask!: (value: Task) => void;
  vi.mocked(api.enqueueAccountModelAutomation).mockReturnValue(
    new Promise((resolve) => {
      resolveTask = resolve;
    }),
  );
  renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await waitFor(() => expect(api.enqueueAccountModelAutomation).toHaveBeenCalled());
  await user.click(screen.getByRole("button", { name: "取消" }));
  await act(async () => {
    resolveTask(task());
  });
  await waitFor(() => expect(api.cancelTask).toHaveBeenCalledWith("temporary-task"));
});

it("模型读取已完成再关闭弹窗时不请求取消", async () => {
  const user = userEvent.setup();
  vi.mocked(api.enqueueAccountModelAutomation).mockResolvedValue(task("succeeded"));
  vi.mocked(api.task).mockResolvedValue(task("succeeded"));
  renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "读取可用模型" })).toBeEnabled());
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(api.cancelTask).not.toHaveBeenCalled();
});

it("确认写入后关闭弹窗时正式任务继续在后台运行", async () => {
  const user = userEvent.setup();
  vi.mocked(api.enqueueAccountModelAutomation)
    .mockResolvedValueOnce(task("succeeded"))
    .mockResolvedValueOnce(task("running", false));
  vi.mocked(api.task).mockImplementation(async (id) =>
    id === "write-task" ? task("running", false) : task("succeeded"),
  );
  renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "预览" }));
  await user.click(await screen.findByRole("button", { name: "确认写入" }));
  await waitFor(() => expect(api.task).toHaveBeenCalledWith("write-task", expect.any(AbortSignal)));
  await user.click(screen.getByRole("button", { name: "关闭弹窗" }));
  expect(api.cancelTask).not.toHaveBeenCalled();
});

it("关闭时取消请求失败且任务仍在运行时提示到任务记录取消", async () => {
  const user = userEvent.setup();
  const notify = vi.spyOn(toast, "error");
  vi.mocked(api.cancelTask).mockRejectedValue(new Error("服务暂时不可用"));
  renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await screen.findByRole("status", { name: "正在读取模型" });
  await user.click(screen.getByRole("button", { name: "取消" }));
  await waitFor(() =>
    expect(notify).toHaveBeenCalledWith(
      expect.stringContaining("请到任务记录查看并取消"),
      expect.any(Object),
    ),
  );
});

it("关闭时取消请求与任务完成同时发生时不误报取消失败", async () => {
  const user = userEvent.setup();
  const notify = vi.spyOn(toast, "error");
  vi.mocked(api.cancelTask).mockRejectedValue(new Error("任务已经结束，无法取消"));
  renderEditor();
  await user.click(await screen.findByRole("checkbox", { name: /生产分组/ }));
  await user.click(screen.getByRole("button", { name: "读取可用模型" }));
  await screen.findByRole("status", { name: "正在读取模型" });
  const previousQueries = vi.mocked(api.task).mock.calls.length;
  vi.mocked(api.task).mockResolvedValue(task("succeeded"));
  await user.click(screen.getByRole("button", { name: "取消" }));
  await waitFor(() => expect(api.cancelTask).toHaveBeenCalled());
  // Wait for the follow-up query that establishes completion.
  await waitFor(() => expect(api.task).toHaveBeenCalledTimes(previousQueries + 1));
  await act(async () => {});
  expect(notify).not.toHaveBeenCalled();
});
