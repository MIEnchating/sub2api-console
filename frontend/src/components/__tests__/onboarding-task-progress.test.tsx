import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { OnboardingTaskProgress } from "../../App";
import type { Task } from "../../api";

function task(status: Task["status"], operation: string, result: Task["result"]): Task {
  return {
    id: "internal-task-id",
    skill: "sub2api-account-onboarding",
    operation,
    status,
    progress: status === "running" ? 55 : 100,
    message:
      status === "running"
        ? "正在添加 2/4：GPT Plus → codex"
        : status === "failed"
          ? "批量添加完成：成功 1 个，失败 1 个"
          : "批量添加完成：成功 2 个",
    result,
    created_at: "2026-08-29T00:00:00Z",
    updated_at: "2026-08-29T00:00:01Z",
  };
}

describe("onboarding task progress", () => {
  it("shows readable batch results without exposing raw result paths or task ids", () => {
    const markup = renderToStaticMarkup(
      <OnboardingTaskProgress
        task={task("succeeded", "onboard-batch", {
          total: 2,
          succeeded: 2,
          failed: 0,
          operation: "account.onboarding.batch",
          items: [
            { upstream_group: "CC Max | 企业专用", local_group: "A-CCMAX-1", status: "成功" },
            { upstream_group: "GPT Plus | 高速稳定", local_group: "codex", status: "成功" },
          ],
        })}
      />,
    );

    expect(markup).toContain("账号批量添加完成");
    expect(markup).toContain("计划添加");
    expect(markup).toContain("添加成功");
    expect(markup).toContain("CC Max | 企业专用");
    expect(markup).toContain("A-CCMAX-1");
    expect(markup).not.toContain("Items / 1 / Local Group");
    expect(markup).not.toContain("account.onboarding.batch");
    expect(markup).not.toContain("internal-task-id");
  });

  it("keeps partial failures next to the affected group", () => {
    const markup = renderToStaticMarkup(
      <OnboardingTaskProgress
        task={task("failed", "onboard-batch", {
          total: 2,
          succeeded: 1,
          failed: 1,
          items: [
            { upstream_group: "GPT Plus", local_group: "codex", status: "成功" },
            {
              upstream_group: "GPT Pro",
              local_group: "pro",
              status: "失败",
              error: "上游分组当前不可创建 Key",
            },
          ],
        })}
      />,
    );

    expect(markup).toContain("部分账号添加失败");
    expect(markup).toContain("部分成功");
    expect(markup).toContain("GPT Pro");
    expect(markup).toContain("上游分组当前不可创建 Key");
  });

  it("shows business progress without internal task metadata", () => {
    const markup = renderToStaticMarkup(
      <OnboardingTaskProgress task={task("running", "onboard-batch", {})} />,
    );

    expect(markup).toContain("正在批量添加账号");
    expect(markup).toContain("正在添加 2/4：GPT Plus → codex");
    expect(markup).toContain("55%");
    expect(markup).not.toContain("internal-task-id");
  });
});
