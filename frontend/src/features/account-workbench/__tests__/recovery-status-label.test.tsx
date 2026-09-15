import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchQueueRecovery } from "../components/workbench-queue-recovery";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("恢复资料包含未识别的__proto__项目状态时显示待核对且不崩溃", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async () =>
      Response.json([
        {
          id: "saved-queue",
          kind: "mixed",
          task_id: "source-task",
          status: "interrupted",
          revision: 1,
          expires_at: new Date(Date.now() + 60000).toISOString(),
          can_resume: true,
          items: [{ index: 0, email: "owner@example.test", status: "__proto__" }],
        },
      ]),
    ),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchQueueRecovery kind="mixed" disabled={false} onResume={() => {}} />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  expect(await screen.findByText("待核对", { exact: true })).toBeInTheDocument();
});
