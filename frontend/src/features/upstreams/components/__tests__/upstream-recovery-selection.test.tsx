import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { BatchAuthTaskProgress } from "../../../../App";
import { UpstreamRecoverySelectionToolbar } from "../upstream-recovery-selection-toolbar";

describe("UpstreamRecoverySelectionToolbar", () => {
  it("shows the selected host count and an accessible recovery command", () => {
    const markup = renderToStaticMarkup(
      <UpstreamRecoverySelectionToolbar
        selectedCount={3}
        pending={false}
        onClear={vi.fn()}
        onRecover={vi.fn()}
      />,
    );

    expect(markup).toContain('role="toolbar"');
    expect(markup).toContain("已选择");
    expect(markup).toContain(">3<");
    expect(markup).toContain('aria-label="已选择 3 个上游的批量操作"');
    expect(markup).toContain('aria-label="清空选择"');
    expect(markup).toContain('aria-label="恢复已选择的 3 个上游鉴权"');
    expect(markup).toContain('aria-label="3 个已选择上游"');
    expect(markup).toContain("bottom-6");
    expect(markup).toContain("left-1/2");
    expect(markup).toContain("bg-background/95");
    expect(markup).toContain("backdrop-blur-lg");
  });

  it("does not render without a selected host", () => {
    const markup = renderToStaticMarkup(
      <UpstreamRecoverySelectionToolbar
        selectedCount={0}
        pending={false}
        onClear={vi.fn()}
        onRecover={vi.fn()}
      />,
    );

    expect(markup).toBe("");
  });

  it("shows successful methods and failed reasons for a completed batch", () => {
    const markup = renderToStaticMarkup(
      <BatchAuthTaskProgress
        task={{
          id: "auth-batch-1",
          skill: "sub2api-upstream-auth",
          operation: "recover-hosts",
          status: "failed",
          progress: 100,
          message: "鉴权恢复完成：成功 1，失败 1",
          result: {
            summary: { hosts: 2, recovered: 1, failed: 1 },
            outcomes: [
              {
                host: "one.example",
                success: true,
                auth_method: "sub2api_user_token",
                refresh_kind: "refresh_token",
              },
              { host: "two.example", success: false, reason: "密码箱项不可用" },
            ],
          },
          created_at: "2026-09-04T00:00:00Z",
          updated_at: "2026-09-04T00:00:01Z",
        }}
      />,
    );

    expect(markup).toContain("Token + 刷新 Token");
    expect(markup).toContain("刷新 Token");
    expect(markup).toContain("密码箱项不可用");
    expect(markup).toContain("已恢复");
    expect(markup).toContain("未恢复");
  });
});
