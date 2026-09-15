import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Task } from "@/api";
import { WorkbenchTaskFiles } from "../components/workbench-task-files";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("查看本地转换结果只展示本任务产物且不提供资料库或再生工具", async () => {
  const requests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (url) => {
      requests.push(String(url));
      return Response.json([
        { id: "this-file", kind: "accounts", count: 2, expires_at: "2099-01-01T00:00:00Z" },
        { id: "other-file", kind: "accounts", count: 8, expires_at: "2099-01-01T00:00:00Z" },
      ]);
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const task: Task = {
    id: "file-task",
    skill: "account-workbench",
    operation: "account-workbench-convert",
    status: "succeeded",
    progress: 100,
    message: "完成",
    created_at: "",
    updated_at: "",
    result: { items: [{ report: { artifact_id: "this-file", scope: "local-export" } }] },
  };
  render(
    <QueryClientProvider client={client}>
      <WorkbenchTaskFiles task={task} />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("this-file")).toBeVisible();
  expect(screen.queryByText("other-file")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /再生成|登录资料/ })).not.toBeInTheDocument();
  await waitFor(() => expect(requests).toEqual(["/api/account-workbench/local-exports"]));
});
