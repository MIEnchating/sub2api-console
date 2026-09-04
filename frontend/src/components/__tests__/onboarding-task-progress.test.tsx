import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { OnboardingTaskProgress } from "../../App";
import type { Task } from "../../api";

function task(status: Task["status"], operation: string, result: Task["result"]): Task {
  let message = "批量添加完成：成功 2 个";
  if (status === "running") message = "正在添加 2/4：GPT Plus → codex";
  else if (status === "failed") message = "批量添加完成：成功 1 个，失败 1 个";
  return {
    id: "internal-task-id",
    skill: "sub2api-account-onboarding",
    operation,
    status,
    progress: status === "running" ? 55 : 100,
    message,
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
            {
              upstream_group: "CC Max | 企业专用",
              local_group: "A-CCMAX-1",
              status: "成功",
            },
            {
              upstream_group: "GPT Plus | 高速稳定",
              local_group: "codex",
              status: "成功",
            },
          ],
        })}
      />,
    );

    expect(markup).toContain("账号批量绑定变更完成");
    expect(markup).toContain("计划变更");
    expect(markup).toContain("处理成功");
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
            {
              upstream_group: "GPT Plus",
              local_group: "codex",
              status: "成功",
            },
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

    expect(markup).toContain("部分账号绑定变更失败");
    expect(markup).toContain("部分成功");
    expect(markup).toContain("GPT Pro");
    expect(markup).toContain("上游分组当前不可创建 Key");
  });

  it("shows business progress without internal task metadata", () => {
    const markup = renderToStaticMarkup(
      <OnboardingTaskProgress task={task("running", "onboard-batch", {})} />,
    );

    expect(markup).toContain("正在批量处理账号绑定");
    expect(markup).toContain("正在添加 2/4：GPT Plus → codex");
    expect(markup).toContain("55%");
    expect(markup).not.toContain("internal-task-id");
  });

  it("paginates long batch results", () => {
    const items = Array.from({ length: 25 }, (_, index) => ({
      upstream_group: `上游分组-${index + 1}`,
      local_group: `本地分组-${index + 1}`,
      status: "成功",
    }));
    const markup = renderToStaticMarkup(
      <OnboardingTaskProgress
        task={task("succeeded", "onboard-batch", {
          total: items.length,
          succeeded: items.length,
          failed: 0,
          items,
        })}
      />,
    );

    expect(markup).toContain("上游分组-20");
    expect(markup).not.toContain("上游分组-21");
    expect(markup).toContain("转到第 2 页");
    expect(markup).toContain("flex h-full min-h-0 flex-col");
    expect(markup).toContain("min-h-0 flex-1 overflow-auto");
    expect(markup).toContain('data-table-panel=""');
  });
});
