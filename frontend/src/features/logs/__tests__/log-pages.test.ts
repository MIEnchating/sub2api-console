import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { UnifiedLogEntry } from "../../../api";
import { navItems, viewForPath } from "../../../App";
import { LogKindFilter, LogChangesTable, LogsFilterToolbar } from "../components/logs-center-page";
import {
  formatLogValue,
  logDetailRows,
  logKindLabel,
  logStatusLabel,
  logTitleLabel,
  relatedChanges,
  relatedEvents,
} from "../lib/log-display";

const entry: UnifiedLogEntry = {
  id: "run:inspection-1",
  kind: "task",
  occurred_at: "2026-08-26T00:00:00Z",
  title: "inspection-run",
  summary: "巡检完成",
  status: "succeeded",
  actor: null,
  object_label: null,
  source: "run_record",
  source_id: "inspection-1",
  related_count: 2,
  details: {
    progress: 100,
    duration_seconds: "1.5",
    events: [
      {
        id: "event:1",
        kind: "event",
        occurred_at: "2026-08-26T00:00:00Z",
        title: "alerts.evaluate",
        summary: "告警检测完成",
        status: "succeeded",
        actor: null,
        object_label: null,
        source: "runtime_event",
        source_id: "1",
        related_count: 0,
        details: {},
      },
    ],
    changes: [
      {
        id: "change:inspection-1",
        kind: "change",
        details: {
          changes: [
            {
              id: 2,
              created_at: "2026-08-26T00:00:00Z",
              object_name: "demo",
              object_id: "41",
              group_names: ["grok"],
              operation_type: "routing.writeback",
              remote_confirmed: true,
              readback_confirmed: true,
              writeback: true,
              field_name: "load_factor,priority",
              before: { priority: 100, load_factor: "17", concurrency: 8 },
              after: { priority: 20, load_factor: "4", concurrency: 8 },
              state: "succeeded",
            },
          ],
        },
      },
    ],
  },
};

