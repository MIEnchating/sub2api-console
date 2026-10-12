import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { AccountModelAutomationSchedule } from "@/api";
import { AccountModelAutomationDialog } from "../account-model-automation-dialog";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});

it("保存任务等待期间锁定提交范围并阻止同时预览，失败后保留草稿恢复编辑", async () => {
  const schedule: AccountModelAutomationSchedule = {
    id: "a".repeat(32),
    name: "生产同步",
    enabled: true,
    interval_minutes: 60,
    last_run_at: null,
    next_run_at: null,
    request: {
      platform_id: "primary",
      group_ids: ["7"],
      channels: [],
      selected_models: [],
      publish_missing: false,
      price_enabled: false,
      pricing_source: "official",
    },
  };
  let complete!: (response: Response) => void;
  vi.spyOn(globalThis, "fetch").mockImplementation((url, options) => {
    if (options?.method === "PUT" || options?.method === "POST") {
      return new Promise((resolve) => {
        complete = resolve;
      });
    }
    if (String(url) === "/api/catalog/group-memberships")
      return Promise.resolve(Response.json([{ id: "7", name: "生产组", account_count: 2 }]));
    if (String(url).startsWith("/api/newapi?"))
      return Promise.resolve(
        Response.json({
          platforms: [],
          local_groups: [],
          sub2api_base_url: "",
          bindings: [{ sub2api_group_id: "7" }],
        }),
      );
    throw new Error(`Unexpected read: ${url}`);
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={client}>
      <AccountModelAutomationDialog
        open
        platformId="primary"
        schedule={schedule}
        channels={[]}
        onOpenChange={vi.fn()}
      />
    </QueryClientProvider>,
  );
  const save = screen.getByRole("button", { name: "保存任务" });
  await waitFor(() => expect(save).toBeEnabled());
  await user.click(save);
  await screen.findByRole("button", { name: "保存中" });

  const name = screen.getByRole("textbox", { name: "运营任务名称" });
  expect(name).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: "启用定时运行" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(screen.getByRole("checkbox", { name: /生产组/ })).toHaveAttribute("aria-disabled", "true");
  expect(screen.getByRole("button", { name: "读取可用模型" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "预览" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();

  await act(async () => {
    complete(Response.json({ detail: "保存失败，请重试" }, { status: 503 }));
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "保存任务" })).toBeEnabled());
  expect(name).toBeEnabled();
  expect(name).toHaveValue("生产同步");
  expect(screen.getByRole("checkbox", { name: /生产组/ })).toBeChecked();
  expect(screen.getByRole("button", { name: "预览" })).toBeEnabled();
});
