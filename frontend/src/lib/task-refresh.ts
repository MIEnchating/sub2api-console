import type { Task } from "../api";
import { taskStopsPolling } from "./task-state";

export type TaskRefreshScope =
  | "upstream-action"
  | "inspection"
  | "active-probe"
  | "management-sync"
  | "account-scheduling"
  | "alerts"
  | "onboarding"
  | "pricing"
  | "upstream-key-cleanup";

const refreshKeys: Record<TaskRefreshScope, string[]> = {
  "upstream-action": [
    "upstreams",
    "upstream-groups",
    "upstream-configuration",
    "auth-recovery-config",
    "accounts",
    "groups",
    "logs",
    "overview",
    "overview-events",
  ],
  inspection: [
    "accounts",
    "groups",
    "upstreams",
    "alerts",
    "auto-inspection",
    "logs",
    "overview",
    "overview-events",
  ],
  "active-probe": ["accounts", "logs", "overview-events"],
  "management-sync": ["accounts", "groups", "upstreams", "logs", "overview", "overview-events"],
  "account-scheduling": ["accounts", "groups", "policy", "logs", "overview", "overview-events"],
  alerts: ["alerts", "logs", "overview", "overview-events"],
  onboarding: [
    "accounts",
    "groups",
    "upstreams",
    "upstream-groups",
    "logs",
    "overview",
    "overview-events",
  ],
  pricing: [
    "pricing",
    "pricing-changes",
    "accounts",
    "groups",
    "logs",
    "overview",
    "overview-events",
  ],
  "upstream-key-cleanup": [
    "upstreams",
    "onboarding-candidates",
    "onboarding-unbound-keys",
    "logs",
    "overview",
    "overview-events",
  ],
};

export function terminalRefreshKeys(scope: TaskRefreshScope, task?: Task): string[][] {
  if (!taskStopsPolling(task)) return [];
  return refreshKeys[scope].map((key) => [key]);
}
