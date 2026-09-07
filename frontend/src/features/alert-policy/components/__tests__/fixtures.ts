import type { QueryClient } from "@tanstack/react-query";
import type { AlertPolicy, GroupStatus, NotificationStatus } from "@/api";

export const policy: AlertPolicy = {
  enabled: true,
  configuration_enabled: true,
  auth_enabled: true,
  rate_sync_enabled: true,
  multiplier_increase_enabled: true,
  multiplier_decrease_enabled: true,
  balance_enabled: true,
  probe_enabled: true,
  routing_breaker_enabled: true,
  routing_degraded_enabled: true,
  routing_degraded_types: ["health_score", "latency"],
  routing_survivor_enabled: true,
  group_unavailable_enabled: true,
  group_survivor_enabled: true,
  apply_failure_enabled: true,
  balance_thresholds: ["20", "10", "5"],
  probe_failure_streak: 3,
  probe_recovery_streak: 3,
  probe_groups: ["codex", "pro"],
  delivery_enabled: true,
  notify_recovery: false,
  recovery_notification_types: ["auth", "balance", "group_unavailable"],
  repeat_interval_minutes: 30,
  state_change_cooldown_minutes: 30,
  merge_threshold: 10,
};

export const groups: GroupStatus[] = [
  {
    name: "codex",
    id: "6",
    account_count: 3,
    scheduling_open: 3,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
  },
  {
    name: "pro",
    id: "8",
    account_count: 2,
    scheduling_open: 2,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
  },
  {
    name: "standard",
    id: "10",
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
  },
];

export const notificationStatus: NotificationStatus = {
  configured: true,
  app_id: "app",
  client_secret_configured: true,
  home_channel: "target",
  channel_type: "c2c",
  destination_configured: true,
  configuration_errors: [],
  queues: {
    producer_firing: 0,
    producer_recovered: 0,
    consumer_pending: 0,
    consumer_failed: 0,
    consumer_active: false,
  },
};

export function cacheAlertPolicyPageData(
  queryClient: QueryClient,
  value: AlertPolicy = policy,
): void {
  queryClient.setQueryData(["alert-policy"], value);
  queryClient.setQueryData(["notification-status"], notificationStatus);
  queryClient.setQueryData(["groups"], groups);
}
