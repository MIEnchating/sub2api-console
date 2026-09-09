import type { AlertIncident, AutoInspectionStatus, Task, UnifiedLogEntry } from "../../../src/api";

export const alerts: AlertIncident[] = Array.from({ length: 22 }, (_, index) => ({
  incident_key: `alert-${index + 1}`,
  event_type: "account.probe",
  object_kind: "account",
  object_id: String(index + 1),
  object_name: `巡检告警-${index + 1}`,
  cause_code: "PROBE",
  status: "firing",
  first_seen_at: "2026-09-09T00:00:00Z",
  last_seen_at: "2026-09-09T00:01:00Z",
  last_error: null,
  delivery_status: "sent",
  delivery_attempts: 1,
  delivered_at: "2026-09-09T00:01:00Z",
}));

export const inspection: AutoInspectionStatus = {
  enabled: true,
  interval_seconds: 15,
  running: false,
  monitoring_configured: true,
  monitoring_enabled: true,
  monitoring_checked_at: null,
  last_run_duration_ms: 1000,
  last_summary: {
    channels: 1,
    probed: 1,
    samples: 1,
    fused: 0,
    recovered: 0,
    applied: 0,
    cleaned_up: 0,
    alerts: 0,
  },
  last_run_at: "2026-09-09T00:00:00Z",
  next_run_at: null,
  last_status: "succeeded",
  last_error: null,
  last_task_id: null,
  queue: [
    {
      task_type: "inspection",
      label: "本轮巡检",
      state: "ready",
      scheduled_for: null,
      detail: "等待主动探测",
      target_count: 1,
      operations: [
        {
          operation: "active_probe",
          label: "主动探测",
          target_count: 1,
          cycle: "每5分钟",
          due: true,
        },
      ],
    },
  ],
  heartbeat_history: [
    {
      checked_at: "2026-09-09T00:00:00Z",
      completed_at: "2026-09-09T00:00:01Z",
      status: "failed",
      operations: ["active_probe"],
      operation_timings: [],
      task_id: null,
      error: "测试探测失败，请检查上游连接。".repeat(12),
      skipped: false,
    },
  ],
};

export const task: Task = {
  id: "layout-task",
  skill: "inspection",
  operation: "automatic-inspection",
  status: "running",
  progress: 30,
  message: "正在读取测试上游",
  result: {},
  created_at: "2026-09-09T00:00:00Z",
  updated_at: "2026-09-09T00:00:01Z",
};

export const log: UnifiedLogEntry = {
  id: "event:layout",
  kind: "event",
  occurred_at: "2026-09-09T00:00:00Z",
  title: "routing.writeback.batch",
  summary: "共 3 个账号：成功 2，失败 1",
  status: "partial",
  actor: "自动巡检",
  object_label: "3 个账号",
  source: "runtime_event",
  source_id: "layout",
  related_count: 3,
  details: {},
};
