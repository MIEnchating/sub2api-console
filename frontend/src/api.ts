import { SessionExpiredError, signalSessionExpired } from "./lib/session-auth";

export type Overview = {
  database_path: string;
  database_available: boolean;
  account_count: number | null;
  group_count: number | null;
  open_alerts: number;
  recent_runs: number;
  last_activity: string | null;
  mode: string;
};

export type RuntimeConfig = {
  database_path: string;
  data_database_path: string;
  database_available: boolean;
  data_database_available: boolean;
  mode: string;
  config_keys: unknown;
  secret_values_hidden: boolean;
  probes_enabled: boolean;
  admin_base_url: string | null;
  request_timeout_seconds: number;
  account_default_concurrency: number;
  account_default_priority: number;
  initialized: boolean;
  target_configured: boolean;
  console_username: string | null;
  configuration_errors: string[];
};

export type RuntimeMode = "监控模式" | "完全模式";

export type NewAPIPlatform = {
  id: string;
  name: string;
  base_url: string;
  user_id: string;
  admin_key_configured: boolean;
  updated_at: string;
};

export type NewAPILocalGroup = {
  id: string;
  name: string;
  ratio: string | null;
};

export type NewAPIGroupBinding = {
  platform_id: string;
  newapi_group_id: string;
  newapi_group_name: string;
  sub2api_group_id: string;
  sync_ratio: boolean;
};

export type NewAPIGroupBindingUpdate = Omit<NewAPIGroupBinding, "platform_id">;

export type NewAPIWorkspace = {
  platforms: NewAPIPlatform[];
  local_groups: NewAPILocalGroup[];
  bindings: NewAPIGroupBinding[];
};

export type NewAPIRemoteGroup = {
  id: string;
  name: string;
  ratio: string | null;
};

export type NewAPIModelPrice = {
  model: string;
  input_ratio: string;
  completion_ratio: string;
};

export type NewAPIRemoteSnapshot = {
  groups: NewAPIRemoteGroup[];
  models: NewAPIModelPrice[];
  references: NewAPIModelPrice[];
  differences: Array<{
    model: string;
    kind: "missing_in_newapi" | "only_in_newapi" | "ratio_mismatch";
    configured: NewAPIModelPrice | null;
    reference: NewAPIModelPrice | null;
  }>;
  fetched_at: string;
};

export type NotificationStatus = {
  configured: boolean;
  app_id: string;
  client_secret_configured: boolean;
  home_channel: string;
  channel_type: string;
  destination_configured: boolean;
  configuration_errors: string[];
  queues: {
    producer_firing: number;
    producer_recovered: number;
    consumer_pending: number;
    consumer_failed: number;
    consumer_active: boolean;
  };
};

type AuthRecordIndex = {
  host: string;
  configured: boolean;
  has_headers: boolean;
};
export type VaultEntryIndex = {
  entry: string;
  hosts: string[];
  has_username: boolean;
  has_password: boolean;
  username_is_email: boolean;
  header_names: string[];
};
export type PrivateAuthConfigStatus = {
  auth_records: AuthRecordIndex[];
  vault_entries: VaultEntryIndex[];
};

export type OnboardingCandidate = {
  number: number;
  upstream_id: string;
  host: string;
  upstream_name: string;
  group_id: string | null;
  group_name: string;
  description: string | null;
  platform: string | null;
  status?: string | null;
  multiplier: string | null;
  recommended_binding: string;
  bindable: boolean;
  can_create_key: boolean;
  can_bind_existing_key: boolean;
  bound: boolean;
  bound_accounts: UpstreamBoundAccount[];
  key_present: boolean;
  upstream_key_id: string | null;
  upstream_key_name: string | null;
  recharge_rate: string | null;
  unavailable_reason: string | null;
};

type UnboundUpstreamKey = {
  key_id: string;
  name: string;
  group_id: string | null;
  status: string | null;
};

export type KeyCleanupPreview = {
  host: string;
  keys: UnboundUpstreamKey[];
};

export type UpstreamBoundAccount = {
  binding_id: number;
  account_id: string;
  account_name: string | null;
  base_url?: string | null;
  account_exists: boolean;
  binding_status: string | null;
  local_group: string;
  local_groups?: Array<{ id: string; name: string }>;
  upstream_key_id: string;
  upstream_key_name: string;
};

export type SetupStatus = {
  initialized: boolean;
  target_configured: boolean;
  setup_token_required: boolean;
  configuration_errors?: string[];
};

export type SessionStatus = {
  authenticated: boolean;
  username: string | null;
};

type UpstreamHost = {
  upstream_id: string;
  host: string;
  hosts: string[];
  base_url: string;
  name: string;
  upstream_type: string;
  account_count: number;
  group_count: number;
  auth_status: string;
  raw_balance: string | null;
  balance: string | null;
  display_balance?: string | null;
  balance_unit?: string | null;
  recharge_rate: string;
  balance_status: string;
  checked_at: string | null;
};

export type UpstreamSummary = {
  hosts: UpstreamHost[];
  total_hosts: number;
  authenticated_hosts: number;
  recovery_required: number;
  source: string;
};

export type UpstreamDeletePreview = {
  host: string;
  base_url: string;
  upstream_type: string;
  account_count: number;
  group_count: number;
  account_ids: string[];
  accounts: Array<{ id: string; name: string; groups: string[] }>;
};

export type UpstreamGroup = {
  upstream_id: string;
  host: string;
  group_id: string | null;
  name: string;
  description: string | null;
  platform: string | null;
  status: string | null;
  raw_rate: string | null;
  effective_rate: string | null;
  recharge_rate: string | null;
  bound: boolean;
  bound_accounts: UpstreamBoundAccount[];
  key_present: boolean;
  bindable: boolean;
  unavailable_reason: string | null;
};

