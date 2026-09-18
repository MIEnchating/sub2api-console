import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { GroupsPage } from "@/App";
import { api, type GroupStatus } from "@/api";
import { policy } from "@/features/accounts/__tests__/fixtures";

let client: QueryClient | undefined;

afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

it.each([
  { scenario: "未纳入指定范围且没有独立开关", enabled: undefined, expected: true },
  { scenario: "明确关闭分组守护", enabled: false, expected: false },
  { scenario: "明确启用分组守护但尚未纳入范围", enabled: true, expected: true },
])("$scenario 时编辑调度策略保留自身参与守护设置", async (test) => {
  const group: GroupStatus = {
    id: "7",
    name: "codex",
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "out_of_scope",
    participation_reason: "当前仅守护指定分组，该分组未加入参与分组列表",
    status: "skipped",
    override: test.enabled === undefined ? null : { enabled: test.enabled },
  };
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "groupProbeModels").mockResolvedValue({
    group_id: "7",
    group_name: group.name,
    models: [],
    account_count: 1,
    accounts_with_models: 0,
    complete: true,
  });
  const save = vi.spyOn(api, "updateGroupPolicy").mockResolvedValue(group);
  client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["groups"], [group]);
  client.setQueryData(["policy"], {
    ...policy,
    advanced_policy: { scope: { managed_group_mode: "selected", managed_group_ids: ["8"] } },
  });
  render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );

  fireEvent.click(screen.getByRole("button", { name: "编辑分组" }));
  const dialog = within(screen.getByRole("dialog", { name: "编辑分组策略" }));
  expect(dialog.getByRole("switch", { name: "参与守护" })).toHaveAttribute(
    "aria-checked",
    String(test.expected),
  );
  fireEvent.click(dialog.getByRole("radio", { name: "价格优先" }));
  fireEvent.click(dialog.getByRole("button", { name: "保存策略" }));

  await waitFor(() =>
    expect(save).toHaveBeenCalledWith(
      "7",
      expect.objectContaining({ strategy: "price_first", enabled: test.expected }),
    ),
  );
});
