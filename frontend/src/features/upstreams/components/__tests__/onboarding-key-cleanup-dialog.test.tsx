import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { Task } from "@/api";
import { Dialog } from "@/components/ui/dialog";
import { dialogContentClass } from "@/components/ui/dialog";
import {
  keyCleanupDialogLayout,
  keyCleanupDialogWidth,
  keyCleanupResultItems,
  OnboardingKeyCleanupDialogContent,
} from "../onboarding-key-cleanup-dialog";

const completedTask: Task = {
  id: "cleanup-1",
  skill: "sub2api-account-onboarding",
  operation: "upstream-key-cleanup",
  status: "succeeded",
  progress: 100,
  message: "无绑定 Key 清理完成：删除 1 个，跳过 1 个",
  result: {
    deleted: 1,
    skipped: 1,
    failed: 0,
    items: [
      { key_id: "17", name: "unused", status: "deleted" },
      { key_id: "18", name: "newly-bound", status: "skipped", reason: "已建立绑定" },
    ],
  },
  created_at: "2026-08-31T00:00:00Z",
  updated_at: "2026-08-31T00:00:01Z",
};

describe("OnboardingKeyCleanupDialog", () => {
  it("shows every unbound key before destructive cleanup", () => {
    const markup = renderToStaticMarkup(
      <Dialog open>
        <OnboardingKeyCleanupDialogContent
          open
          preview={{
            host: "api.example",
            keys: [{ key_id: "17", name: "unused", group_id: "6", status: "active" }],
          }}
          previewPending={false}
          previewError={null}
          task={null}
          taskPending={false}
          taskError={null}
          onOpenChange={() => undefined}
          onRefresh={() => undefined}
          onConfirm={() => undefined}
          onComplete={() => undefined}
        />
      </Dialog>,
    );

    expect(markup).toContain("清理无绑定上游 Key");
    expect(markup).toContain("unused");
    expect(markup).toContain("Key ID");
    expect(markup).toContain("确认删除 1 个 Key");
    expect(markup).toContain('data-table-panel=""');
  });

  it("parses deleted and protected-at-execution results", () => {
    expect(keyCleanupResultItems(completedTask)).toEqual([
      { keyId: "17", name: "unused", status: "deleted", reason: null },
      { keyId: "18", name: "newly-bound", status: "skipped", reason: "已建立绑定" },
    ]);
  });

  it("uses a compact adaptive dialog when the scan has no rows", () => {
    expect(keyCleanupDialogWidth({ host: "api.example", keys: [] }, null)).toBe("medium");
    expect(
      keyCleanupDialogWidth(
        {
          host: "api.example",
          keys: [{ key_id: "17", name: "unused", group_id: "6", status: "active" }],
        },
        null,
      ),
    ).toBe("wide");
    const className = dialogContentClass(
      "medium",
      keyCleanupDialogLayout.height,
      keyCleanupDialogLayout.content,
    );
    expect(className).toContain("max-h-[calc(100svh-2rem)]");
    expect(className).not.toContain("h-[min(42rem");
  });

  it("removes the destructive zero-count action from the empty state", () => {
    const markup = renderToStaticMarkup(
      <Dialog open>
        <OnboardingKeyCleanupDialogContent
          open
          preview={{ host: "api.example", keys: [] }}
          previewPending={false}
          previewError={null}
          task={null}
          taskPending={false}
          taskError={null}
          onOpenChange={() => undefined}
          onRefresh={() => undefined}
          onConfirm={() => undefined}
          onComplete={() => undefined}
        />
      </Dialog>,
    );

    expect(markup).toContain("关闭");
    expect(markup).toContain('aria-label="刷新扫描结果"');
    expect(markup).not.toContain("确认删除 0 个 Key");
  });
});
