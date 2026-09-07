import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { LogDetailsContent, LogStructuredValue } from "../components/log-details-dialog";
import { formatLogValue, logDetailLabel } from "../lib/log-display";

describe("automatic inspection log details", () => {
  it("keeps delegated rate-sync and alert results out of the inspection overview", () => {
    const markup = renderToStaticMarkup(
      createElement(LogDetailsContent, {
        entry: {
          id: "task:inspection-1",
          kind: "task",
          occurred_at: "2026-09-07T01:07:13Z",
          title: "automatic-inspection",
          summary: "巡检完成",
          status: "succeeded",
          actor: null,
          object_label: null,
          source: "task",
          source_id: "inspection-1",
          related_count: 0,
          details: {},
        },
        details: {
          operation: "automatic-inspection",
          result: {
            origin: "automatic-inspection",
            account_rate_sync: {
              queued: true,
              task_id: "rate-sync-child-1",
            },
            alert_evaluation: {
              status: "succeeded",
              findings: 15,
              summary: "重复的告警检测结果",
            },
            operation_timings: [
              { operation: "account_rate_sync", duration_seconds: 3 },
              { operation: "alert_evaluation", duration_seconds: 1 },
            ],
          },
        },
      }),
    );

    expect(markup).toContain('data-slot="log-operation-timings"');
    expect(markup).toContain("账号倍率与名称同步");
    expect(markup).toContain("告警检测");
    expect(markup).not.toContain("rate-sync-child-1");
    expect(markup).not.toContain("重复的告警检测结果");
    expect(markup).not.toContain("发现告警");
  });

  it("shows writeback payloads and moves between pages for an aggregated task", () => {
    const events = Array.from({ length: 21 }, (_, index) => ({
      id: `event:${index + 1}`,
      kind: "event" as const,
      occurred_at: "2026-09-04T14:32:39Z",
      title: "routing.applied",
      summary: `账号 ${index + 1} 自动写回已生效`,
      status: "succeeded",
      actor: "自动巡检",
      object_label: String(index + 1),
      source: "runtime_event" as const,
      source_id: String(index + 1),
      related_count: 0,
      details: {
        event_type: "routing.applied",
        payload: {
          task_id: "inspection-1",
          account_id: String(index + 1),
          desired: { priority: index + 10 },
          effective: { priority: index + 10 },
          remote_write: true,
        },
      },
    }));
    render(
      createElement(LogDetailsContent, {
        entry: {
          id: "event-group:inspection-1",
          kind: "event",
          occurred_at: "2026-09-04T14:32:39Z",
          title: "routing.writeback.batch",
          summary: "共 21 个账号：成功 21",
          status: "succeeded",
          actor: "自动巡检",
          object_label: "21 个账号",
          source: "runtime_event",
          source_id: "inspection-1",
          related_count: 21,
          details: { events },
        },
      }),
    );

    expect(screen.getAllByText("写入目标")).toHaveLength(20);
    expect(screen.getAllByText("实际生效值")).toHaveLength(20);
    expect(screen.getAllByText("远程写入")).toHaveLength(20);
    expect(screen.getByText("账号 20 自动写回已生效")).toBeInTheDocument();
    expect(screen.queryByText("账号 21 自动写回已生效")).not.toBeInTheDocument();
    expect(screen.queryByText("inspection-1")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));

    expect(screen.getByText("账号 21 自动写回已生效")).toBeInTheDocument();
    expect(screen.queryByText("账号 20 自动写回已生效")).not.toBeInTheDocument();
  });

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

  it("shows large execution item collections immediately and paginates their records", () => {
    const items = Array.from({ length: 21 }, (_, index) => ({
      account_id: String(index + 1),
      account_name: `倍率账号 ${index + 1}`,
      upstream_host: `upstream-${index + 1}.example.com`,
      platform: "openai",
      before: index === 0 ? "0.15" : "0.12",
      after: "0.12",
      name_before: index === 0 ? "倍率账号 1（旧）" : `倍率账号 ${index + 1}`,
      name_after: `倍率账号 ${index + 1}`,
      upstream_raw_multiplier: "1.2",
      recharge_rate: "10",
      account_multiplier: "0.12",
      observation_source: "live",
      remote_write: index === 0,
      readback_confirmed: index === 0,
      status: index === 0 ? "已同步" : "已确认一致",
    }));
    const view = render(
      createElement(LogStructuredValue, {
        value: {
          origin: "automatic-inspection",
          items,
        },
      }),
    );

    expect(screen.getByText("触发来源")).toBeInTheDocument();
    expect(screen.getByText("自动巡检")).toBeInTheDocument();
    expect(screen.getByText("执行明细")).toBeInTheDocument();
    expect(screen.getByText("21 项")).toBeInTheDocument();
    expect(screen.getAllByText("倍率账号 1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("倍率账号 10").length).toBeGreaterThan(0);
    expect(screen.queryByText("倍率账号 11")).not.toBeInTheDocument();
    expect(screen.getByText("执行明细").closest("details")).toBeNull();
    expect(screen.getByRole("table", { name: "账号倍率同步执行明细" })).toBeInTheDocument();
    expect(screen.getAllByRole("row")).toHaveLength(11);
    expect(screen.getByRole("columnheader", { name: "账号" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "上游" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "倍率" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "名称处理" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "远程执行" })).toBeInTheDocument();
    expect(screen.getByText("0.15 → 0.12")).toBeInTheDocument();
    expect(screen.getAllByText("0.12")).toHaveLength(9);
    expect(screen.getAllByText("未变化")).toHaveLength(9);
    expect(screen.queryByText("0.12 → 0.12")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "转到下一页" }));

    expect(screen.getByText("倍率账号 11")).toBeInTheDocument();
    expect(screen.queryByText("倍率账号 10")).not.toBeInTheDocument();
    view.unmount();
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
