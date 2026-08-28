import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { AccountMaintenanceTaskStatus } from "../../App";
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
