import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { AccountMaintenanceTaskStatus, AccountRateSyncTaskStatus } from "../../App";
import type { Task } from "../../api";

function maintenanceTask(items: Array<Record<string, string>>): Task {
  return {
    id: "account-maintenance-task",
    skill: "sub2api-operations",
    operation: "account-binding-revalidation",
    status: "succeeded",
    progress: 100,
    message: "账号批量复验完成",
    result: {
      bound: items.length,
      verified: items.filter((item) => item.status === "已确认存在").length,
      missing: items.filter((item) => item.status === "管理平台不存在").length,
      items,
    },
    created_at: "2026-08-28T00:00:00Z",
    updated_at: "2026-08-28T00:00:01Z",
  };
}

describe("account maintenance status", () => {
  it("offers cleanup and shows account identity when a stable ID is missing remotely", () => {
    const markup = renderToStaticMarkup(
      <AccountMaintenanceTaskStatus
        task={maintenanceTask([
          {
            account_id: "744",
            account_name: "号池-0.08",
            upstream_host: "api.anc1ent.top",
            status: "管理平台不存在",
          },
        ])}
        onCleanupMissing={vi.fn()}
      />,
    );

    expect(markup).toContain("号池-0.08");
    expect(markup).toContain("ID 744");
    expect(markup).toContain("api.anc1ent.top");
    expect(markup).toContain('href="https://api.anc1ent.top"');
    expect(markup).toContain('target="_blank"');
    expect(markup).toContain("修复 1 个失效绑定");
  });

  it("does not offer cleanup when every stable ID still exists remotely", () => {
    const markup = renderToStaticMarkup(
      <AccountMaintenanceTaskStatus
        task={maintenanceTask([
          {
            account_id: "744",
            account_name: "Anc1ent API-0.08",
            upstream_host: "api.anc1ent.top",
            status: "已确认存在",
          },
        ])}
        onCleanupMissing={vi.fn()}
      />,
    );

    expect(markup).not.toContain("失效绑定");
  });
});

describe("account rate sync status", () => {
  it("shows per-account before and after values with concrete partial failures", () => {
    const task: Task = {
      id: "account-rate-task",
      skill: "sub2api-operations",
      operation: "account-rate-sync",
      status: "failed",
      progress: 100,
      message: "账号倍率同步部分失败",
      result: {
        updated: 1,
        unchanged: 0,
        missing: 1,
        failed: 1,
        items: [
          {
            account_id: "11",
            account_name: "alpha",
            status: "已同步",
            before: "0.1",
            after: "0.75",
            name_before: "Example-0.1",
            name_after: "Example-0.75",
          },
          {
            account_id: "12",
            account_name: "beta",
            status: "管理平台不存在",
            before: "0.2",
          },
          {
            account_id: "13",
            account_name: "gamma",
            status: "同步失败",
            before: "0.3",
            error: "管理平台账号倍率必须大于 0",
          },
        ],
      },
      created_at: "2026-08-29T00:00:00Z",
      updated_at: "2026-08-29T00:00:01Z",
    };

    const markup = renderToStaticMarkup(<AccountRateSyncTaskStatus task={task} />);

    expect(markup).toContain("alpha");
    expect(markup).toContain("0.1 → 0.75");
    expect(markup).toContain("名称 Example-0.1 → Example-0.75");
    expect(markup).toContain("管理平台不存在");
    expect(markup).toContain("管理平台账号倍率必须大于 0");
    expect(markup).not.toContain("account-rate-task");
  });
});
