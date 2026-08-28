import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { AccountSyncTaskStatus } from "../../App";
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