export type UpstreamGroupChange = {
  id: number;
  upstream_id: string;
  group_id: string;
  group_name: string;
  change_type: "added" | "removed";
  changed_at: string;
};

export type UpstreamConfiguration = {
  upstream_id: string;
  host: string;
  name: string;
  base_url: string;
  account_base_url: string;
  upstream_type: string;
  auth_mode: string;
  recharge_rate: string;
  raw_balance: string | null;
  balance: string | null;
  has_access_token: boolean;
  has_refresh_token: boolean;
  has_admin_key: boolean;
  has_user_id: boolean;
  headers: Record<string, string>;
  header_names: string[];
  cookie_names: string[];
  groups: UpstreamGroup[];
  rate_sync_task_id?: string;
  rate_sync_error?: string;
  base_url_sync_task_id?: string;
  base_url_sync_error?: string;
};

export type UpstreamConfigurationUpdate = {
  name?: string;
  base_url: string;
  account_base_url: string;
  upstream_type: string;
  auth_mode: string;
  recharge_rate: string;
  access_token?: string | null;
  refresh_token?: string | null;
  admin_key?: string | null;
  user_id?: string | null;
  headers?: Record<string, string> | null;
  cookies?: Record<string, string> | null;
  username?: string | null;
  password?: string | null;
  save_to_vault?: boolean;
  entry?: string | null;
};

export type UpstreamCreatePayload = UpstreamConfigurationUpdate & {
  host: string;
  name: string;
};

export type UpstreamDetection = {
  base_url: string;
  host: string;
  upstream_type: string | null;
  auth_mode: string | null;
  name: string | null;
  type_detected: boolean;
  name_detected: boolean;
  evidence: string | null;
};

export type OnboardingContext = {
  upstream: UpstreamConfiguration;
  candidates: OnboardingCandidate[];
};

export type ProbeResult = {
  status: "passed" | "failed" | "skipped";
  message: string;
  request_model: string;
  actual_model: string;
  latency_ms: number;
  http_status: number;
  temporary_key?: boolean;
};

export type OnboardingRequest = {
  host: string;
  upstream_type: string;
  base_url?: string;
  platform?: string;
  account_type?: string;
  notes?: string;
  local_group_id?: number;
  local_group_ids?: number[];
  upstream_group_id: string;
  account_ids?: string[];
  extra?: Record<string, unknown>;
  priority?: number;
  concurrency?: number;
  schedulable?: boolean;
};

export type PolicySnapshot = {
  available: boolean;
  source: string;
  mode: RuntimeMode;
  global_strategy: string | null;
  group_strategies: Array<{
    id: string | null;
    name: string;
    platforms: string[];
    strategy: string;
    strategy_source: "global_default" | "group_override" | "configuration_error";
    participation_status: "participating" | "out_of_scope" | "configuration_error";
    participation_reason: string | null;
    account_count: number;
  }>;
  missing_rate_fallback: string | null;
  change_threshold: string | null;
  cooldown_seconds: number | null;
  auto_apply: Record<string, boolean> | null;
  excluded_group_ids: string[] | null;
  traffic_enabled: boolean | null;
  probe_interval_seconds: number | null;
  probe_model: string | null;
  traffic_lookback_minutes: number | null;
  max_samples_per_account: number | null;
  advanced_policy: Record<string, unknown>;
  configuration_errors: string[];
};

export type PolicyUpdate = {
  mode: RuntimeMode;
  global_strategy: string;
  missing_rate_fallback: string;
  change_threshold: string;
  cooldown_seconds: number;
  auto_apply: Record<string, boolean>;
  excluded_group_ids: string[];
  traffic_enabled: boolean;
  probe_interval_seconds: number;
  probe_model: string;
  traffic_lookback_minutes: number;
  max_samples_per_account: number;
  advanced_policy?: Record<string, unknown>;
  group_strategies?: Record<string, string | null>;
};

export type PolicyUpdatePayload = Partial<PolicyUpdate> & {
  group_strategies?: Record<string, string | null>;
};

export type AutoInspectionConfig = {
  enabled: boolean;
  interval_seconds: number;
};

type InspectionRoundSummary = {
  channels: number;
  probed: number;
  samples: number;
  fused: number;
  recovered: number;
  applied: number;
  cleaned_up: number;
  alerts: number;
};

export type AutoInspectionStatus = AutoInspectionConfig & {
  running: boolean;
  monitoring_configured: boolean;
  monitoring_enabled: boolean;
  monitoring_checked_at: string | null;
  last_run_duration_ms: number;
  last_summary: InspectionRoundSummary;
  last_run_at: string | null;
  next_run_at: string | null;
  last_status: "succeeded" | "failed" | "cancelled" | null;
  last_error: string | null;
  last_task_id: string | null;
  queue: Array<{
    task_type: "inspection";
    label: string;
    state: "ready" | "waiting" | "disabled" | "blocked";
    scheduled_for: string | null;
    detail: string;
    target_count: number | null;
    operations: Array<{
      operation: string;
      label: string;
      target_count: number | null;
      cycle: string;
      due: boolean;
    }>;
  }>;
  heartbeat_history: Array<{
    checked_at: string;
    completed_at: string | null;
    status: "running" | "succeeded" | "failed" | "cancelled";
    operations: string[];
    operation_timings: Array<{
      operation: string;
      duration_seconds: number;
      started_at?: string | null;
    }>;
    task_id: string | null;
    error: string | null;
    skipped: boolean;
    summary?: InspectionRoundSummary | null;
    monitoring_enabled?: boolean | null;
  }>;
};

export type LogCleanupStatus = {
  enabled: boolean;
  retention_days: number;
  last_run_at: string | null;
  next_run_at: string | null;
};

export type LogCleanupResult = {
  tasks: number;
  runs: number;
  events: number;
  changes: number;
  protected_tasks: number;
  total: number;
  retention_days: number;
  cutoff_at: string;
  completed_at: string;
};

