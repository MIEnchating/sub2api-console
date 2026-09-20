import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { PolicyPage } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("启用长期异常自动删除时先确认，取消不写入，确认后保存原策略版本", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("PointerEvent", MouseEvent);
  const current = {
    ...policy,
    revision: "original-policy",
    advanced_policy: {
      ...policy.advanced_policy,
      abnormal_cleanup: {
        enabled: true,
        action: "delete",
        duration_minutes: 120,
        max_per_round: 1,
        keep_last_in_group: true,
      },
    },
  };
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "PUT") writes.push(JSON.parse(String(init.body)));
      return new Response(JSON.stringify(current), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["policy"], current);
  client.setQueryData(["config"], { probes_enabled: true });
  client.setQueryData(["dictionaries", "scheduling_strategy"], { items: [] });
  render(
    <QueryClientProvider client={client}>
      <PolicyPage />
    </QueryClientProvider>,
  );
  await user.click(screen.getByRole("button", { name: "保存策略" }));
  expect(screen.getByRole("dialog", { name: "确认启用自动删除" })).toBeVisible();
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(writes).toHaveLength(0);
  await user.click(screen.getByRole("button", { name: "保存策略" }));
  await user.click(screen.getByRole("button", { name: "确认保存" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toEqual(
    expect.objectContaining({
      expected_revision: "original-policy",
      advanced_policy: expect.objectContaining({
        abnormal_cleanup: current.advanced_policy.abnormal_cleanup,
      }),
    }),
  );
});
