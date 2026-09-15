import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AutoInspectionHeartbeatDetails } from "@/App";
import type { AutoInspectionStatus, Task } from "@/api";

const record: AutoInspectionStatus["heartbeat_history"][number] = {
  checked_at: "2026-09-14T10:28:40Z",
  completed_at: "2026-09-14T10:30:44Z",
  status: "partial",
  operations: ["upstream_sync", "auth_recovery"],
  operation_timings: [],
  task_id: "inspection-auth-failures",
  error: "鉴权自动恢复部分失败：成功 0，失败 1",
  skipped: false,
};

function task(result: Record<string, unknown>): Task {
  return {
    id: record.task_id!,
    skill: "sub2api-auto-inspection",
    operation: "automatic-inspection",
    status: "partial",
    progress: 100,
    message: "巡检完成，但存在部分失败",
    result,
    created_at: record.checked_at,
    updated_at: record.completed_at!,
  };
}

const syncFailure = {
  hosts: [
    { host: "challenge.example.test", status: "auth_failed", reason: "refresh token 已失效" },
  ],
};

describe("巡检鉴权恢复失败明细", () => {
  it("鉴权错误码与原型属性同名时保留失败原因且不渲染无效恢复指引", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({
          auth_recovery: {
            failed: 1,
            results: [
              {
                host: "challenge.example.test",
                success: false,
                code: "__proto__",
                reason: "授权服务返回未识别的错误码",
              },
            ],
          },
        })}
      />,
    );
    expect(within(screen.getByRole("alert")).getByText("授权服务返回未识别的错误码")).toBeVisible();
  });

  it("巡检操作名与原型属性同名时按原始操作名展示步骤", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, operations: ["__proto__"], error: null }}
        task={task({ planned_operations: [{ operation: "__proto__" }] })}
      />,
    );
    expect(screen.getAllByText("__proto__").length).toBeGreaterThan(0);
  });

  it("续签失败后密码箱登录触发人机验证时显示最终原因和人工恢复指引", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({
          upstream_sync: syncFailure,
          auth_recovery: {
            failed: 1,
            results: [
              {
                host: "challenge.example.test",
                success: false,
                code: "browser_challenge_required",
                reason: "登录触发浏览器人机验证",
              },
            ],
          },
        })}
      />,
    );
    const failures = screen.getByRole("alert");
    expect(within(failures).getByText("登录触发浏览器人机验证")).toBeVisible();
    expect(within(failures).getByText(/恢复鉴权.*Token.*刷新 Token/)).toBeVisible();
    expect(within(failures).queryByText(/打开浏览器手动验证/)).not.toBeInTheDocument();
    expect(within(failures).queryByText("refresh token 已失效")).not.toBeInTheDocument();
  });

  it("本轮没有上游同步但鉴权自动恢复失败时仍显示失败 Host", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({
          auth_recovery: {
            failed: 1,
            results: [{ host: "challenge.example.test", success: false, reason: "未选择密码箱项" }],
          },
        })}
      />,
    );
    const failures = screen.getByRole("alert");
    expect(within(failures).getByText("challenge.example.test")).toBeVisible();
    expect(within(failures).getByText("未选择密码箱项")).toBeVisible();
  });

  it("鉴权已恢复成功时不再把更早的续签失败列为待处理项", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, error: "自动执行部分失败：1 项" }}
        task={task({
          upstream_sync: syncFailure,
          auth_recovery: {
            recovered: 1,
            results: [{ host: "challenge.example.test", success: true }],
          },
        })}
      />,
    );
    expect(
      within(screen.getByRole("alert")).queryByText("challenge.example.test"),
    ).not.toBeInTheDocument();
  });

  it("旧任务未记录鉴权恢复明细时保留原始同步失败原因", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({ upstream_sync: syncFailure })}
      />,
    );
    expect(within(screen.getByRole("alert")).getByText("refresh token 已失效")).toBeVisible();
  });

  it("鉴权恢复成功但目录同步另有失败时保留未解决的同步错误", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({
          upstream_sync: {
            hosts: [{ host: "challenge.example.test", status: "failed", reason: "目录响应不完整" }],
          },
          auth_recovery: {
            recovered: 1,
            results: [{ host: "challenge.example.test", success: true }],
          },
        })}
      />,
    );
    expect(within(screen.getByRole("alert")).getByText("目录响应不完整")).toBeVisible();
  });
});
