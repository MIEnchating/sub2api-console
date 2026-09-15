import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchMaintenanceAuthorization } from "../components/workbench-maintenance-authorization";
import { workbenchKeys } from "../constants";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it("取消确认期间后台切换授权队列时仍只取消原先确认的批次", async () => {
  const fetcher = vi.fn<typeof fetch>(async () => Response.json({ attached: true }));
  vi.stubGlobal("fetch", fetcher);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const key = ["account-workbench", "maintenance-authorization"];
  client.setQueryData(key, { attached: true, current_reauthorization_id: "batch-a" });
  for (const id of ["batch-a", "batch-b"])
    client.setQueryData(workbenchKeys.batch(id), {
      id,
      task_id: id,
      status: "running",
      items: [],
      available: 0,
      message: id,
    });
  render(
    <QueryClientProvider client={client}>
      <WorkbenchMaintenanceAuthorization
        config={{
          enabled: true,
          reauthorize_with_profiles: true,
          revision: 4,
          interval_minutes: 5,
          cooldown_minutes: 10,
          group_ids: [],
          check_after_repair: false,
          model: "",
        }}
      />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "取消本轮重新授权" }));
  await act(async () => {
    client.setQueryData(key, { attached: true, current_reauthorization_id: "batch-b" });
  });
  await screen.findByText("batch-b");
  fireEvent.click(screen.getByRole("button", { name: "取消本轮授权" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth-batches/batch-a",
      expect.objectContaining({ method: "DELETE" }),
    ),
  );
});
