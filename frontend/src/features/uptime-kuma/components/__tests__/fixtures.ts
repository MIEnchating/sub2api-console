import type { KumaConfig, KumaMonitor, Task } from "@/api";

export const config: KumaConfig = {
  base_url: "https://kuma.example",
  username: "admin",
  api_key_configured: true,
  management_configured: true,
  revision: 1,
};
export const monitor: KumaMonitor = {
  id: 19,
  key: "id:19",
  name: "智谱",
  type: "http",
  url: "https://monitor.example/health",
  url_redacted: false,
  active: true,
  parent: null,
  interval: 300,
  status: 0,
  response_time: null,
  certificate_days: 32,
  uptime: 0,
  revision: "monitor-version",
};
export const task: Task = {
  id: "test-task",
  skill: "uptime-kuma",
  operation: "uptime-kuma-delete",
  status: "succeeded",
  progress: 100,
  message: "操作完成",
  result: {},
  created_at: "2026-09-10T00:00:00Z",
  updated_at: "2026-09-10T00:00:01Z",
};
