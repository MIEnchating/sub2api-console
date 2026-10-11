import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { api, type AccountModelAutomationSchedule, type GroupStatus } from "@/api";
import { AccountModelAutomationPage } from "../account-model-automation-page";

const group: GroupStatus = {
  id: "7",
  name: "生产组",
  account_count: 1,
  scheduling_open: 1,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "default",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "healthy",
};
const schedules: AccountModelAutomationSchedule[] = ["任务 A", "任务 B"].map((name, index) => ({
  id: String(index + 1).repeat(32),
  name,
  request: {
    platform_id: "platform",
    group_ids: ["7"],
    channels: [],
    selected_models: [],
    publish_missing: false,
    price_enabled: false,
    pricing_source: "official",
  },
  enabled: true,
  schedule_type: "interval",
  interval_minutes: 60,
  last_run_at: null,
  next_run_at: null,
}));
let client: QueryClient | undefined;

afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

it("旧任务保存完成时保留新打开任务的编辑草稿", async () => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  vi.spyOn(api, "newAPIWorkspace").mockResolvedValue({
    platforms: [
      {
        id: "platform",
        name: "测试平台",
        base_url: "https://example.test",
        user_id: "1",
        admin_key_configured: true,
        updated_at: "2026-10-09T00:00:00Z",
      },
    ],
    local_groups: [{ id: "7", name: "生产组", ratio: null }],
    bindings: [
      {
        platform_id: "platform",
        newapi_group_id: "remote-7",
        newapi_group_name: "生产组",
        sub2api_group_id: "7",
        sync_ratio: true,
      },
    ],
    sub2api_base_url: "",
  });
  vi.spyOn(api, "accountModelAutomationSchedules").mockResolvedValue(schedules);
  vi.spyOn(api, "accountModelAutomationTaskGroups").mockResolvedValue([]);
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(globalThis, "fetch").mockImplementation(
    async () => new Response(JSON.stringify({ items: [], total: 0 }), { status: 200 }),
  );
  let completeSave!: (schedule: AccountModelAutomationSchedule) => void;
  const save = vi.spyOn(api, "saveAccountModelAutomationSchedule").mockImplementation(
    () =>
      new Promise((resolve) => {
        completeSave = resolve;
      }),
  );
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <AccountModelAutomationPage />
    </QueryClientProvider>,
  );

  await user.click(
    within(await screen.findByRole("group", { name: "任务 A 的操作" })).getByRole("button", {
      name: "编辑",
    }),
  );
  const firstDialog = await screen.findByRole("dialog", { name: "编辑运营任务" });
  await user.click(await within(firstDialog).findByRole("button", { name: "保存任务" }));
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  await user.click(within(firstDialog).getByRole("button", { name: "取消" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

  await user.click(
    within(screen.getByRole("group", { name: "任务 B 的操作" })).getByRole("button", {
      name: "编辑",
    }),
  );
  const secondDialog = await screen.findByRole("dialog", { name: "编辑运营任务" });
  const name = await within(secondDialog).findByRole("textbox", { name: "运营任务名称" });
  await user.clear(name);
  await user.type(name, "任务 B 草稿");

  await act(async () => {
    completeSave(schedules[0]!);
  });
  expect(await screen.findByRole("dialog", { name: "编辑运营任务" })).toContainElement(name);
  expect(name).toHaveValue("任务 B 草稿");
});
