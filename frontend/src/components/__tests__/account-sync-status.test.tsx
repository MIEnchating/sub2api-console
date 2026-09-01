import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { AccountSyncTaskStatus, BalanceTaskProgress } from "../../App";
import type { Task } from "../../api";

function failedTask(operation: Task["operation"], remoteWrite: boolean): Task {
  return {
    id: "account-sync-task",
    skill: "sub2api-account-sync",
    operation,
    status: "failed",
    progress: 100,
    message: "账号同步失败",
    result: {
      error: "Sub2API 读回结果不一致",
      remote_write: remoteWrite,
    },
    created_at: "2026-08-27T00:00:00Z",
    updated_at: "2026-08-27T00:00:01Z",
  };
}

describe("account synchronization status", () => {
  it("reads a single-host balance from the upstream sync batch result", () => {
    const task: Task = {
      id: "balance-sync-task",
      skill: "sub2api-upstream-info",
      operation: "balance-sync",
      status: "succeeded",
      progress: 100,
      message: "上游同步完成：成功 1",
      result: {
        succeeded: 1,
        failed: 0,
        hosts: [
          {
            host: "api.example.test",
            status: "succeeded",
            auth_status: "已鉴权",
            balance_status: "已读取",
            balance: "199.62365227",
          },
        ],
      },
      created_at: "2026-08-31T10:12:03Z",
      updated_at: "2026-08-31T10:12:04Z",
    };

    const markup = renderToStaticMarkup(<BalanceTaskProgress task={task} />);

    expect(markup).toContain("同步成功");
    expect(markup).toContain("api.example.test");
    expect(markup).toContain("$199.6237");
    expect(markup).not.toContain("上游未返回余额");
  });

  it("shows the failed account, reason, and completed remote write for field sync", () => {
    const markup = renderToStaticMarkup(
      <AccountSyncTaskStatus
        task={failedTask("account-fields-sync", true)}
        accountId="139"
        onClose={vi.fn()}
      />,
    );

    expect(markup).toContain("账号字段同步失败");
    expect(markup).toContain("账号 ID 139");
    expect(markup).toContain("Sub2API 读回结果不一致");
    expect(markup).toContain("已写入，后续读回或本地提交失败");
    expect(markup).not.toContain("remote_write");
  });

  it("shows that no remote write occurred when group sync fails before writing", () => {
    const markup = renderToStaticMarkup(
      <AccountSyncTaskStatus
        task={failedTask("account-groups-sync", false)}
        accountId="63"
        onClose={vi.fn()}
      />,
    );

    expect(markup).toContain("账号分组同步失败");
    expect(markup).toContain("账号 ID 63");
    expect(markup).toContain("未写入");
  });
});