describe("log center contracts", () => {
  it("exposes exactly one current log-center navigation entry", () => {
    expect(navItems.filter((item) => item.id === "logs")).toHaveLength(1);
    expect(navItems.find((item) => item.id === "logs")?.to).toBe("/logs");
    expect(navItems.some((item) => ["runs", "events", "writebacks"].includes(item.id))).toBe(false);
    expect(viewForPath("/logs")).toBe("logs");
  });

  it("renders record types as compact tabs matching the revenue analysis view switcher", () => {
    const markup = renderToStaticMarkup(
      createElement(LogKindFilter, {
        value: "event",
        onChange: () => undefined,
      }),
    );

    expect(markup).toContain('role="tablist"');
    expect(markup).toContain('aria-label="记录类型"');
    expect(markup.match(/role="tab"/g)).toHaveLength(4);
    expect(markup).toContain("全部记录");
    expect(markup).toContain("任务记录");
    expect(markup).toContain("事件日志");
    expect(markup).toContain("远程读写");
    expect(markup).toMatch(/role="tab"[^>]*aria-selected="true"[^>]*>事件日志<\/button>/);
    expect(markup).toContain('data-slot="segmented-control"');
    expect(markup).toContain("bg-background shadow-xs");
    expect(markup).not.toContain('data-slot="select-trigger"');
  });

  it("places filters and refresh in one toolbar without a duplicate total", () => {
    const markup = renderToStaticMarkup(
      createElement(LogsFilterToolbar, {
        search: "",
        kind: "all",
        state: "all",
        eventLevel: "all",
        eventGroup: "all",
        groups: [],
        fetching: false,
        truncated: false,
        onSearchChange: () => undefined,
        onKindChange: () => undefined,
        onStateChange: () => undefined,
        onEventLevelChange: () => undefined,
        onEventGroupChange: () => undefined,
        onRefresh: () => undefined,
      }),
    );

    expect(markup).toContain('data-testid="logs-filter-toolbar"');
    expect(markup).toContain('aria-label="日志筛选"');
    expect(markup).toContain('aria-label="搜索任务、对象或原因"');
    expect(markup).toContain('aria-label="记录类型"');
    expect(markup).toContain('aria-label="执行结果"');
    expect(markup).toContain('aria-label="刷新日志"');
    expect(markup.match(/data-slot="select-trigger"/g)).toHaveLength(1);
    expect(markup.indexOf('aria-label="记录类型"')).toBeLessThan(
      markup.indexOf('aria-label="搜索任务、对象或原因"'),
    );
    expect(markup).not.toContain("条");
  });

  it("shows event level and group filters for the event log", () => {
    const markup = renderToStaticMarkup(
      createElement(LogsFilterToolbar, {
        search: "",
        kind: "event",
        state: "all",
        eventLevel: "warning",
        eventGroup: "codex",
        groups: [{ name: "codex", id: "7", account_count: 2 } as never],
        fetching: false,
        truncated: false,
        onSearchChange: () => undefined,
        onKindChange: () => undefined,
        onStateChange: () => undefined,
        onEventLevelChange: () => undefined,
        onEventGroupChange: () => undefined,
        onRefresh: () => undefined,
      }),
    );

    expect(markup).toContain('aria-label="事件级别"');
    expect(markup).toContain('aria-label="事件分组"');
    expect(markup).toContain("警告");
    expect(markup).toContain("codex");
    expect(markup).not.toContain('aria-label="执行结果"');
  });

  it("uses readable names and states instead of internal identifiers", () => {
    expect(logKindLabel("change")).toBe("远程读写");
    expect(logTitleLabel("inspection-run")).toBe("巡检任务");
    expect(logTitleLabel("upstream.sync")).toBe("上游同步");
    expect(logTitleLabel("automatic-inspection")).toBe("自动巡检");
    expect(logStatusLabel("waiting_input")).toBe("等待输入");
  });

  it("formats structured details and associated records", () => {
    expect(logDetailRows(entry.details)).toEqual([
      { key: "progress", label: "执行进度", value: "100%" },
      { key: "duration_seconds", label: "耗时", value: "1.5 秒" },
    ]);
    expect(relatedEvents(entry)).toHaveLength(1);
    expect(relatedChanges(entry)).toEqual([
      {
        id: "2",
        object: "demo",
        objectId: "41",
        occurredAt: "2026-08-26T00:00:00Z",
        groups: ["grok"],
        operation: "更新账号",
        change: "负载因子：17 → 4；优先级：100 → 20",
        status: "succeeded",
        result: "成功",
      },
    ]);
    expect(formatLogValue({ priority: 100, schedulable: false })).toBe(
      "优先级：100；调度状态：停用",
    );
    expect(
      formatLogValue({
        key_count: 3,
        group_count: 2,
        auth_recovered: true,
      }),
    ).toBe("密钥数量：3；分组数量：2；鉴权已恢复：是");
    expect(
      logDetailRows({
        operation_type: "routing.writeback",
        phase: "readback",
        request_id: null,
        source: "console",
        event_type: "upstream.sync",
      }),
    ).toEqual([
      { key: "operation_type", label: "操作类型", value: "自动调度写回" },
      { key: "phase", label: "执行阶段", value: "读取复核" },
      { key: "source", label: "数据来源", value: "控制台" },
      { key: "event_type", label: "事件类型", value: "上游同步" },
    ]);
    expect(logDetailRows({ operation_type: "account.delete" })).toEqual([
      { key: "operation_type", label: "操作类型", value: "手动删除账号" },
    ]);
  });

  it("renders remote audits with the complete comparison columns", () => {
    const changes = relatedChanges(entry);
    const markup = renderToStaticMarkup(createElement(LogChangesTable, { changes }));

    for (const heading of ["时间", "渠道", "分组", "操作", "变更", "结果"]) {
      expect(markup).toContain(`>${heading}<`);
    }
    expect(markup).toContain("demo");
    expect(markup).toContain("#41");
    expect(markup).toContain("grok");
    expect(markup).toContain("更新账号");
    expect(markup).toContain("负载因子：17 → 4；优先级：100 → 20");
    expect(markup).toContain("whitespace-nowrap");
    expect(markup).toContain('data-table-panel=""');
    expect(markup).not.toContain("update_account");
    expect(markup).not.toContain("load_factor");
    expect(markup).not.toContain("concurrency");
  });

  it("describes boolean scheduling changes in plain language", () => {
    const changeEntry: UnifiedLogEntry = {
      ...entry,
      kind: "change",
      details: {
        changes: [
          {
            id: 3,
            operation_type: "routing.writeback",
            field_name: "schedulable",
            before: { schedulable: false },
            after: { schedulable: true },
            writeback: true,
            state: "succeeded",
          },
        ],
      },
    };

    expect(relatedChanges(changeEntry)[0]?.change).toBe("调度状态：停用 → 启用");
  });
});