export type GroupStatus = {
  name: string;
  id: string | null;
  platform?: string | null;
  platforms?: string[];
  rate_multiplier?: string | null;
  probe_interval_seconds?: number;
  account_count: number;
  scheduling_open: number;
  scheduling_closed: number;
  scheduling_unknown: number;
  healthy_accounts?: number;
  degraded_accounts?: number;
  fused_accounts?: number;
  paused_accounts?: number;
  disabled_accounts?: number;
  excluded_accounts?: number;
  rate_limited_accounts?: number;
  pending_accounts?: number;
  available_accounts?: number;
  needs_attention?: number;
  scored_accounts?: number;
  average_health_score?: number | null;
  strategy: string;
  strategy_source: "global_default" | "group_override" | "configuration_error";
  participation_status: "participating" | "out_of_scope" | "configuration_error";
  participation_reason: string | null;
  status:
    | "healthy"
    | "rate_limited"
    | "partial_degraded"
    | "survivor_only"
    | "all_fused"
    | "all_unavailable"
    | "skipped"
    | "excluded"
    | "empty"
    | "configuration_error";
  override?: GroupPolicyOverride | null;
};

export type GroupAllocationChannel = {
  account_id: string;
  account_name: string;
  health: string;
  health_score: number | null;
  short_score: number | null;
  long_score: number | null;
  sample_count: number;
  ttfb_p95_ms: number | null;
  rate: string | null;
  priority: number | null;
  weight: number | null;
  assigned_concurrency: number | null;
  schedulable: boolean | null;
  rank: number | null;
  reason: string | null;
  updated_at: string | null;
};

export type GroupAllocation = {
  group_id: string;
  group_name: string;
  platform: string | null;
  rate_multiplier: string | null;
  status: GroupStatus["status"];
  probe_interval_seconds: number;
  weight_budget: number;
  total_weight: number;
  has_allocation: boolean;
  strategy: string;
  account_count: number;
  healthy_accounts: number;
  available_accounts: number;
  fused_accounts: number;
  paused_accounts: number;
  unavailable_accounts: number;
  rate_limited_accounts: number;
  pending_accounts: number;
  highest_health_score: number | null;
  average_health_score: number | null;
  assigned_concurrency: number;
  channels: GroupAllocationChannel[];
};

export type PricingConfig = {
  enabled: boolean;
  profit_margin: number;
  exchange_group_sets: string[][];
  exchange_group_set_names: string[];
  interval_seconds: number;
  write_concurrency: number;
};

export type PricingGroup = {
  id: string;
  name: string;
  platform: string;
  status: string;
  rate_multiplier: string | null;
  managed: boolean;
  available: boolean;
  reason: string | null;
};

export type PricingDecision = {
  account_id: string;
  account_name: string;
  platform: string;
  cost_multiplier: string | null;
  current_group_ids: string[];
  desired_group_ids: string[];
  eligible_groups: string[];
  changed: boolean;
  skipped: boolean;
  reason: string | null;
};

export type PricingSnapshot = {
  config: PricingConfig;
  groups: PricingGroup[];
  decisions: PricingDecision[];
  accounts: number;
  changes: number;
  skipped: number;
  generated_at: string;
};

export type PricingBackup = {
  id: string;
  name: string;
  actor: string;
  account_count: number;
  created_at: string;
};

export type RevenueRow = {
  account_id: string;
  account_name: string;
  local_group: string;
  upstream_host: string;
  upstream_key_name: string;
  account_cost: number | null;
  actual_cost: number | null;
  upstream_raw_cost: number | null;
  recharge_rate: number | null;
  upstream_cost: number | null;
  difference: number | null;
  revenue: number | null;
  category: "计费异常" | "正常" | "无法核对";
  note: string;
  attribution_level: "key" | "unavailable";
};

type RevenueSummary = {
  group: string;
  accounts: number;
  account_cost: number;
  actual_cost: number;
  upstream_raw_cost: number;
  upstream_cost: number;
  difference: number;
  revenue: number;
};

export type RevenueReport = {
  report_date: string;
  timezone: string;
  tolerance: number;
  rows: RevenueRow[];
  summaries: RevenueSummary[];
  issues: Array<{ host: string; reason: string }>;
  comparable: number;
  unavailable: number;
  abnormal: number;
  generated_at: string;
};

type GroupPolicyOverride = {
  enabled?: boolean | null;
  strategy?: "balanced" | "price_first" | "speed_first" | "reliability" | null;
  min_pool_size?: number | null;
  weight_budget?: number | null;
  balanced_price_ratio?: number | null;
  breaker_enabled?: boolean | null;
  recovery_enabled?: boolean | null;
  weights_enabled?: boolean | null;
  scaling_enabled?: boolean | null;
  probe_enabled?: boolean | null;
  probe_interval_seconds?: number | null;
  probe_model?: string | null;
};

export type GroupPolicyOverrideUpdate = {
  enabled: boolean;
  strategy: "balanced" | "price_first" | "speed_first" | "reliability";
  min_pool_size: number;
  weight_budget: number;
  balanced_price_ratio: number;
  breaker_enabled: boolean;
  recovery_enabled: boolean;
  weights_enabled: boolean;
  scaling_enabled: boolean;
  probe_enabled: boolean;
  probe_interval_seconds: number;
  probe_model: string | null;
};

