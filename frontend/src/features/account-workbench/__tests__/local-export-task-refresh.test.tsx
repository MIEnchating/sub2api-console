import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { WorkbenchExportArtifacts } from "../components/workbench-export-artifacts";
import { WorkbenchTask } from "../components/workbench-task";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});

it("本地转换任务完成后自动刷新已缓存的本地私有文件列表", async () => {
  const task: Task = {
    id: "local-conversion",
    skill: "account-workbench",
    operation: "account-workbench-convert",
    status: "queued",
    progress: 0,
    message: "等待转换",
    result: {},
    created_at: "2026-09-14T00:00:00Z",
    updated_at: "2026-09-14T00:00:00Z",
  };
  let finish!: (task: Task) => void;
  vi.spyOn(api, "task").mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  vi.spyOn(api, "workbenchLocalExports").mockResolvedValue([
    {
      id: "completed-local-file",
      kind: "accounts",
      count: 1,
      created_at: task.created_at,
      expires_at: "2026-09-15T00:00:00Z",
    },
  ]);
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryDefaults(workbenchKeys.localExports, { staleTime: Infinity });
  client.setQueryData(workbenchKeys.localExports, []);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchTask task={task} />
      <WorkbenchExportArtifacts scope="local-export" />
    </QueryClientProvider>,
  );
  expect(screen.getByText("暂无私有导出文件")).toBeVisible();
  await act(async () =>
    finish({ ...task, status: "succeeded", progress: 100, message: "转换完成" }),
  );
  expect(await screen.findByText("completed-local-file")).toBeVisible();
});
