import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { TaskDetailsDialog } from "../task-details-dialog";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

const completed: Task = {
  id: "completed",
  skill: "console",
  operation: "active-probe",
  status: "succeeded",
  progress: 100,
  message: "已读取的执行结果",
  result: {},
  created_at: "2026-10-10T00:00:00Z",
  updated_at: "2026-10-10T00:00:01Z",
};

it.each([false, true])("关联正式任务=%s，终态详情刷新失败后保留内容并可重试", async (linked) => {
  let failed = true;
  vi.stubGlobal("fetch", async () =>
    failed
      ? Response.json({ detail: "任务读取暂不可用" }, { status: 503 })
      : Response.json({ ...completed, message: "重新读取后的执行结果" }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["task", completed.id], completed);
  if (linked)
    client.setQueryData(["task", "preview"], {
      ...completed,
      id: "preview",
      operation: "account-model-automation",
      result: { execution_task_id: completed.id },
    });
  render(
    <QueryClientProvider client={client}>
      <TaskDetailsDialog taskId={linked ? "preview" : completed.id} onClose={() => {}} />
    </QueryClientProvider>,
  );
  await act(async () => {
    await client.refetchQueries({ queryKey: ["task", completed.id], exact: true });
  });
  await waitFor(() => expect(client.getQueryState(["task", completed.id])?.status).toBe("error"));
  expect(screen.getByText("已读取的执行结果")).toBeVisible();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  failed = false;
  await userEvent.setup().click(retry);
  expect(await screen.findByText("重新读取后的执行结果")).toBeVisible();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
