import type { NewAPIWorkspace, RuntimeConfig } from "../../../src/api";

const config: RuntimeConfig = {
  database_available: true,
  data_database_available: true,
  mode: "完全模式",
  config_keys: [],
  secret_values_hidden: true,
  probes_enabled: true,
  account_default_concurrency: 10,
  account_default_priority: 1,
  admin_base_url: "https://sub2api.example.test",
  request_timeout_seconds: 60,
  initialized: true,
  target_configured: true,
  console_username: "布局测试",
  configuration_errors: [],
};

export const workspace: NewAPIWorkspace = {
  platforms: [
    {
      id: "layout",
      name: "测试平台",
      base_url: "https://newapi.example.test",
      user_id: "1",
      admin_key_configured: true,
      updated_at: "2026-09-09T00:00:00Z",
    },
  ],
  local_groups: [{ id: "6", name: "标准", ratio: "1" }],
  bindings: [],
  sub2api_base_url: "https://sub2api.example.test",
};

export const pageFixtures: Record<string, unknown> = {
  "/api/config": config,
  "/api/config/log-cleanup": {
    enabled: false,
    retention_days: 30,
    last_run_at: null,
    next_run_at: null,
  },
  "/api/config/model-sync": { blocked_patterns: [] },
  "/api/auth-recovery/config": { auth_records: [], vault_entries: [] },
  "/api/model-checks/capabilities": { claude_standards: [], sol_models: ["test-model"] },
  "/api/model-checks/account-statuses": [],
  "/api/upstreams": {
    hosts: [],
    total_hosts: 0,
    authenticated_hosts: 0,
    recovery_required: 0,
    source: "test",
  },
  "/api/alerts": [],
  "/api/notifications/status": {
    configured: true,
    app_id: "layout",
    client_secret_configured: true,
    home_channel: "layout",
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
  },
  "/api/newapi": workspace,
  "/api/newapi/platforms/layout/refresh": {
    groups: [{ id: "default", name: "默认", ratio: "1" }],
    models: [{ model: "test-model", input_ratio: "0.5", completion_ratio: "4" }],
    unset_models: [],
    references: [],
    tool_prices: [],
    differences: [],
    upstream_prices: [],
  },
  "/api/logs": { items: [], total: 0, page: 1, page_size: 20, counts: {}, truncated: false },
};
