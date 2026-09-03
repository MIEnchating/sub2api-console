import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AccountBatchDeleteTaskStatus } from "../../../../App";
import type { Task } from "../../../../api";

function batchTask(status: Task["status"]): Task {
  return {
    id: "delete-batch",
    skill: "sub2api-account-management",
    operation: "account-delete-batch",
    status,
    progress: 100,
    message: "账号批量删除完成：成功 1 个，失败 1 个",
    result: {
      requested: 2,
      succeeded: 1,
      failed: 1,
      items: [
        {
          account_id: "37",
          account_name: "重复账号 A",
          status: "succeeded",
          upstream_key_deleted: true,
          management_account_deleted: true,
          local_projection_deleted: true,
        },
        {
          account_id: "38",
          account_name: "重复账号 B",
          status: "failed",
          upstream_key_deleted: false,
          management_account_deleted: false,
          local_projection_deleted: false,
          error: "账号处于人工优先位，删除前请先解除人工管控",
        },
      ],
    },
    created_at: "2026-09-02T00:00:00Z",
    updated_at: "2026-09-02T00:01:00Z",
  };
}

describe("AccountBatchDeleteTaskStatus", () => {
  it("shows each successful and failed deletion without hiding partial completion", () => {
    const markup = renderToStaticMarkup(
      <AccountBatchDeleteTaskStatus task={batchTask("partial")} />,
    );

    expect(markup).toContain("计划删除");
    expect(markup).toContain("删除成功");
    expect(markup).toContain("删除失败");
    expect(markup).toContain("重复账号 A");
    expect(markup).toContain("重复账号 B");
    expect(markup).toContain("账号处于人工优先位");
    expect(markup).toContain("上游 Key 已删除");
    expect(markup).toContain("本地记录未清理");
  });

  it("shows determinate progress while the batch task is running", () => {
    const task = batchTask("running");
    task.progress = 50;
    task.message = "批量删除进度：已处理 1/2 个账号";

    const markup = renderToStaticMarkup(<AccountBatchDeleteTaskStatus task={task} />);

    expect(markup).toContain("批量删除进度：已处理 1/2 个账号");
    expect(markup).toContain('aria-valuenow="50"');
  });
});
