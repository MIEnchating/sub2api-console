import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { TaskDetailsDialog } from "../task-details-dialog";

let client: QueryClient;
const preview: Task = {
  id: "preview",
  skill: "sub2api-account-model-automation",
  operation: "account-model-automation",
  status: "running",
  progress: 1,
  message: "正在读取账号模型目录（12/257）",
  result: { dry_run: true, schedule_id: "schedule", phase: "discovering", items: [], models: [] },
  created_at: "2026-10-08T00:00:00Z",
  updated_at: "2026-10-08T00:00:00Z",
};
function show(task: Task) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["task", "preview"], task);
  return render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog taskId="preview" onClose={vi.fn()} />
    </QueryClientProvider>,
  );
}
afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
});

it("计划运行尚在读取账号时展示实时进度和自动执行说明", () => {
  show(preview);
  expect(screen.getAllByText("正在读取账号模型目录（12/257）").length).toBeGreaterThan(0);
  expect(screen.getByText(/预览通过后将自动执行/)).toBeVisible();
  expect(screen.queryByText("模型上架预览（尚未执行）")).not.toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "执行前检查" })).toBeVisible();
  expect(screen.getByText("正在汇总账号可用模型")).toBeVisible();
  expect(screen.queryByText("读取到模型 0 个")).not.toBeInTheDocument();
  expect(screen.queryByText("预览结果（尚未执行）")).not.toBeInTheDocument();
});

it("计划预览结束后轮询等待正式执行关联", async () => {
  vi.spyOn(api, "task").mockResolvedValue({
    ...preview,
    status: "succeeded",
    result: { ...preview.result, execution_error: "渠道版本已变化，请重新运行" },
  });
  show({ ...preview, status: "succeeded" });
  expect(await screen.findByText(/渠道版本已变化，请重新运行/)).toBeVisible();
});

it("计划预览已关联正式任务时详情展示实际上架结果", async () => {
  vi.spyOn(api, "task").mockResolvedValue({
    ...preview,
    id: "execution",
    status: "succeeded",
    progress: 100,
    message: "已上架 1 个模型",
    result: {
      dry_run: false,
      result_version: 2,
      models: ["model-a"],
      items: [{ channel_id: "11", status: "succeeded", added: ["model-a"] }],
    },
  });
  show({
    ...preview,
    status: "succeeded",
    result: { ...preview.result, execution_task_id: "execution" },
  });
  await waitFor(() => expect(screen.getByRole("region", { name: "运营执行结果" })).toBeVisible());
  expect(screen.getByText("execution")).toBeVisible();
  expect(screen.getByText(/已新增（1 个）/)).toBeVisible();
  expect(screen.queryByRole("region", { name: "运营预览结果" })).not.toBeInTheDocument();
});

it("计划预览通过但正式任务启动失败时显示具体原因", () => {
  show({
    ...preview,
    status: "succeeded",
    result: { ...preview.result, execution_error: "账号模型写入需要完全模式" },
  });
  expect(screen.getByText(/正式执行未启动：账号模型写入需要完全模式/)).toBeVisible();
  expect(screen.getByText("执行未启动")).toBeVisible();
});

it("正式执行详情读取失败后可以重新读取并显示执行状态", async () => {
  const user = userEvent.setup();
  vi.spyOn(api, "task")
    .mockRejectedValueOnce(new Error("任务详情暂时无法读取"))
    .mockResolvedValue({
      ...preview,
      id: "execution",
      status: "running",
      message: "正在上架所选模型",
      result: { dry_run: false, result_version: 2, items: [] },
    });
  show({
    ...preview,
    status: "succeeded",
    result: { ...preview.result, execution_task_id: "execution" },
  });
  await user.click(await screen.findByRole("button", { name: "重新读取" }));
  expect(await screen.findByRole("region", { name: "运营执行结果" })).toBeVisible();
  expect(screen.getByText("execution")).toBeVisible();
  expect(screen.getAllByText("正在上架所选模型").length).toBeGreaterThan(0);
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});

it("独立手动预览完成时保留尚未写入的说明", () => {
  show({ ...preview, status: "succeeded", result: { dry_run: true, items: [] } });
  expect(screen.getByText("预览结果（尚未执行）")).toBeVisible();
  expect(screen.queryByText(/预览通过后将自动执行/)).not.toBeInTheDocument();
});