export type AccountStatus = {
  id: string;
  name: string;
  groups: string[];
  upstream_id: string | null;
  upstream_host: string | null;
  recorded_upstream_host?: string | null;
  upstream_host_repairable?: boolean;
  upstream_type: string | null;
  base_url?: string | null;
  base_url_checked_at?: string | null;
  base_url_source?: "explicit" | "platform_default" | null;
  upstream_base_url?: string | null;
  base_url_check?:
    | "matched"
    | "different_allowed"
    | "official_mismatch"
    | "invalid"
    | "unchecked"
    | "unknown";
  base_url_check_reason?: string | null;
  key_status?: string | null;
  key_status_reason?: string | null;
  sub2api_status?: string | null;
  sub2api_error?: string | null;
  platform?: string | null;
  account_type?: string | null;
  schedulable: boolean | null;
  priority: number | null;
  manual_priority?: number | null;
  manual_sync_balance_multiplier?: boolean;
  load_factor: string | null;
  concurrency: number | null;
  multiplier: string | null;
  balance: string | null;
  paused: boolean | null;
  paused_reason: string | null;
  routing_state: string | null;
  health_status: string | null;
  health: string;
  desired_health: string | null;
  apply_pending: boolean;
  apply_error: string | null;
  decision_state: string | null;
  decision_reason: string | null;
  last_error?: string | null;
  upstream_block?: string | null;
  upstream_block_reason?: string | null;
  failure_streak: number | null;
  recovery_pass_streak: number | null;
  target_priority: number | null;
  target_load_factor: string | null;
  target_schedulable: boolean | null;
  target_concurrency: number | null;
  health_score: number | null;
  short_score: number | null;
  long_score: number | null;
  sample_count: number;
  model_check_status?: ModelCheckAccountStatus["status"] | "loading" | "unavailable" | null;
  model_check_checked_at?: string | null;
  recent_results: AccountRecentResult[];
  ttfb_p50_ms: number | null;
  ttfb_p95_ms: number | null;
  weight: number | null;
};

export type AccountRecentResult = {
  result: string | null;
  event_type?: string | null;
  score?: number | null;
  observed_at: string | null;
  latency_ms: number | null;
  duration_ms?: number | null;
  failure_reason: string | null;
  source: string;
};

type AccountBinding = {
  id: number;
  local_account_id: string;
  upstream_id: string;
  upstream_host: string;
  upstream_key_id: string;
  upstream_key_name: string;
  upstream_group: string | null;
  upstream_group_id: string | null;
  local_group: string;
  local_rate: string | null;
  upstream_rate: string | null;
  source_auth_host: string | null;
  binding_host_alias: string | null;
  description: string | null;
  status: string | null;
  updated_at: string;
};

export type AccountDetail = AccountStatus & {
  metadata: Record<string, unknown>;
  group_rates: Record<string, string | null>;
  group_ids: Record<string, string | null>;
  bindings: AccountBinding[];
  test_model: string | null;
};

export type AccountDeletePreview = {
  account_id: string;
  account_name: string;
  groups: string[];
  management_base_url: string;
  binding: {
    id: number;
    upstream_id: string;
    upstream_host: string;
    auth_host: string;
    upstream_key_id: string;
    upstream_key_name: string;
  } | null;
};

export type AccountControlAction = "pause" | "resume" | "exclude" | "include" | "fuse" | "recover";

export type RunEvent = {
  id: number;
  event_type: string;
  created_at: string;
  status: string;
  summary: string;
  payload: Record<string, unknown>;
};

export type CaptchaChallenge = {
  challenge_id: string;
  host: string;
  image_data: string;
  expires_at: string;
  credential: {
    entry: string;
  };
  interaction_kind: "image_captcha_ocr";
};

export type CaptchaRecoveryResult = {
  success: true;
  host: string;
  profile_status: "verified";
  balance: unknown;
  concurrency: unknown;
  keys: number;
  groups: number;
  stored: true;
  interaction_kind: "image_captcha_ocr";
  projection: Record<string, unknown>;
};

export type ManualAuthVerifyResult =
  | {
      host: string;
      verified: true;
      balance_sync: {
        status: "succeeded" | "failed";
        balance_status: string;
        balance?: string | number | null;
        display_balance?: string | number | null;
        balance_unit?: string | null;
        checked_at?: string | null;
        reason?: string | null;
      };
    }
  | {
      host: string;
      verified: false;
      captcha_challenge: CaptchaChallenge;
    };

export type UsageRecord = {
  id: number;
  request_id: string;
  account_id: string | null;
  account_name: string | null;
  group_name: string | null;
  is_error: boolean | null;
  error_reason: string | null;
  first_token_ms: string | null;
  duration_ms: string | null;
  summary: string | null;
  observed_at: string | null;
  source: string;
  payload: Record<string, unknown>;
};

export type SystemLogSearchQuery = {
  timeRange: "5m" | "30m" | "1h" | "6h" | "24h" | "7d" | "30d";
  startTime: string;
  endTime: string;
  host: string;
  level: string;
  requestId: string;
  clientRequestId: string;
  apiKeyId: string;
  accountId: string;
  platform: string;
  model: string;
  keyword: string;
  page: number;
  pageSize: number;
};

export type SystemLogPage = {
  items: UsageRecord[];
  total: number;
  page: number;
  page_size: number;
};

export type RequestTrace = {
  request_id: string;
  matched: boolean;
  account_id: string | null;
  account_name: string | null;
  records: UsageRecord[];
  recent_errors: UsageRecord[];
};

export type AlertIncident = {
  incident_key: string;
  event_type: string;
  object_kind: string;
  object_id: string;
  object_name: string | null;
  cause_code: string;
  status: string;
  first_seen_at: string;
  last_seen_at: string;
  last_error: string | null;
  delivery_status: string | null;
  delivery_attempts: number;
  delivered_at: string | null;
};

export type NotificationQueueItem = AlertIncident & {
  queue_status?: string;
  queue_reason?: string;
};

export type NotificationQueueDetails = {
  producer_firing: NotificationQueueItem[];
  producer_recovered: NotificationQueueItem[];
  consumer_pending: NotificationQueueItem[];
  consumer_failed: NotificationQueueItem[];
  consumer_items: NotificationQueueItem[];
};

