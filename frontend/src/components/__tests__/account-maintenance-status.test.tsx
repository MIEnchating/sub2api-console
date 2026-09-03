import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import {
  AccountDefaultsRepairTaskStatus,
  AccountMaintenanceTaskStatus,
  AccountRateSyncTaskStatus,
} from "../../App";
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
  it("shows default parameter repair counts and readable before and after values", () => {
    const task = maintenanceTask([
      {
        account_id: "11",
        account_name: "account-11",
        upstream_host: "api.example",
        status: "已修复",
        before: "并发 0 · 负载 跟随并发（有效 0）· 优先级 0",
        after: "并发 10 · 负载 跟随并发（有效 10）· 优先级 1",
      },
      {
        account_id: "12",
        account_name: "external",
        upstream_host: "api.example",
        status: "非本控制台添加，未修改",
      },
    ]);
    task.result = {
      ...task.result,
      repaired: 1,
      unchanged: 0,
      skipped: 1,
      failed: 0,
    };

    const markup = renderToStaticMarkup(<AccountDefaultsRepairTaskStatus task={task} />);

    expect(markup).toContain("已修复");
    expect(markup).toContain("无需修复");
    expect(markup).toContain("已跳过");
    expect(markup).toContain("并发 0");
    expect(markup).toContain("并发 10");
    expect(markup).toContain("非本控制台添加，未修改");
    expect(markup).toContain('data-table-panel=""');
  });

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

  it("paginates long maintenance results", () => {
    const markup = renderToStaticMarkup(
      <AccountMaintenanceTaskStatus
        task={maintenanceTask(
          Array.from({ length: 25 }, (_, index) => ({
            account_id: String(index + 1),
            account_name: `分页账号-${index + 1}`,
            upstream_host: "api.example",
            status: "已确认存在",
          })),
        )}
      />,
    );

    expect(markup).toContain("分页账号-20");
    expect(markup).not.toContain("分页账号-21");
    expect(markup).toContain("转到第 2 页");
    expect(markup).toContain("flex h-full min-h-0 flex-col");
    expect(markup).toContain("min-h-0 flex-1 divide-y overflow-y-auto");
    expect(markup).toContain('data-table-panel=""');
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
            upstream_raw_multiplier: "1.5",
            recharge_rate: "2",
            account_multiplier: "0.75",
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
    expect(markup).toContain("上游原始倍率 1.5 ÷ 充值比例 2 = 账号成本 0.75");
    expect(markup).toContain("名称 Example-0.1 → Example-0.75");
    expect(markup).toContain("管理平台不存在");
    expect(markup).toContain("管理平台账号倍率必须大于 0");
    expect(markup).not.toContain("account-rate-task");
    expect(markup).toContain('data-table-panel=""');
  });

  it("shows skipped count and the live probe error for read-only fallback", () => {
    const task: Task = {
      id: "account-rate-fallback",
      skill: "sub2api-operations",
      operation: "account-rate-sync",
      status: "succeeded",
      progress: 100,
      message: "账号倍率同步完成",
      result: {
        updated: 0,
        unchanged: 0,
        skipped: 1,
        missing: 0,
        failed: 0,
        items: [
          {
            account_id: "25",
            account_name: "Pixel API-0.25",
            status: "只读降级，已跳过写回",
            probe_error: "上游倍率接口暂时不可用",
            upstream_raw_multiplier: "0.25",
            recharge_rate: "1",
            account_multiplier: "0.25",
          },
        ],
      },
      created_at: "2026-09-02T00:00:00Z",
      updated_at: "2026-09-02T00:00:01Z",
    };

    const markup = renderToStaticMarkup(<AccountRateSyncTaskStatus task={task} />);

    expect(markup).toContain("已跳过");
    expect(markup).toContain("1 个");
    expect(markup).toContain("只读降级，已跳过写回");
    expect(markup).toContain("实时探测失败：上游倍率接口暂时不可用");
  });
});
