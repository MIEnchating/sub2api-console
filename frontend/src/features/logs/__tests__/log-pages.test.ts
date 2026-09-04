import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { UnifiedLogEntry } from "../../../api";
import { navItems, viewForPath } from "../../../App";
import {
  LogChangesList,
  LogDetailsContent,
  LogStructuredValue,
  logDetailsDialogWidth,
} from "../components/log-details-dialog";
import { LogKindFilter, LogsFilterToolbar } from "../components/logs-center-page";
import {
  formatLogValue,
  logDetailLabel,
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
        truncated: false,
        onSearchChange: () => undefined,
        onKindChange: () => undefined,
        onStateChange: () => undefined,
        onEventLevelChange: () => undefined,
        onEventGroupChange: () => undefined,
      }),
    );

    expect(markup).toContain('data-testid="logs-filter-toolbar"');
    expect(markup).toContain('aria-label="日志筛选"');
    expect(markup).toContain('aria-label="搜索任务、对象或原因"');
    expect(markup).toContain('aria-label="记录类型"');
    expect(markup).toContain('aria-label="执行结果筛选"');
    expect(markup).not.toContain('aria-label="刷新日志"');
    expect(markup).not.toContain('data-slot="select-trigger"');
    expect(markup.indexOf('aria-label="搜索任务、对象或原因"')).toBeLessThan(
      markup.indexOf('aria-label="记录类型"'),
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
        truncated: false,
        onSearchChange: () => undefined,
        onKindChange: () => undefined,
        onStateChange: () => undefined,
        onEventLevelChange: () => undefined,
        onEventGroupChange: () => undefined,
      }),
    );

    expect(markup).toContain('aria-label="事件级别筛选"');
    expect(markup).toContain('aria-label="事件分组筛选"');
    expect(markup).toContain("警告");
    expect(markup).toContain("codex");
    expect(markup).not.toContain('aria-label="执行结果筛选"');
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

  it("renders remote audits as compact records that remain readable on narrow screens", () => {
    const changes = relatedChanges(entry);
    const markup = renderToStaticMarkup(createElement(LogChangesList, { changes }));

    expect(markup).toContain('data-slot="log-change-list"');
    expect(markup).toContain("demo");
    expect(markup).toContain("#41");
    expect(markup).toContain("grok");
    expect(markup).toContain("更新账号");
    expect(markup).toContain("负载因子：17 → 4；优先级：100 → 20");
    expect(markup).toContain("成功");
    expect(markup).not.toContain("min-w-[72rem]");
    expect(markup).not.toContain('data-slot="table"');
    expect(markup).not.toContain("update_account");
    expect(markup).not.toContain("load_factor");
    expect(markup).not.toContain("concurrency");
  });

  it("uses compact type-specific detail sections for every log kind", () => {
    const taskMarkup = renderToStaticMarkup(createElement(LogDetailsContent, { entry }));
    const eventEntry = relatedEvents(entry)[0] as UnifiedLogEntry;
    const eventMarkup = renderToStaticMarkup(
      createElement(LogDetailsContent, { entry: eventEntry }),
    );
    const changeEntry: UnifiedLogEntry = { ...entry, kind: "change", source: "operation_audit" };
    const changeMarkup = renderToStaticMarkup(
      createElement(LogDetailsContent, { entry: changeEntry }),
    );

    expect(taskMarkup).toContain("任务信息");
    expect(taskMarkup).toContain("关联事件");
    expect(taskMarkup).toContain("关联远程读写");
    expect(eventMarkup).toContain("事件信息");
    expect(eventMarkup).toContain("事件级别");
    expect(eventMarkup).toContain("信息");
    expect(changeMarkup).toContain("操作信息");
    expect(changeMarkup).toContain('data-slot="log-change-list"');
    expect(logDetailsDialogWidth("task")).toBe("wide");
    expect(logDetailsDialogWidth("event")).toBe("medium");
    expect(logDetailsDialogWidth("change")).toBe("wide");
  });

  it("renders nested task result items as summary metrics and separate records", () => {
    const result = {
      updated: 0,
      failed: 0,
      fallback: 13,
      items: [
        {
          account_id: "14",
          account_name: "mdkj-0.1",
          upstream_host: "mdkj.lol",
          before: "0.1",
          after: "0.1",
          observation_source: "live",
          readback_confirmed: false,
          status: "已确认一致",
        },
        {
          account_id: "25",
          account_name: "Pixel API-0.25",
          upstream_host: "speed.ai-pixel.online",
          probe_error: "上游倍率探测失败",
          status: "只读降级",
        },
      ],
    };
    const markup = renderToStaticMarkup(createElement(LogStructuredValue, { value: result }));

    expect(markup).toContain('data-slot="log-result-summary"');
    expect(markup).toContain('data-slot="log-result-items"');
    expect(markup).toContain("降级读取");
    expect(markup).toContain("mdkj-0.1");
    expect(markup).toContain("Pixel API-0.25");
    expect(markup).toContain("变更前");
    expect(markup).toContain("变更后");
    expect(markup).toContain("上游倍率探测失败");
    expect(markup).not.toContain("items：");
    expect(logDetailsDialogWidth("task", { result })).toBe("wide");
  });

  it("translates alert delivery fields and source values without leaking dictionary keys", () => {
    const alertResult = {
      evaluation_disabled: false,
      findings: 22,
      remote_write: false,
      source: "console-domain-db",
      delivery: {
        attempted: 0,
        batches: 0,
        configured: true,
        dry_run: false,
        failed: 0,
        sent: 0,
        skipped: 51,
        suppressed: 498,
      },
    };
    const markup = renderToStaticMarkup(createElement(LogStructuredValue, { value: alertResult }));

    for (const label of [
      "尝试发送",
      "投递批次",
      "通知已配置",
      "试运行",
      "发送成功",
      "已抑制",
      "Console 业务数据库",
    ]) {
      expect(markup).toContain(label);
    }
    for (const raw of [
      "attempted",
      "batches",
      "configured",
      "dry_run",
      "sent",
      "suppressed",
      "console-domain-db",
    ]) {
      expect(markup).not.toContain(`>${raw}<`);
    }
  });

  it("provides Chinese labels for current backend task result fields", () => {
    const taskResultFields = [
      "abnormal",
      "account_ids",
      "account_cost",
      "account_rate_sync_error",
      "account_rate_sync_task_id",
      "actual_cost",
      "attempted",
      "attribution_level",
      "audit_failed",
      "auth_status",
      "backup_id",
      "base_url_failed",
      "base_url_resolved",
      "base_url_unavailable",
      "batches",
      "bound",
      "captured_at",
      "captcha_challenge",
      "category",
      "challenge_id",
      "cleaned",
      "cleanup_warnings",
      "combinations",
      "completed",
      "configured",
      "coverage_percent",
      "current_group_ids",
      "decisions",
      "desired_group_ids",
      "dry_run",
      "eligible_groups",
      "exact_model_resolved",
      "expires_at",
      "failure_reason",
      "identity_group",
      "identity_match_percent",
      "issues",
      "latency_p50_ms",
      "latency_p95_ms",
      "latency_p99_ms",
      "local_group_ids",
      "local_group_names",
      "local_groups",
      "local_sync",
      "management_account_deleted",
      "management_base_url",
      "message_ids",
      "model_count",
      "model_rewritten",
      "nearest_outside_model",
      "observed_at",
      "outcomes",
      "parameters_failed",
      "parameters_repaired",
      "parameters_skipped",
      "parameters_unchanged",
      "probe_error",
      "report_date",
      "renamed",
      "request_model",
      "requested_rounds",
      "resolved",
      "response_models",
      "results",
      "revenue",
      "rows",
      "saved",
      "same_standard_percent",
      "sent",
      "standard_model",
      "status_code",
      "stored",
      "suppressed",
      "summaries",
      "summary",
      "sync_balance_multiplier",
      "target_id",
      "target_type",
      "temporary_key",
      "tests",
      "upstream_base_url",
      "upstream_cost",
      "upstream_description",
      "upstream_group_id",
      "upstream_group_name",
      "upstream_key_deleted",
      "upstream_key_count",
      "upstream_key_name",
      "upstream_raw_cost",
      "upstream_type",
      "usable_rounds",
      "verified",
    ];

    for (const field of taskResultFields) {
      expect(logDetailLabel(field), field).toMatch(/[\u3400-\u9fff]/);
    }
  });

  it("translates current backend enum values according to their field context", () => {
    const enumValues: Array<[string, string, string]> = [
      ["source", "console-domain-db", "Console 业务数据库"],
      ["observation_source", "last_successful", "最近一次成功倍率"],
      ["checker", "claude", "Claude 检测器"],
      ["protocol", "anthropic-messages", "Anthropic Messages 协议"],
      ["verdict", "SOL_CONSISTENT", "与 Sol 特征一致"],
      ["verdict", "LUNA_LIKE", "更接近 Luna"],
      ["target_type", "c2c", "私聊"],
      ["target_type", "group", "群聊"],
      ["target_type", "channel", "频道"],
      ["interaction_kind", "image_captcha_ocr", "图片验证码识别"],
      ["code", "recovered_by_refresh", "刷新令牌恢复成功"],
      ["code", "browser_challenge_required", "需要浏览器验证"],
      ["attribution_level", "key", "按上游 Key 精确归因"],
      ["attribution_level", "unavailable", "无法精确归因"],
      ["stage", "quick", "快速检测"],
    ];

    for (const [key, value, expected] of enumValues) {
      expect(formatLogValue(value, key), `${key}=${value}`).toBe(expected);
    }
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
