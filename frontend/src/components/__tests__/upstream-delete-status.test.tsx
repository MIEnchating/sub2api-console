import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { UpstreamDeleteTaskStatus } from "../../App";
import type { Task } from "../../api";

function task(status: Task["status"], result: Task["result"]): Task {
  return {
    id: "delete-task",
    skill: "sub2api-upstream-info",
    operation: "upstream-delete",
    status,
    progress: status === "running" ? 50 : 100,
    message: status === "failed" ? "上游删除失败" : "上游及关联账号已删除",
    result,
    created_at: "2026-08-25T00:00:00Z",
    updated_at: "2026-08-25T00:00:01Z",
  };
}

describe("upstream deletion status", () => {
  it("summarizes deleted account and group counts on success", () => {
    const markup = renderToStaticMarkup(
      <UpstreamDeleteTaskStatus
        task={task("succeeded", { deleted_accounts: 2, deleted_groups: 3 })}
      />,
    );

    expect(markup).toContain("删除完成");
    expect(markup).toContain("删除账号");
    expect(markup).toContain("2 个");
    expect(markup).toContain("清理分组");
    expect(markup).toContain("3 个");
  });

  it("makes partial remote deletion explicit when the task fails", () => {
    const markup = renderToStaticMarkup(
      <UpstreamDeleteTaskStatus
        task={task("failed", {
          reason: "第二个账号删除失败",
          remote_deleted_accounts: 1,
        })}
      />,
    );

    expect(markup).toContain("删除失败");
    expect(markup).toContain("第二个账号删除失败");
    expect(markup).toContain("Sub2API 已删除 1 个账号");
    expect(markup).toContain("本地数据尚未清理");
  });
});
