import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { SchedulerHeaderControls } from "../App";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("调度状态首次读取失败时禁止启动，重新读取成功后允许启动", async () => {
  let unavailable = true;
  const requests: string[] = [];
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    requests.push(`${init?.method ?? "GET"} ${input}`);
    if (unavailable) return Response.json({ detail: "调度状态读取失败" }, { status: 503 });
    return Response.json({ enabled: input.endsWith("/resume"), running: false });
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <SchedulerHeaderControls />
    </QueryClientProvider>,
  );

  const start = screen.getByRole("button", { name: "启动自动调度" });
  expect(start).toBeDisabled();
  await screen.findByText("状态读取失败");
  expect(start).toBeDisabled();
  await user.click(start);
  expect(requests).toEqual(["GET /api/inspection/automation"]);

  unavailable = false;
  await client.invalidateQueries({ queryKey: ["auto-inspection"] });
  await waitFor(() => expect(start).toBeEnabled());
  await user.click(start);
  expect(await screen.findByRole("button", { name: "取消自动调度" })).toBeEnabled();
  expect(requests).toContain("POST /api/inspection/automation/resume");
});