export type AlertPolicy = {
  enabled: boolean;
  configuration_enabled: boolean;
  auth_enabled: boolean;
  rate_sync_enabled: boolean;
  balance_enabled: boolean;
  probe_enabled: boolean;
  routing_breaker_enabled: boolean;
  routing_degraded_enabled: boolean;
  routing_survivor_enabled: boolean;
  group_unavailable_enabled: boolean;
  group_survivor_enabled: boolean;
  apply_failure_enabled: boolean;
  balance_thresholds: string[];
  probe_failure_streak: number;
  probe_recovery_streak: number;
  probe_groups: string[];
  delivery_enabled: boolean;
  notify_recovery: boolean;
  repeat_interval_minutes: number;
  state_change_cooldown_minutes: number;
  merge_threshold: number;
};

export type UnifiedLogKind = "all" | "task" | "event" | "change";
export type UnifiedLogState = "all" | "active" | "failed" | "warning" | "succeeded";
export type UnifiedLogEventLevel = "all" | "info" | "warning" | "error";

export type UnifiedLogEntry = {
  id: string;
  kind: Exclude<UnifiedLogKind, "all">;
  occurred_at: string;
  title: string;
  summary: string;
  status: string;
  actor: string | null;
  object_label: string | null;
  source: "task" | "run_record" | "runtime_event" | "operation_audit";
  source_id: string;
  related_count: number;
  details: Record<string, unknown>;
};

export type UnifiedLogPage = {
  items: UnifiedLogEntry[];
  total: number;
  page: number;
  page_size: number;
  counts: Record<string, number>;
  truncated: boolean;
};

export type Task = {
  id: string;
  skill: string;
  operation: string;
  status: "queued" | "running" | "waiting_input" | "succeeded" | "failed" | "cancelled";
  progress: number;
  message: string;
  result: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type ModelCheckCapabilities = {
  claude_standards: string[];
  sol_models: string[];
};

export type ModelCheckAccountStatus = {
  account_id: string;
  status: "consistent" | "inconsistent" | "inconclusive";
  checked_at: string;
  task_id: string;
};

export type ModelCheckRequest = {
  account_ids: string[];
  models: string[];
  rounds: number;
  timeout_seconds: number;
};

export type TrafficRankingSort = "traffic" | "stability" | "success_rate" | "latency";

export type TrafficRankingRow = {
  rank: number;
  account_id: string;
  account_name: string;
  upstream_host: string;
  platform: string;
  groups: string[];
  requests: number;
  successful: number;
  failed: number;
  traffic_share: number | null;
  success_rate: number | null;
  stability_score: number | null;
  average_latency_ms: number | null;
  p95_latency_ms: number | null;
  active_buckets: number;
  total_buckets: number;
  latest_at: string | null;
};

export type TrafficRanking = {
  start_at: string;
  end_at: string;
  group_name: string;
  sort_by: TrafficRankingSort;
  bucket: "hour" | "day";
  total_requests: number;
  accounts_with_traffic: number;
  accounts: TrafficRankingRow[];
};

const configuredApiBase = import.meta.env.VITE_API_BASE_URL as string | undefined;
const apiBase = configuredApiBase === undefined ? "" : configuredApiBase;
const apiEndpoint = (path: string) => `${apiBase}${path}`;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiEndpoint(path), {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    if (response.status === 401 && path !== "/api/auth/login") {
      signalSessionExpired();
      throw new SessionExpiredError();
    }
    const body = (await response.json().catch(() => null)) as {
      detail?: unknown;
    } | null;
    if (body !== null && Object.prototype.hasOwnProperty.call(body, "detail")) {
      if (body.detail === null) throw new Error("空值");
      if (typeof body.detail === "string") throw new Error(body.detail);
      throw new Error(JSON.stringify(body.detail));
    }
    throw new Error(`请求失败（${response.status}）`);
  }
  return response.json() as Promise<T>;
}

