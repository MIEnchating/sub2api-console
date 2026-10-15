import type { QueryClient } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { boundCandidate, renderOnboarding, upstream } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("添加账号成功后立即返回统计变化时，刷新仍在有效期内的未绑定缓存并显示已绑定", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const completed: Task = {
    id: "created-account",
    skill: "onboarding",
    operation: "onboard",
    status: "succeeded",
    progress: 100,
    message: "账号添加完成",
    result: { account_id: "41" },
    created_at: "2026-10-15T00:00:00Z",
    updated_at: "2026-10-15T00:00:00Z",
  };
  vi.spyOn(api, "onboard").mockResolvedValue(completed);
  vi.spyOn(api, "task").mockResolvedValue(completed);
  vi.spyOn(api, "previewOnboardingConcurrency").mockResolvedValue({
    items: [{ concurrency: 1 }],
  });
  vi.spyOn(api, "allUpstreamGroupHistory").mockResolvedValue([
    {
      id: 1,
      upstream_id: upstream.upstream_id,
      group_id: "7",
      group_name: "已有绑定分组",
      change_type: "added",
      changed_at: "2026-10-15T00:00:00Z",
    },
  ]);
  vi.spyOn(api, "upstreamGroupBindingAudit").mockResolvedValue({
    items: [
      {
        upstream_id: upstream.upstream_id,
        host: upstream.host,
        group_id: "7",
        group_name: "已有绑定分组",
        account_count: 1,
        accounts: [{ id: "41", name: "新增账号" }],
        status: "present",
        reason: null,
      },
    ],
    total_bindings: 1,
    present: 1,
    missing: 0,
    unknown: 0,
  });
  vi.spyOn(api, "upstreamUsage").mockResolvedValue([]);
  client = renderOnboarding(
    { ...boundCandidate("active"), bound: false, bound_accounts: [], can_create_key: true },
    true,
    undefined,
    { history: "overview", realUpstreamsPage: true },
  );
  // Keep the pre-onboarding snapshot fresh until an explicit mutation invalidates it.
  client.setQueryDefaults(["upstream-group-binding-audit"], { staleTime: Infinity });
  client.setQueryData(["upstream-group-binding-audit"], {
    items: [],
    total_bindings: 0,
    present: 0,
    missing: 0,
    unknown: 0,
  });
  vi.mocked(api.upstreams).mockResolvedValue({
    hosts: [
      {
        ...upstream,
        hosts: [upstream.host],
        account_count: 1,
        group_count: 1,
        auth_status: "已鉴权",
        balance_status: "已读取",
        checked_at: null,
      },
    ],
    total_hosts: 1,
    authenticated_hosts: 1,
    recovery_required: 0,
    source: "Console",
  });

  const groups = await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" });
  fireEvent.click(groups);
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(groups, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: "预览添加账号" }));
  const confirmation = within(await screen.findByRole("dialog", { name: "确认账号绑定变更" }));
  fireEvent.click(confirmation.getByRole("button", { name: "确认提交 1 项变更" }));
  const taskDialog = within(await screen.findByRole("dialog", { name: "账号绑定变更" }));
  fireEvent.click(await taskDialog.findByRole("button", { name: "完成" }));
  fireEvent.click(screen.getByRole("button", { name: "返回统计变化" }));

  const history = within(await screen.findByRole("dialog", { name: "上游分组变化" }));
  expect(await history.findByText("已绑定")).toBeVisible();
  expect(history.queryByRole("button", { name: "向已有绑定分组添加账号" })).not.toBeInTheDocument();
});
