import { act, cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { RunList } from "../components/run-list";
import { runKeys } from "../constants";
import type { WorkbenchRun } from "../types";

const running: WorkbenchRun = {
  id: "11111111111111111111111111111111",
  revision: 1,
  task_id: "task-one",
  action: "import",
  status: "running",
  created_at: "2026-09-17T10:00:00Z",
  updated_at: "2026-09-17T10:00:00Z",
  expires_at: "2099-01-01T00:00:00Z",
  duplicate_count: 0,
  items: [
    {
      id: "item-one",
      index: 0,
      kind: "codex_json",
      name: "",
      email: "one@example.test",
      plan: "",
      identity_source: "official_signature",
      status: "running",
      message: "",
      template_name: "默认配置",
    },
  ],
};

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("运行批次每秒只读取自身，完成后停止高频读取并更新可见状态", async () => {
  vi.useFakeTimers();
  let detailReads = 0;
  let listReads = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith(`/runs/${running.id}`)) {
        detailReads += 1;
        return Response.json({
          ...running,
          revision: detailReads + 1,
          status: detailReads === 2 ? "completed" : "running",
        });
      }
      listReads += 1;
      return Response.json([running]);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(runKeys.list, [running]);
  render(
    <QueryClientProvider client={client}>
      <RunList />
    </QueryClientProvider>,
  );

  await act(async () => {
    await Promise.resolve();
  });
  expect(detailReads).toBe(1);
  expect(listReads).toBe(0);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10);
  });
  expect(detailReads).toBe(2);
  expect(listReads).toBe(0);
  expect(
    client.getQueryData([...runKeys.list, "detail", running.id, running.revision]),
  ).toMatchObject({
    status: "completed",
  });
  expect(screen.getByText("已完成")).toBeVisible();

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });
  expect(detailReads).toBe(2);
  await act(async () => {
    client.setQueryData(runKeys.list, [{ ...running, revision: 5 }]);
    await vi.advanceTimersByTimeAsync(10);
  });
  expect(detailReads).toBe(3);
  expect(screen.getAllByText("处理中")).toHaveLength(2);
  client.clear();
});