export const api = {
  autoInspectionEventsURL: () => apiEndpoint("/api/inspection/automation/events"),
  taskEventsURL: (id: string) => apiEndpoint(`/api/tasks/${encodeURIComponent(id)}/events`),
  setupStatus: () => request<SetupStatus>("/api/setup/status"),
  initialize: (
    payload: {
      username: string;
      password: string;
      admin_base_url: string;
      admin_key: string;
    },
    setupToken?: string,
  ) =>
    request<SetupStatus>("/api/setup/initialize", {
      method: "POST",
      headers:
        setupToken === undefined
          ? undefined
          : {
              "X-Setup-Token": setupToken,
            },
      body: JSON.stringify(payload),
    }),
  session: () => request<SessionStatus>("/api/auth/session"),
  login: (payload: { username: string; password: string }) =>
    request<SessionStatus>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  logout: () => request<SessionStatus>("/api/auth/logout", { method: "POST" }),
  updateProfile: (payload: { username: string; current_password: string; new_password?: string }) =>
    request<SessionStatus>("/api/profile", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  overview: () => request<Overview>("/api/overview"),
  newAPIWorkspace: (platformId?: string) => {
    const query = platformId ? `?platform_id=${encodeURIComponent(platformId)}` : "";
    return request<NewAPIWorkspace>(`/api/newapi${query}`);
  },
  saveNewAPIPlatform: (payload: {
    id?: string;
    name: string;
    base_url: string;
    admin_key: string;
    user_id: string;
  }) =>
    request<NewAPIPlatform>("/api/newapi/platforms", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  deleteNewAPIPlatform: (platformId: string) =>
    request<{ deleted: boolean }>(`/api/newapi/platforms/${encodeURIComponent(platformId)}`, {
      method: "DELETE",
    }),
  refreshNewAPIPlatform: (platformId: string) =>
    request<NewAPIRemoteSnapshot>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/refresh`,
      { method: "POST" },
    ),
  saveNewAPIGroupBindings: (platformId: string, bindings: NewAPIGroupBindingUpdate[]) =>
    request<NewAPIGroupBinding[]>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/group-bindings`,
      { method: "PUT", body: JSON.stringify({ bindings }) },
    ),
  createNewAPIChannel: (
    platformId: string,
    payload: {
      name: string;
      sub2api_group_id: string;
      base_url: string;
      service_key: string;
      models: string[];
    },
  ) =>
    request<Record<string, unknown>>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channels`,
      { method: "POST", body: JSON.stringify(payload) },
    ),
  saveNewAPIModelPrices: (platformId: string, prices: NewAPIModelPrice[]) =>
    request<NewAPIRemoteSnapshot>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/model-prices`,
      { method: "PUT", body: JSON.stringify({ prices }) },
    ),
  trafficRanking: (filters: {
    timeRange: "1h" | "6h" | "24h" | "7d" | "30d";
    group?: string;
    sortBy: TrafficRankingSort;
  }) => {
    const query = new URLSearchParams({
      time_range: filters.timeRange,
      sort_by: filters.sortBy,
    });
    if (filters.group) query.set("group", filters.group);
    return request<TrafficRanking>(`/api/traffic/ranking?${query.toString()}`);
  },
  config: () => request<RuntimeConfig>("/api/config"),
  notificationStatus: () => request<NotificationStatus>("/api/notifications/status"),
  notificationQueue: () => request<NotificationQueueDetails>("/api/notifications/queue"),
  configureNotification: (payload: {
    app_id: string;
    client_secret: string;
    home_channel: string;
    home_channel_type: "c2c" | "group" | "channel";
  }) =>
    request<NotificationStatus>("/api/notifications/config", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  discoverNotificationTarget: (payload: {
    app_id: string;
    client_secret: string;
    target_type: "c2c" | "group" | "channel";
  }) =>
    request<Task>("/api/notifications/target-discovery", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  cancelNotificationTargetDiscovery: (taskId: string) =>
    request<{ cancelled: boolean }>(
      `/api/notifications/target-discovery/${encodeURIComponent(taskId)}`,
      { method: "DELETE" },
    ),
  testNotification: () =>
    request<{
      sent?: boolean;
      simulated?: boolean;
      detail?: string;
      message_id?: string | null;
      runtime_event_id: number;
      persisted: boolean;
    }>("/api/notifications/test", {
      method: "POST",
      body: JSON.stringify({
        message: "Sub2API Console 通知测试",
        dry_run: false,
      }),
    }),
  setMode: (mode: RuntimeMode) =>
    request<RuntimeConfig>("/api/config/mode", {
      method: "POST",
      body: JSON.stringify({ mode }),
    }),
  setProbesEnabled: (enabled: boolean) =>
    request<RuntimeConfig>("/api/config/probes", {
      method: "POST",
      body: JSON.stringify({ enabled }),
    }),
  setAccountDefaults: (payload: { concurrency: number; priority: number }) =>
    request<RuntimeConfig>("/api/config/account-defaults", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  setAdminTarget: (payload: {
    admin_base_url: string;
    admin_key: string;
    request_timeout_seconds: number;
  }) =>
    request<RuntimeConfig>("/api/config/target", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  policy: () => request<PolicySnapshot>("/api/policy"),
  updatePolicy: (payload: PolicyUpdatePayload) =>
    request<PolicySnapshot>("/api/policy", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  restorePolicyControl: () =>
    request<{
      restored: number;
      failed: number;
      remote_write: boolean;
      results: Array<{
        account_id: string;
        restored: boolean;
        changed?: boolean;
        error?: string;
      }>;
    }>("/api/policy/restore-control", { method: "POST" }),
  pricing: () => request<PricingSnapshot>("/api/pricing"),
  updatePricingConfig: (payload: PricingConfig) =>
    request<PricingSnapshot>("/api/pricing/config", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  applyPricing: () => request<Task>("/api/pricing/apply", { method: "POST" }),
  pricingBackups: () => request<PricingBackup[]>("/api/pricing/backups"),
  createPricingBackup: (name: string) =>
    request<PricingBackup>("/api/pricing/backups", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  restorePricingBackup: (backupId: string) =>
    request<Task>(`/api/pricing/backups/${encodeURIComponent(backupId)}/restore`, {
      method: "POST",
    }),
  calculateRevenue: (date: string) =>
    request<Task>("/api/pricing/revenue", {
      method: "POST",
      body: JSON.stringify({ date }),
    }),
  latestRevenue: () => request<Task | null>("/api/pricing/revenue/latest"),
  groups: () => request<GroupStatus[]>("/api/groups"),
  updateGroupPolicy: (id: string, payload: GroupPolicyOverrideUpdate) =>
    request<GroupStatus>(`/api/groups/${encodeURIComponent(id)}/policy`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  clearGroupPolicy: (id: string) =>
    request<GroupStatus>(`/api/groups/${encodeURIComponent(id)}/policy`, {
      method: "DELETE",
    }),
  setGroupExcluded: (id: string, excluded: boolean) =>
    request<GroupStatus>(`/api/groups/${encodeURIComponent(id)}/excluded`, {
      method: "PUT",
      body: JSON.stringify({ excluded }),
    }),
  upstreams: () => request<UpstreamSummary>("/api/upstreams"),
  upstreamConfiguration: (host: string) =>
    request<UpstreamConfiguration>(`/api/upstreams/${encodeURIComponent(host)}/configuration`),
  detectUpstream: (baseUrl: string) =>
    request<UpstreamDetection>("/api/upstreams/detect", {
      method: "POST",
      body: JSON.stringify({ base_url: baseUrl }),
    }),
  createUpstream: (payload: UpstreamCreatePayload) =>
    request<UpstreamConfiguration>("/api/upstreams", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  updateUpstreamConfiguration: (host: string, payload: UpstreamConfigurationUpdate) =>
    request<UpstreamConfiguration>(`/api/upstreams/${encodeURIComponent(host)}/configuration`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  upstreamGroups: (host: string, includeBound = true) =>
    request<UpstreamGroup[]>(
      `/api/upstreams/${encodeURIComponent(host)}/groups?include_bound=${includeBound ? "true" : "false"}`,
    ),
  upstreamGroupHistory: (host: string) =>
    request<UpstreamGroupChange[]>(
      `/api/upstreams/${encodeURIComponent(host)}/group-history?limit=200`,
    ),
  upstreamDeletePreview: (host: string) =>
    request<UpstreamDeletePreview>(`/api/upstreams/${encodeURIComponent(host)}/delete-preview`),
  deleteUpstream: (host: string, expectedAccountIds: string[]) =>
    request<Task>(`/api/upstreams/${encodeURIComponent(host)}/delete`, {
      method: "POST",
      body: JSON.stringify({
        confirmation_host: host,
        expected_account_ids: expectedAccountIds,
      }),
    }),
  accounts: () => request<AccountStatus[]>("/api/accounts"),
  groupAllocation: (groupId: string) =>
    request<GroupAllocation>(`/api/groups/${encodeURIComponent(groupId)}/allocation`),
  syncManagement: () => request<Task>("/api/management/sync", { method: "POST" }),
  syncAccountRates: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/rates/sync", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  revalidateAccounts: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/revalidate", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  validateAccountBaseURLs: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/base-url/validate", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  checkAccountConfiguration: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/configuration/check", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  repairAccountBaseURLs: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/base-url/repair", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  repairAccountUpstreamHosts: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/upstream-hosts/repair", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  repairAccountNames: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/names/repair", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  cleanupMissingBindings: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/missing-bindings/cleanup", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  syncUpstreamBalances: () => request<Task>("/api/upstreams/balances/sync", { method: "POST" }),
  repairUpstreamNames: () => request<Task>("/api/upstreams/names/repair", { method: "POST" }),
  syncUpstreamGroups: () => request<Task>("/api/upstreams/groups/sync", { method: "POST" }),
  account: (accountId: string) =>
    request<AccountDetail>(`/api/accounts/${encodeURIComponent(accountId)}`),
  accountDeletePreview: (accountId: string) =>
    request<AccountDeletePreview>(`/api/accounts/${encodeURIComponent(accountId)}/delete-preview`),
  deleteAccount: (preview: AccountDeletePreview) => {
    const body: Record<string, unknown> = {
      confirmation_account_id: preview.account_id,
      expected_management_base_url: preview.management_base_url,
    };
    if (preview.binding !== null) {
      body.expected_binding_id = preview.binding.id;
      body.expected_upstream_id = preview.binding.upstream_id;
      body.expected_upstream_host = preview.binding.upstream_host;
      body.expected_auth_host = preview.binding.auth_host;
      body.expected_upstream_key_id = preview.binding.upstream_key_id;
    }
    return request<Task>(`/api/accounts/${encodeURIComponent(preview.account_id)}/delete`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  setAccountControl: (accountId: string, action: AccountControlAction) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/control`, {
      method: "POST",
      body: JSON.stringify({ action }),
    }),
  accountModels: (accountId: string) =>
    request<{ models: string[] }>(`/api/accounts/${encodeURIComponent(accountId)}/models`),
  setAccountTestModel: (accountId: string, model: string | null) =>
    request<{ saved: boolean }>(`/api/accounts/${encodeURIComponent(accountId)}/test-model`, {
      method: "PUT",
      body: JSON.stringify({ model }),
    }),
  syncAccount: (
    accountId: string,
    payload: {
      name?: string;
      priority?: number;
      load_factor?: string;
      concurrency?: number;
      upstream_host?: string;
      base_url?: string;
      notes?: string | null;
    },
  ) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/sync`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  setAccountManualPriority: (
    accountId: string,
    priority: number,
    loadFactor: string,
    concurrency: number,
    syncBalanceMultiplier: boolean,
  ) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/manual-priority`, {
      method: "PUT",
      body: JSON.stringify({
        priority,
        load_factor: loadFactor,
        concurrency,
        sync_balance_multiplier: syncBalanceMultiplier,
      }),
    }),
  clearAccountManualPriority: (accountId: string) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/manual-priority`, {
      method: "DELETE",
    }),
  recentEvents: () => request<RunEvent[]>("/api/events?limit=8"),
  logs: (params: {
    kind: UnifiedLogKind;
    state: UnifiedLogState;
    level: UnifiedLogEventLevel;
    group: string;
    groupId: string;
    search: string;
    page: number;
    pageSize: number;
  }) => {
    const query = new URLSearchParams({
      kind: params.kind,
      state: params.state,
      level: params.level,
      group: params.group,
      group_id: params.groupId,
      search: params.search,
      page: String(params.page),
      page_size: String(params.pageSize),
    });
    return request<UnifiedLogPage>(`/api/logs?${query.toString()}`);
  },
  logCleanupStatus: () => request<LogCleanupStatus>("/api/config/log-cleanup"),
  updateLogCleanup: (payload: { enabled: boolean; retention_days: number }) =>
    request<LogCleanupStatus>("/api/config/log-cleanup", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  clearLogs: (retentionDays: number) =>
    request<LogCleanupResult>(`/api/logs?retention_days=${retentionDays}`, {
      method: "DELETE",
    }),
  runInspection: (payload?: { account_id?: string; group_name?: string }) =>
    request<Task>("/api/inspection/run", {
      method: "POST",
      body: JSON.stringify(payload ?? {}),
    }),
  autoInspection: () => request<AutoInspectionStatus>("/api/inspection/automation"),
  updateAutoInspection: (payload: AutoInspectionConfig) =>
    request<AutoInspectionStatus>("/api/inspection/automation", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  clearAutoInspectionHistory: () =>
    request<{ deleted: number }>("/api/inspection/automation/history", {
      method: "DELETE",
    }),
  cancelAutoInspection: () =>
    request<{ canceled: boolean; status: AutoInspectionStatus }>(
      "/api/inspection/automation/cancel",
      { method: "POST" },
    ),
  resumeAutoInspection: () =>
    request<AutoInspectionStatus>("/api/inspection/automation/resume", {
      method: "POST",
    }),
  runActiveProbe: (payload?: { account_id?: string; group_name?: string }) =>
    request<Task>("/api/inspection/probe", {
      method: "POST",
      body: JSON.stringify(payload ?? {}),
    }),
  modelCheckCapabilities: () => request<ModelCheckCapabilities>("/api/model-checks/capabilities"),
  modelCheckAccountStatuses: () =>
    request<ModelCheckAccountStatus[]>("/api/model-checks/account-statuses"),
  runModelCheck: (payload: ModelCheckRequest) =>
    request<Task>("/api/model-checks", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  authRecoveryConfig: () => request<PrivateAuthConfigStatus>("/api/auth-recovery/config"),
  verifyManualAuth: (payload: {
    host: string;
    auth_mode?: string;
    access_token?: string;
    refresh_token?: string;
    admin_key?: string;
    user_id?: string;
    username?: string;
    password?: string;
    save_to_vault?: boolean;
    entry?: string;
    headers?: Record<string, string>;
  }) =>
    request<ManualAuthVerifyResult>("/api/auth-recovery/manual", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  configureVaultEntry: (payload: {
    entry: string;
    username?: string | null;
    password?: string | null;
    hosts?: string[];
    headers?: Record<string, string>;
  }) =>
    request<{ entry: string; configured: boolean }>("/api/auth-recovery/vault-entry", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  deleteVaultEntry: (entry: string) => {
    const query = new URLSearchParams({ entry });
    return request<{ entry: string; deleted: boolean }>(
      `/api/auth-recovery/vault-entry?${query.toString()}`,
      { method: "DELETE" },
    );
  },
  runAuthRecovery: (host: string, entry: string) =>
    request<Task>("/api/auth-recovery/run", {
      method: "POST",
      body: JSON.stringify({ host, entry }),
    }),
  submitAuthCaptcha: (challengeId: string, captchaCode: string) =>
    request<CaptchaRecoveryResult>("/api/auth-recovery/captcha/submit", {
      method: "POST",
      body: JSON.stringify({
        challenge_id: challengeId,
        captcha_code: captchaCode,
      }),
    }),
  cancelAuthCaptcha: (challengeId: string) =>
    request<{ cancelled: boolean }>("/api/auth-recovery/captcha/cancel", {
      method: "POST",
      body: JSON.stringify({ challenge_id: challengeId }),
    }),
  runRateSync: (host: string, keyId?: string) => {
    const body = keyId === undefined ? { host } : { host, key_id: keyId };
    return request<Task>(`/api/upstreams/${encodeURIComponent(host)}/rate-sync`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  runBalanceSync: (host: string) =>
    request<Task>(`/api/upstreams/${encodeURIComponent(host)}/balance-sync`, {
      method: "POST",
    }),
  runUpstreamSync: () => request<Task>("/api/upstreams/sync", { method: "POST" }),
  requestTrace: (requestId: string) =>
    request<RequestTrace>(`/api/usage/trace/${encodeURIComponent(requestId)}`),
  systemLogs: (params: SystemLogSearchQuery) => {
    const query = new URLSearchParams({
      time_range: params.timeRange,
      start_time: params.startTime,
      end_time: params.endTime,
      host: params.host,
      level: params.level,
      request_id: params.requestId,
      client_request_id: params.clientRequestId,
      api_key_id: params.apiKeyId,
      account_id: params.accountId,
      platform: params.platform,
      model: params.model,
      q: params.keyword,
      page: String(params.page),
      page_size: String(params.pageSize),
    });
    return request<SystemLogPage>(`/api/ops/system-logs?${query.toString()}`);
  },
  // Keep the alert view bounded; an unbounded incident table can block the
  // browser when a long-running instance has accumulated a large history.
  alerts: () => request<AlertIncident[]>("/api/alerts?limit=200"),
  clearAlerts: () => request<{ deleted: number }>("/api/alerts", { method: "DELETE" }),
  alertPolicy: () => request<AlertPolicy>("/api/alerts/policy"),
  updateAlertPolicy: (payload: AlertPolicy) =>
    request<AlertPolicy>("/api/alerts/policy", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  evaluateAlerts: () => request<Task>("/api/alerts/evaluate", { method: "POST" }),
  task: (id: string) => request<Task>(`/api/tasks/${id}`),
  onboard: (payload: OnboardingRequest) =>
    request<Task>("/api/onboarding", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  onboardBatch: (items: OnboardingRequest[]) =>
    request<Task>("/api/onboarding/batch", {
      method: "POST",
      body: JSON.stringify({ items }),
    }),
  prepareOnboarding: (host: string) =>
    request<OnboardingContext>("/api/onboarding/prepare", {
      method: "POST",
      body: JSON.stringify({ host }),
    }),
  previewUnboundUpstreamKeys: (host: string) =>
    request<KeyCleanupPreview>("/api/onboarding/keys/cleanup-preview", {
      method: "POST",
      body: JSON.stringify({ host }),
    }),
  cleanupUnboundUpstreamKeys: (host: string, keyIds: string[]) =>
    request<Task>("/api/onboarding/keys/cleanup", {
      method: "POST",
      body: JSON.stringify({ host, key_ids: keyIds }),
    }),
  onboardingProbeModels: (host: string, groupId: string) =>
    request<{ models: string[] }>("/api/onboarding/probe/models", {
      method: "POST",
      body: JSON.stringify({ host, group_id: groupId }),
    }),
  runOnboardingProbe: (host: string, groupId: string, model: string) =>
    request<ProbeResult>("/api/onboarding/probe", {
      method: "POST",
      body: JSON.stringify({ host, group_id: groupId, model }),
    }),
};
