import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { AccountProbeDialog } from "../account-probe-dialog";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function task(status: Task["status"], stage: string): Task {
  return {
    id: "probe-task",
    skill: "onboarding",
    operation: "onboarding-probe-models",
    status,
    progress: 0,
    message: "临时 Key 创建失败",
    created_at: "",
    updated_at: "",
    result: {
      models: ["kimi-k2"],
      steps: [
        {
          stage,
          status: status === "running" ? "running" : "failed",
          started_at: "2026-09-11T00:00:00Z",
        },
      ],
    },
  };
}

function setup() {
  vi.spyOn(api, "accountCreationSettings").mockImplementation(() => new Promise(() => {}));
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const close = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <AccountProbeDialog
        open
        target={{
          kind: "onboarding",
          host: "upstream.test",
          groupId: "6",
          name: "Kimi",
          platform: "kimi",
        }}
        onOpenChange={close}
      />
    </QueryClientProvider>,
  );
  return { client, close };
}

it("Key 创建未完成时只显示后端实际阶段，禁止提前探活且保留取消入口", async () => {
  vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(task("queued", "create_key"));
  vi.spyOn(api, "task").mockResolvedValue(task("running", "create_key"));
  const view = setup();
  await userEvent.click(await screen.findByRole("button", { name: /创建临时上游 Key/ }));
  const timeline = await screen.findByRole("list", { name: "探活过程" });
  expect(within(timeline).getByText("创建临时上游 Key")).toBeInTheDocument();
  expect(within(timeline).getByText("进行中")).toBeInTheDocument();
  expect(within(timeline).queryByText("获取上游模型列表")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "开始测试" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消探活" })).toBeEnabled();
  vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
  cleanup();
  view.client.clear();
});

it("Key 创建失败时保留失败阶段并提供模型获取重试", async () => {
  vi.spyOn(api, "startOnboardingProbeTask").mockResolvedValue(task("queued", "create_key"));
  vi.spyOn(api, "task").mockResolvedValue(task("failed", "create_key"));
  const view = setup();
  await userEvent.click(await screen.findByRole("button", { name: /创建临时上游 Key/ }));
  const timeline = await screen.findByRole("list", { name: "探活过程" });
  expect(await within(timeline).findByText("失败")).toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole("button", { name: "获取上游模型" })).toBeEnabled());
  expect(screen.getByRole("button", { name: "开始测试" })).toBeDisabled();
  cleanup();
  view.client.clear();
});

it("任务创建期间关闭弹窗会在拿到任务 ID 后取消并等待清理结束", async () => {
  let resolveStart!: (value: Task) => void;
  const start = vi.spyOn(api, "startOnboardingProbeTask").mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveStart = resolve;
      }),
  );
  start.mockResolvedValue({ ...task("succeeded", "cleanup_key"), id: "cleanup-task" });
  const cancel = vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
  vi.spyOn(api, "task").mockImplementation(async (id) => ({
    ...task(id === "cleanup-task" ? "succeeded" : "cancelled", "cleanup_key"),
    id,
  }));
  const view = setup();
  await userEvent.keyboard("{Escape}");
  expect(view.close).not.toHaveBeenCalled();
  await act(async () => resolveStart(task("queued", "create_key")));
  await waitFor(() => expect(cancel).toHaveBeenCalledWith("probe-task"));
  await waitFor(() => expect(view.close).toHaveBeenCalledWith(false));
  expect(start).toHaveBeenLastCalledWith("cleanup", "upstream.test", "6", undefined, undefined);
  cleanup();
  view.client.clear();
});
