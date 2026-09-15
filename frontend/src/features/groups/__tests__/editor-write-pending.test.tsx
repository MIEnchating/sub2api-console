import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { GroupsPage } from "@/App";
import { api, type GroupStatus } from "@/api";
import { policy } from "@/features/accounts/__tests__/fixtures";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("分组策略保存期间锁定所有字段，失败后恢复编辑并保留草稿", async () => {
  const group: GroupStatus = {
    id: "6",
    name: "测试分组",
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
  };
  vi.spyOn(api, "groups").mockResolvedValue([group]);
  vi.spyOn(api, "groupProbeModels").mockResolvedValue({
    group_id: "6",
    group_name: group.name,
    models: ["probe-a"],
    account_count: 1,
    accounts_with_models: 1,
    complete: true,
  });
  let rejectSave!: (error: Error) => void;
  vi.spyOn(api, "updateGroupPolicy").mockReturnValue(
    new Promise((_resolve, reject) => {
      rejectSave = reject;
    }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["groups"], [group]);
  client.setQueryData(["policy"], policy);
  const view = render(
    <QueryClientProvider client={client}>
      <GroupsPage />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "编辑分组" }));
  const dialog = within(screen.getByRole("dialog", { name: "编辑分组策略" }));
  const model = dialog.getByRole("textbox", { name: "手动输入探活模型" });
  fireEvent.change(model, { target: { value: "probe-draft" } });
  fireEvent.click(dialog.getByRole("button", { name: "保存策略" }));
  await waitFor(() => expect(dialog.getByRole("button", { name: "保存中…" })).toBeDisabled());
  for (const field of [
    ...dialog.getAllByRole("spinbutton"),
    ...dialog.getAllByRole("textbox"),
    ...dialog.getAllByRole("radio"),
  ])
    expect(field).toBeDisabled();
  for (const control of dialog.getAllByRole("switch"))
    expect(control).toHaveAttribute("aria-disabled", "true");
  await act(async () => rejectSave(new Error("保存失败")));
  await waitFor(() => expect(model).toBeEnabled());
  expect(model).toHaveValue("probe-draft");
  expect(dialog.getByRole("button", { name: "保存策略" })).toBeEnabled();
  view.unmount();
  client.clear();
});
