import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { GroupsPage, navItems } from "../../../App";
import type { GroupStatus } from "../../../api";

const groups: GroupStatus[] = [
  {
    name: "codex",
    id: "6",
    platform: "openai",
    platforms: ["openai"],
    account_count: 3,
    scheduling_open: 2,
    scheduling_closed: 1,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "partial_degraded",
    override: null,
  },
  {
    name: "pro",
    id: "8",
    account_count: 2,
    scheduling_open: 0,
    scheduling_closed: 2,
    scheduling_unknown: 0,
    strategy: "speed_first",
    strategy_source: "group_override",
    participation_status: "out_of_scope",
    participation_reason: "分组 ID 8 位于排除分组列表中",
    status: "excluded",
    override: {
      enabled: true,
      strategy: "speed_first",
    },
  },
  {
    name: "all-models",
    id: "10",
    platform: "composite",
    platforms: ["composite"],
    account_count: 4,
    scheduling_open: 4,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
    override: null,
  },
];

function renderGroupsPage() {
  const queryClient = new QueryClient();
  queryClient.setQueryData(["groups"], groups);
  return renderToStaticMarkup(
    <QueryClientProvider client={queryClient}>
      <GroupsPage />
    </QueryClientProvider>,
  );
}

describe("分组管理页面", () => {
  it("使用分组管理名称并以状态替代原因", () => {
    const markup = renderGroupsPage();

    expect(navItems.find((item) => item.id === "groups")?.label).toBe("分组管理");
    expect(markup).toContain("分组管理");
    expect(markup).toContain('aria-label="刷新分组"');
    expect(markup).toContain(">状态</");
    expect(markup).toContain(">平台</");
    expect(markup).not.toContain(">调度未知</th>");
    expect(markup).not.toContain(">状态未知</th>");
    expect(markup).not.toContain(">类型</th>");
    expect(markup).toContain(">OpenAI</span>");
    expect(markup).not.toContain(">openai</span>");
    expect(markup).toContain("Composite");
    expect(markup).toContain("部分异常");
    expect(markup).toContain("已排除");
    expect(markup).not.toContain(">原因</");
    expect(markup).not.toContain(">来源</");
    expect(markup).toContain("全局默认");
    expect(markup).toContain("排除");
    expect(markup).toContain("恢复管控");
    expect(markup).toContain("编辑");
    expect(markup).toContain("分组账号调度状态");
    expect(markup).toContain('aria-label="查看分组账号调度状态"');
    expect(markup).toContain('aria-label="排除分组"');
    expect(markup).toContain('aria-label="恢复管控"');
    expect(markup).toContain('aria-label="编辑分组"');
    expect(markup).toContain('aria-label="回落到全局策略"');
    expect(markup).not.toContain("分组 ID 8 位于排除分组列表中");
    expect(markup).toContain('data-table-panel=""');
  });
});
