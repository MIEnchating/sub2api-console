import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LogStructuredValue } from "../components/log-details-dialog";
import { formatLogValue, logDetailLabel } from "../lib/log-display";

describe("automatic inspection log details", () => {
  it("renders evidence metrics with Chinese labels and source values", () => {
    const markup = renderToStaticMarkup(
      createElement(LogStructuredValue, {
        value: {
          evidence: {
            effective_source: "traffic+active_probe",
            fallback_reason: "没有新鲜真实流量，按探测间隔补充主动探测",
            malformed_rows: 0,
            monitored_accounts: 239,
            monitoring_available: true,
            probe_duration_seconds: 126.1499534,
            probes_persisted: 90,
            requested_source: "traffic",
            traffic_checked: true,
            traffic_duration_seconds: 1.990429115,
            traffic_persisted: 124,
          },
        },
      }),
    );

    for (const label of [
      "有效数据来源",
      "降级原因",
      "异常数据行",
      "监控账号数",
      "监控数据可用",
      "主动探测耗时",
      "已保存主动探测样本",
      "请求的数据来源",
      "已检查真实流量",
      "真实流量耗时",
      "已保存流量样本",
      "真实流量 + 主动探测",
    ]) {
      expect(markup).toContain(label);
    }
    expect(markup).not.toContain("effective source");
    expect(markup).not.toContain("traffic+active_probe");
  });

  it("renders operation timings as one compact localized row per operation", () => {
    const markup = renderToStaticMarkup(
      createElement(LogStructuredValue, {
        value: {
          operations: ["evidence_collection", "routing_calculation", "routing_writeback"],
          completed_operations: ["evidence_collection", "routing_calculation", "routing_writeback"],
          operation_timings: [
            {
              operation: "evidence_collection",
              duration_seconds: 129.478945578,
              started_at: "2026-09-04T14:30:29Z",
            },
            {
              operation: "routing_calculation",
              duration_seconds: 0.660987445,
              started_at: "2026-09-04T14:32:38Z",
            },
            {
              operation: "routing_writeback",
              duration_seconds: 3.388639898,
              started_at: "2026-09-04T14:32:39Z",
            },
          ],
        },
      }),
    );

    expect(markup).toContain('data-slot="log-operation-timings"');
    expect(markup).toContain("请求记录与探针");
    expect(markup).toContain("调度计算");
    expect(markup).toContain("自动执行");
    expect(markup).toContain("2 分 9.5 秒");
    expect(markup).toContain("0.7 秒");
    expect(markup).not.toContain("第 1 项");
    expect(markup).not.toContain("evidence collection");
    expect(markup).not.toContain("routing calculation");
    expect(markup).not.toContain("129.478945578");
    expect(markup).not.toContain('data-slot="log-operation-list"');
  });

  it("renders inspection operation collections as a compact localized summary", () => {
    const markup = renderToStaticMarkup(
      createElement(LogStructuredValue, {
        value: {
          planned_operations: [
            { operation: "traffic_refresh" },
            { operation: "disk_evaluation" },
            { operation: "alert_evaluation" },
          ],
          completed_operations: ["traffic_refresh", "disk_evaluation"],
        },
      }),
    );

    expect(markup).toContain('data-slot="log-operation-list"');
    expect(markup).toContain("真实流量同步");
    expect(markup).toContain("磁盘检查");
    expect(markup).toContain("告警检测");
    expect(markup).not.toContain("第 1 项");
    expect(markup).not.toContain("disk_evaluation");
  });

  it("collapses and paginates large routing record maps", () => {
    const accountTargets = Object.fromEntries(
      Array.from({ length: 12 }, (_, index) => [
        String(index + 1),
        {
          account_id: String(index + 1),
          schedulable: true,
          desired_load_factor: "50",
        },
      ]),
    );
    const markup = renderToStaticMarkup(
      createElement(LogStructuredValue, {
        value: { routing: { account_targets: accountTargets } },
      }),
    );

    expect(markup).toContain("<details");
    expect(markup).toContain("调度目标");
    expect(markup).toContain("12 项");
    expect(markup).toContain('data-slot="log-result-items"');
    expect(markup).not.toContain("第 11 项");
  });

  it("provides Chinese labels for inspection and routing result fields", () => {
    const fields = [
      "abandon_control",
      "account_decisions",
      "account_rate_sync",
      "account_targets",
      "alerts",
      "applied",
      "auth_recovery",
      "channels",
      "cleanup_action",
      "cleaned_up",
      "configuration_error",
      "configuration_errors",
      "cost_tier",
      "cost_wall",
      "desired_concurrency",
      "desired_health",
      "desired_load_factor",
      "degraded",
      "diagnostic_detail",
      "diagnostic_only",
      "fused_until",
      "fused",
      "health_evaluations",
      "health_score",
      "latest_event",
      "long_score",
      "newly_fused",
      "monitoring_enabled",
      "price_management",
      "probed",
      "rank",
      "rate_known",
      "rate_reason",
      "recovery_target",
      "release_control",
      "routing_state",
      "sample_count",
      "samples",
      "scaling_cooldown_active",
      "short_score",
      "state_since",
      "survivors",
      "target_concurrency",
      "target_load_factor",
      "target_priority",
      "target_schedulable",
      "ttfb_p50_ms",
      "ttfb_p95_ms",
      "weight",
      "write_cooldown_active",
    ];

    for (const field of fields) {
      expect(logDetailLabel(field), field).toMatch(/[\u3400-\u9fff]/);
    }

    expect(formatLogValue("slow_ttfb", "latest_event")).toBe("首字延迟过高");
    expect(formatLogValue("cost_blocked", "desired_health")).toBe("成本墙拦截");
  });
});
