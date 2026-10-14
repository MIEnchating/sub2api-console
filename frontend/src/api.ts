import {
  SessionExpiredError,
  signalSessionExpired,
  currentSessionGeneration,
  assertCurrentSession,
} from "./lib/session-auth";
import type {
  ImportModelSettings,
  TemplateLibrary,
  TemplateConfig,
  WorkbenchAccount,
  WorkbenchAccountActionPreview,
  WorkbenchAccountActionInput,
  WorkbenchTemplate,
  WorkbenchPreview,
  WorkbenchProxy,
  WorkbenchRun,
  WorkbenchArtifact,
  WorkbenchInput,
  WorkbenchMaintenance,
  MaintenanceSettings,
  MaintenancePreview,
  RememberedAccount,
} from "./features/account-workbench/types";

export type Overview = {
  database_available: boolean;
  account_count: number | null;
  group_count: number | null;
  open_alerts: number;
  recent_runs: number;
  last_activity: string | null;
  mode: string;
};

export type RuntimeConfig = {
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

export type TaskConcurrencySettings = {
  pools: { id: string; limit: number; queue_capacity: number; running: number; waiting: number }[];
  queue_capacity: number;
  version: string;
};

export type TaskConcurrencyUpdate = {
  limits: Record<string, number>;
  queue_capacity: number;
  version: string;
};

export type RuntimeMode = "监控模式" | "完全模式";

export type AccountCreationPolicy = {
  models: string[];
  concurrency: number;
  load_factor: string | null;
  priority: number;
  pool_mode: boolean;
  pool_mode_retry_count: number;
  pool_mode_retry_status_codes: number[];
};

type AccountCreationGroupSettings = AccountCreationPolicy & {
  group_id: string;
};

export type AccountCreationSettings = {
  default: AccountCreationPolicy;
  groups: AccountCreationGroupSettings[];
  platform_probe_models: Record<string, string>;
};

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

export type NewAPIGroupBindingUpdate = Omit<NewAPIGroupBinding, "platform_id"> & {
  sub2api_ratio: string;
};

export type NewAPIWorkspace = {
  platforms: NewAPIPlatform[];
  local_groups: NewAPILocalGroup[];
  bindings: NewAPIGroupBinding[];
  sub2api_base_url: string;
};

export type NavigationPreferences = { hidden_item_ids: string[] | null; version: string };

export type NewAPIChannelGroup = { id: string; name: string; channel_ids: string[] };
export type NewAPIChannelGroups = { groups: NewAPIChannelGroup[]; version: string };

export type NewAPIChannel = {
  id: string;
  name: string;
  type: number;
  status: number;
  models: string[];
  groups: string[];
  version: string;
};

export type NewAPIChannelLookup = {
  items: NewAPIChannel[];
  failures: Array<{ id: string; code: string; message: string }>;
};

export type NewAPIChannelModelChange = {
  action: "add" | "remove";
  models: string[];
  version: string;
};

export type AccountModelAutomationTaskRequest = {
  group_ids: string[];
  platform_id: string;
  channels: Array<{ id: string; version: string }>;
  selected_models?: string[];
  publish_missing: boolean;
  price_enabled: boolean;
  pricing_source: "official" | "cached";
  dry_run: boolean;
  preview_task_id?: string;
};

export type AccountModelAutomationSchedule = {
  id: string;
  name: string;
  request: Omit<AccountModelAutomationTaskRequest, "dry_run" | "preview_task_id">;
  enabled: boolean;
  schedule_type?: "interval" | "daily";
  daily_times?: string[];
  timezone?: string;
  interval_minutes: number;
  last_run_at: string | null;
  next_run_at: string | null;
};

export type AccountModelAutomationScheduleInput = {
  id?: string;
  name: string;
  group_ids: string[];
  platform_id: string;
  channels: Array<{ id: string; version: string }>;
  selected_models?: string[];
  publish_missing: boolean;
  price_enabled: boolean;
  pricing_source: "official" | "cached";
  enabled: boolean;
  schedule_type?: "interval" | "daily";
  daily_times?: string[];
  timezone?: string;
  interval_minutes: number;
};

export type AccountModelAutomationTaskGroup = {
  id: string;
  name: string;
  schedule_ids: string[];
};

export type AccountModelAutomationTaskGroupInput = {
  id?: string;
  name: string;
  schedule_ids: string[];
};

export type NewAPIChannelKey = {
  key_id: string;
  name: string;
  group_id: string;
  endpoints: NewAPIChannelEndpoint[];
};

export type NewAPIChannelEndpoint = {
  name: string;
  base_url: string;
  default: boolean;
};

export type NewAPIChannelKeyRequest =
  | {
      sub2api_group_id: string;
      credential_source: "vault";
      vault_entry: string;
    }
  | {
      sub2api_group_id: string;
      credential_source: "custom";
      username: string;
      password: string;
    };

export type NewAPIRemoteGroup = {
  id: string;
  name: string;
  ratio: string | null;
};

export type NewAPIModelPrice = {
  model: string;
  model_price?: string;
  input_ratio: string;
  completion_ratio: string;
  input_price?: string;
  completion_price?: string;
  cache_create_price?: string;
  cache_read_price?: string;
  billing_mode?: string;
  billing_expr?: string;
  cache_ratio?: string;
  create_cache_ratio?: string;
  create_cache_1h_ratio?: string;
  image_ratio?: string;
  audio_ratio?: string;
  audio_completion_ratio?: string;
};

type NewAPIToolPrice = {
  tool: string;
  price: string;
};

export type ModelTimePricing = {
  timezone: string;
  weekdays_only: boolean;
  periods: Array<{ start_time: string; end_time: string }>;
  peak: { input_price: string; output_price: string; cache_read_price: string };
};

export type OfficialPriceTier = {
  label: string;
  condition: string;
  input_price: string;
  output_price: string;
  cache_read_price?: string;
  cache_write_price?: string;
  cache_write_1h_price?: string;
};

export type Sub2APIModelPrice = {
  sync_error?: string;
  source_scope?: string;
  price_tiers?: OfficialPriceTier[];
  billing_expr?: string;
  source?: "remote" | "sub2api" | "official";
  source_url?: string;
  time_pricing?: ModelTimePricing;
  model: string;
  input_price: string;
  output_price: string;
  image_input_price?: string;
  image_output_price?: string;
  image_output_unit?: "token" | "image";
  provider?: string;
  mode?: string;
  input_ratio?: string;
  model_ratio: string;
  completion_ratio: string;
  cache_write_price?: string;
  cache_write_1h_price?: string;
  cache_read_price?: string;
  cache_ratio?: string;
  create_cache_ratio?: string;
  create_cache_1h_ratio?: string;
  image_ratio?: string;
  long_context_threshold?: number;
  long_context_threshold_inclusive?: boolean;
  long_context_input_price?: string;
  long_context_output_price?: string;
  long_context_cache_write_price?: string;
  long_context_cache_write_1h_price?: string;
  long_context_cache_read_price?: string;
};

export type RemoteModelPricingSource = {
  stale?: boolean;
  warning?: string;
  source_url: string;
  content: string;
  fetched_at: string;
  size_bytes: number;
  sha256: string;
};

export type ModelPriceCatalog = {
  models: Sub2APIModelPrice[];
  missing_models?: string[];
  fetched_at?: string;
  expires_at?: string;
  stale?: boolean;
  warning?: string;
};

type NewAPIUpstreamPriceCatalog = {
  host: string;
  name: string;
  upstream_type: string;
  models: NewAPIModelPrice[];
};

export type NewAPIRemoteSnapshot = {
  groups: NewAPIRemoteGroup[];
  models: NewAPIModelPrice[];
  unset_models: NewAPIModelPrice[];
  tool_prices: NewAPIToolPrice[];
  references: NewAPIModelPrice[];
  newapi_models?: NewAPIModelPrice[];
  sub2api_models?: Sub2APIModelPrice[];
  upstream_prices?: NewAPIUpstreamPriceCatalog[];
  upstream_price_warning?: string;
  differences: Array<{
    model: string;
    kind: "missing_in_model_plaza" | "missing_in_platform" | "ratio_mismatch";
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

export type UpstreamBindingCleanup = {
  upstream_id: string;
  account_id: string;
  upstream_key_id: string;
  upstream_group_id: string;
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

export type UpstreamAccountRequestUsage = {
  account_id: string;
  account_name: string;
  request_count: number;
  share: number;
};

export type UpstreamUsage = {
  upstream_id: string;
  request_count: number;
  request_window_days: number;
  account_request_usage: UpstreamAccountRequestUsage[];
};

type UpstreamHost = {
  excluded?: boolean;
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
  concurrency_limit?: number | null;
  concurrency_status?: "known" | "unlimited" | "unknown" | "stale";
  concurrency_checked_at?: string | null;
  allocated_concurrency?: number | null;
  target_concurrency?: number | null;
  last_auth_success_method?: string | null;
  last_auth_recovery_method?: string | null;
  last_auth_success_at?: string | null;
  spending?: { total: string; started_at: string; updated_at: string } | null;
  request_count?: number;
  request_window_days?: number;
  account_request_usage?: UpstreamAccountRequestUsage[];
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

export type UpstreamExclusionPreview = UpstreamDeletePreview & {
  upstream_id: string;
  excluded: boolean;
};
export type UpstreamExclusionRequest = {
  upstream_id: string;
  excluded: boolean;
  stop_scheduling: boolean;
  expected_account_ids: string[];
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

type UpstreamGroupBindingAuditAccount = {
  id: string;
  name: string | null;
};

export type UpstreamGroupBindingAuditItem = {
  upstream_id: string;
  host: string;
  group_id: string | null;
  group_name: string;
  account_count: number;
  accounts: UpstreamGroupBindingAuditAccount[];
  status: "present" | "missing" | "unknown";
  reason: string | null;
};

export type UpstreamGroupBindingAudit = {
  items: UpstreamGroupBindingAuditItem[];
  total_bindings: number;
  present: number;
  missing: number;
  unknown: number;
};

export type UpstreamGroupChange = {
  id: number;
  upstream_id: string;
  group_id: string;
  group_name: string;
  effective_rate?: string | null;
  change_type: "added" | "removed";
  changed_at: string;
};

export type UpstreamConfiguration = {
  name_mode?: "auto" | "manual";
  name_sync_task_id?: string;
  name_sync_error?: string;
  upstream_id: string;
  host: string;
  name: string;
  base_url: string;
  account_base_url: string;
  upstream_type: string;
  auth_mode: string;
  entry?: string | null;
  recharge_rate: string;
  raw_balance: string | null;
  balance: string | null;
  concurrency_limit?: number | null;
  concurrency_status?: UpstreamSummary["hosts"][number]["concurrency_status"];
  concurrency_checked_at?: string | null;
  allocated_concurrency?: number | null;
  target_concurrency?: number | null;
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
  name_mode?: "auto" | "manual";
  host?: string;
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
  animation?: AnimationResult;
  status: "passed" | "failed" | "skipped";
  message: string;
  request_model: string;
  actual_model: string;
  response_text?: string;
  response_json?: string;
  latency_ms: number;
  http_status: number;
  temporary_key?: boolean;
};

type AnimationPromptTemplate = { id: string; name: string; prompt: string };
export type AnimationPromptSettings = {
  prompt: string;
  version: number;
  templates: AnimationPromptTemplate[];
  default_id: string;
};

export type OnboardingProbeMode = "stream" | "default" | "animation" | "candy";

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
  test_models?: string[];
  model_mapping?: Record<string, string>;
  schedulable?: boolean;
  waiting_for_capacity?: boolean;
  allocation_override?: boolean;
};

export type UpstreamAllocationSetting = {
  revision: string;
  target_id: string;
  upstream_id: string;
  override: boolean | null;
  selected: boolean;
  effective: boolean;
  global_enabled: boolean;
  source: "account" | "upstream" | "policy";
};
export type UpstreamAllocationTarget = "accounts" | "upstreams";

export type PolicySnapshot = {
  revision?: string;
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
  expected_revision?: string;
  group_strategies?: Record<string, string | null>;
};

export type AutoInspectionConfig = {
  enabled: boolean;
  interval_seconds: number;
  account_rate_sync_interval_seconds?: number;
  account_rate_sync_batch_size?: number;
  account_rate_sync_batch_percent?: number;
};

type InspectionRoundSummary = {
  channels: number;
  probed: number;
  samples: number;
  fused: number;
  recovered: number;
  auth_recovered?: number;
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
  last_status: "succeeded" | "partial" | "failed" | "cancelled" | null;
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
    status: "running" | "succeeded" | "partial" | "failed" | "cancelled";
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
  task_id?: string;
  enabled: boolean;
  retention_days: number;
  last_run_at: string | null;
  next_run_at: string | null;
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

export type DictionaryKind =
  | "platform"
  | "group"
  | "account_type"
  | "upstream_type"
  | "auth_status"
  | "scheduling_strategy"
  | "task_status"
  | "account_status"
  | "alert_status"
  | "kuma_monitor_type";
export type DictionaryEntry = {
  id: string;
  kind: DictionaryKind;
  name: string;
  value: string;
  description: string;
  enabled: boolean;
  sort_order: number;
  created_at: string;
  updated_at: string;
  version: number;
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
  assigned_concurrency: number | null;
  channels: GroupAllocationChannel[];
};

export type GroupProbeModels = {
  group_id: string;
  group_name: string;
  models: string[];
  account_count: number;
  accounts_with_models: number;
  complete: boolean;
};

export type PricingConfig = {
  enabled: boolean;
  profit_margin: number;
  exchange_group_sets: string[][];
  exchange_group_set_names: string[];
  group_min_cost_multipliers?: Record<string, string>;
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

export type PricingChangeGroup = {
  id: string;
  name: string;
};

export type PricingAccountChange = {
  account_id: string;
  account_name: string;
  before: PricingChangeGroup[];
  after: PricingChangeGroup[];
};

export type PricingChangeRecord = {
  id: number;
  actor: string;
  created_at: string;
  account_count?: number;
  group_link_count?: number;
  changes: PricingAccountChange[] | null;
};

export type RevenueRow = {
  account_id: string;
  account_name: string;
  local_group: string;
  upstream_host: string;
  upstream_key_name: string;
  account_cost: string | null;
  actual_cost: string | null;
  upstream_raw_cost: string | null;
  recharge_rate: string | null;
  upstream_cost: string | null;
  difference: string | null;
  revenue: string | null;
  category: "计费异常" | "正常" | "无法核对";
  note: string;
  attribution_level: "key" | "unavailable";
};

type RevenueSummary = {
  group: string;
  accounts: number;
  account_cost: string;
  actual_cost: string;
  upstream_raw_cost: string;
  upstream_cost: string;
  difference: string;
  revenue: string;
};

export type RevenueReport = {
  report_date: string;
  timezone: string;
  tolerance: string;
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
  animation_enabled?: boolean | null;
  animation_pass_multiplier?: number | null;
  animation_fail_multiplier?: number | null;
  animation_failure_action?: "ignore" | "degrade" | "fuse" | null;
};

export type GroupPolicyOverrideUpdate = {
  enabled: boolean;
  strategy: "balanced" | "price_first" | "speed_first" | "reliability" | null;
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
  animation_enabled: boolean;
  animation_pass_multiplier: number;
  animation_fail_multiplier: number;
  animation_failure_action: "ignore" | "degrade" | "fuse";
};

export type AccountRecovery = {
  evaluated_at: string;
  ready: boolean;
  conditions: Array<{ code: string; met: boolean; detail: string }>;
};

export type AccountStatus = {
  probe_placeholder_at?: string | null;
  probe_placeholder?: {
    group_name?: string;
    request_model?: string;
    result?: string;
    attempt?: number;
    preview?: string;
  } | null;
  ignore_cost_wall?: boolean;
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
  groups_locked?: boolean;
  manual_sync_balance_multiplier?: boolean;
  manual_fused?: boolean;
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
  evidence_pending?: boolean;
  recovery?: AccountRecovery | null;
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
  health_evaluated_at?: string;
  health_evidence_at?: string | null;
  short_score: number | null;
  long_score: number | null;
  sample_count: number;
  short_sample_count?: number | null;
  long_sample_count?: number;
  model_check_status?: ModelCheckAccountStatus["status"] | "loading" | "unavailable" | null;
  model_check?: ModelCheckAccountStatus;
  stability?: AccountQualityStatistics;
  model_check_checked_at?: string | null;
  recent_results: AccountRecentResult[];
  ttfb_p50_ms: number | null;
  ttfb_p95_ms: number | null;
  weight: number | null;
};

export type AccountRecentResult = {
  id?: string;
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

export type AccountControlState = Pick<
  AccountStatus,
  | "id"
  | "name"
  | "health"
  | "health_status"
  | "routing_state"
  | "paused"
  | "schedulable"
  | "manual_priority"
>;

export type AccountDetail = AccountStatus & {
  metadata: Record<string, unknown>;
  group_rates: Record<string, string | null>;
  group_ids: Record<string, string | null>;
  bindings: AccountBinding[];
  test_models: string[];
};

export type AccountGroupsPreview = {
  account_id: string;
  current_group_ids: string[];
  groups: Array<{ id: string; name: string }>;
  target_version: string;
};

export type AccountGroupsInput = {
  group_ids: string[];
  expected_group_ids: string[];
  target_version: string;
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

export type AccountDeleteBatchPreview = {
  accounts: AccountDeletePreview[];
  account_count: number;
  upstream_key_count: number;
};

type AccountModelCoverage = {
  model: string;
  account_count: number;
};

export type AccountModelSyncAccount = {
  enabled_models?: string[];
  account_id: string;
  account_name: string;
  platform?: string;
  models: string[];
  probe_model: string;
};

export type AccountModelSyncPreview = {
  account_count: number;
  accounts_with_catalog: number;
  blocked_patterns: string[];
  blocked_models: string[];
  models: AccountModelCoverage[];
  accounts: AccountModelSyncAccount[];
  fingerprint: string;
};

export type AccountModelSelection = {
  account_id: string;
  models: string[];
  manual_models?: string[];
};

export type AccountModelSyncSettings = {
  blocked_patterns: string[];
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

export type BrowserFrame = {
  image?: string;
  width: number;
  height: number;
};

export type BrowserInput = {
  kind: "click" | "text" | "key" | "scroll";
  x?: number;
  y?: number;
  text?: string;
  key?: string;
  delta?: number;
  shift?: boolean;
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
  input_tokens?: string | null;
  output_tokens?: string | null;
  cache_read_tokens?: string | null;
  cache_write_tokens?: string | null;
  usage_available?: boolean;
  summary: string | null;
  observed_at: string | null;
  source: string;
  payload: Record<string, unknown>;
};

export type SystemLogSearchQuery = {
  requestId: string;
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
  multiplier_increase_enabled: boolean;
  multiplier_decrease_enabled: boolean;
  cost_traffic_enabled: boolean;
  balance_enabled: boolean;
  probe_enabled: boolean;
  routing_breaker_enabled: boolean;
  routing_degraded_enabled: boolean;
  routing_degraded_types: Array<"health_score" | "gateway_error_rate" | "latency" | "other">;
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
  recovery_notification_types: Array<
    | "configuration"
    | "auth"
    | "rate_sync"
    | "balance"
    | "probe"
    | "routing_breaker"
    | "routing_degraded"
    | "routing_survivor"
    | "group_unavailable"
    | "group_survivor"
    | "cost_traffic"
    | "apply_failure"
  >;
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
  status: "queued" | "running" | "waiting_input" | "succeeded" | "partial" | "failed" | "cancelled";
  progress: number;
  message: string;
  result: Record<string, unknown>;
  display_account_names?: Record<string, string>;
  created_at: string;
  updated_at: string;
};

export type TaskSummary = Omit<Task, "result" | "display_account_names"> & { system_info: true };

export type SystemMetrics = {
  sampled_at: string;
  cpu: {
    usage_percent: number;
    logical_cores: number;
  };
  memory: {
    total_bytes: number;
    used_bytes: number;
    available_bytes: number;
    usage_percent: number;
  };
  disk: {
    total_bytes: number;
    used_bytes: number;
    available_bytes: number;
    usage_percent: number;
  };
};

export type AnimationTarget = { account_id: string; model: string; models?: string[] };
type TerminalContinuityVerdict = "normal" | "suspected" | "inconclusive" | "error";
export type TerminalContinuityResult = {
  round_results?: TerminalContinuityRound[];
  account_id: string;
  account_name: string;
  model: string;
  response_model?: string;
  request_id: string;
  verdict: TerminalContinuityVerdict;
  response?: string;
  error?: string;
  duration_ms: number;
  completed_at: string;
};
export type TerminalContinuityRound = {
  round: number;
  verdict: TerminalContinuityVerdict;
  request_id: string;
  response?: string;
  response_model?: string;
  error?: string;
  duration_ms: number;
  completed_at: string;
};
export type DetectionTask = {
  models?: string[];
  id: string;
  version: number;
  name: string;
  group_ids: string[];
  model: string;
  prompt?: string;
  animation?: boolean;
  precheck: boolean;
  precheck_questions?: PrecheckQuestionID[];
  terminal: boolean;
  terminal_rounds: number;
  automatic: boolean;
  schedule_type: "interval" | "daily";
  interval_minutes: number;
  daily_times?: string[];
  timezone?: string;
  timeout_seconds: number;
  concurrency?: number;
  running?: boolean;
  next_at?: string;
  last_task_id?: string;
  last_error?: string;
};
export type PrecheckQuestionID = "candy";
export type AnimationCustomEndpoint = {
  models?: string[];
  base_url: string;
  api_key: string;
  platform: "openai" | "anthropic" | "gemini";
  model: string;
};
export type AnimationRequest = {
  prompt?: string;
  precheck_questions?: PrecheckQuestionID[];
  mode?: "animation" | "precheck";
  targets: AnimationTarget[];
  timeout_seconds: number;
  custom?: AnimationCustomEndpoint;
};
export type AnimationQualitySettings = {
  enabled: boolean;
  account_id: string;
  model: string;
  timeout_seconds: number;
  version: number;
};
export type AnimationQualityPrediction = {
  source?: "model_review";
  explanation?: string;
  reviewer_account_id?: string;
  settings_version?: number;
  reviewed_at?: string;
  response_model?: string;
  status: "normal" | "suspicious" | "unknown";
  score: number | null;
  reason?: string;
  benchmark: string;
  model_version?: string;
  model_sha256?: string;
};
type AnimationQualityAnnotation = {
  label: AnimationQualityPrediction["status"];
  reason: string;
  source_group: string;
  version: number;
  updated_at: string;
  updated_by: string;
};
export type AnimationQualityReview = {
  prediction: AnimationQualityPrediction;
  annotation?: AnimationQualityAnnotation;
  sample_id: string;
  benchmark: string;
  source_sha256: string;
  prompt_sha256: string;
};
export type AnimationQualitySample = {
  id: string;
  task_id: string;
  account_id: string;
  request_id: string;
  benchmark: string;
  prompt: string;
  prompt_sha256: string;
  html: string;
  source_sha256: string;
  prediction: AnimationQualityPrediction;
  annotation: AnimationQualityAnnotation;
  created_at: string;
};
export type AnimationResult = {
  quality?: AnimationQualityPrediction;
  source_task_id?: string;
  source_task_status?: Task["status"];
  manual_verdict?: AnimationManualVerdict;
  mode?: "animation" | "precheck";
  precheck?: {
    verdict: "passed" | "not_passed" | "inconclusive" | "error";
    profile_version: string;
    questions: Array<{
      id: string;
      verdict: "passed" | "not_passed" | "inconclusive" | "error";
      answer?: string;
      answer_truncated?: boolean;
      error?: string;
      request_id: string;
    }>;
  };
  account_id: string;
  account_name: string;
  endpoint?: string;
  platform?: "openai" | "anthropic" | "gemini";
  model: string;
  response_model?: string;
  request_id: string;
  status: "succeeded" | "failed";
  phase?: "queued" | "running" | "generating" | "reviewing" | "completed";
  html?: string;
  source?: string;
  source_truncated?: boolean;
  prompt?: string;
  reasoning_effort?: string;
  usage?: {
    input_tokens?: number;
    output_tokens?: number;
    total_tokens?: number;
    reasoning_tokens?: number;
  };
  generation_duration_ms?: number;
  svg?: string;
  error?: string;
  retry_count?: number;
  duration_ms: number;
  completed_at: string;
};
export type AnimationManualVerdict = {
  verdict: "passed" | "not_passed" | "unmarked";
  updated_at: string;
  updated_by: string;
};
export type AnimationSchedule = {
  models?: string[];
  prompt?: string;
  schedule_type?: "interval" | "daily";
  daily_time?: string;
  daily_times?: string[];
  timezone?: string;
  precheck_questions?: PrecheckQuestionID[];
  mode?: "animation" | "precheck" | "both";
  account_id: string;
  enabled: boolean;
  model: string;
  interval_minutes: number;
  timeout_seconds: number;
  version: number;
  next_at?: string;
  last_task_id?: string;
  last_error?: string;
};

export type ModelCheckCapabilities = {
  claude_standards: string[];
  sol_models: string[];
  astra_models?: string[];
};

export type ModelCheckAccountStatus = {
  confidence?: AccountQualityStatistics;
  account_id: string;
  status: "consistent" | "inconsistent" | "inconclusive" | "not_checked";
  checked_at: string;
  task_id: string;
};

export type AccountQualityWindow = {
  score: number | null;
  samples: number;
  passed: number;
  failed: number;
  inconclusive: number;
};

export type AccountQualityStatistics = {
  short: AccountQualityWindow;
  long: AccountQualityWindow;
  evaluated_at: string;
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
  input_tokens: number | null;
  output_tokens: number | null;
  cache_read_tokens: number | null;
  cache_write_tokens: number | null;
  usage_available: boolean;
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

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const generation = currentSessionGeneration();
  try {
    const response = await fetch(apiEndpoint(path), {
      ...init,
      credentials: "include",
      headers: { "Content-Type": "application/json", ...init?.headers },
    });
    assertCurrentSession(generation);
    if (!response.ok) {
      if (response.status === 401 && path !== "/api/auth/login") {
        signalSessionExpired();
        throw new SessionExpiredError();
      }
      const body = (await response.json().catch(() => null)) as {
        code?: unknown;
        detail?: unknown;
      } | null;
      assertCurrentSession(generation);
      const code =
        typeof body?.code === "string" && body.code !== "" ? body.code : "request_failed";
      if (body !== null && Object.prototype.hasOwnProperty.call(body, "detail")) {
        if (body.detail === null) throw new ApiError("空值", code, response.status);
        if (typeof body.detail === "string") throw new ApiError(body.detail, code, response.status);
        throw new ApiError(JSON.stringify(body.detail), code, response.status);
      }
      throw new ApiError(`请求失败（${response.status}）`, code, response.status);
    }
    if (response.status === 204 || response.headers.get("content-length") === "0") {
      return undefined as T;
    }
    const result = (await response.json()) as T;
    assertCurrentSession(generation);
    return result;
  } catch (error) {
    // The current request may itself have signalled expiry and changed the generation.
    if (!(error instanceof SessionExpiredError)) assertCurrentSession(generation);
    throw error;
  }
}

export type KumaConfig = {
  base_url: string;
  username: string;
  api_key_configured: boolean;
  management_configured: boolean;
  revision: number;
};
export type KumaTemplate = {
  body_encoding?: string;
  monitoring?: KumaTemplateMonitoring;
  url_configured?: boolean;
  url_redacted?: boolean;
  request_profile?: string;
  model?: string;
  id: string;
  revision: number;
  name: string;
  method: string;
  auth_method: string;
  headers_configured: boolean;
  body_configured: boolean;
  auth_configured: boolean;
};
export type KumaTemplateDistributionPreview = {
  template_id: string;
  revision: number;
  config_revision: number;
  scope: string;
  items: { monitor_id: number; name: string; template_revision: number; reason: string }[];
};
export type KumaTemplateDetail = KumaTemplate & { headers: string; body: string };
export type KumaTemplateInput = {
  body_encoding?: string;
  monitoring?: KumaTemplateMonitoring;
  clear_url?: boolean;
  request_profile?: string;
  model?: string;
  revision: number;
  name: string;
  method: string;
  headers: string;
  body: string;
  auth_method: string;
  auth_username: string;
  auth_password: string;
  clear_headers: boolean;
  clear_body: boolean;
  clear_auth: boolean;
};

type KumaTemplateMonitoring = {
  type: string;
  url: string;
  interval: number;
  timeout: number;
  retry_interval: number;
  max_retries: number;
  max_redirects: number;
  accepted_status_codes: string[];
  ignore_tls: boolean;
  upside_down: boolean;
  hostname: string;
  port: number;
  keyword: string;
  dns_record_type: string;
  dns_resolver: string;
};

export type KumaConfigInput = {
  base_url: string;
  api_key: string;
  username: string;
  password: string;
  otp: string;
  disable_management: boolean;
  revision: number;
};

type KumaMonitorOptions = {
  hostname: string;
  port: number;
  keyword: string;
  dns_record_type: string;
  dns_resolver: string;
  method: string;
  timeout: number;
  retry_interval: number;
  max_retries: number;
  resend_interval: number;
  max_redirects: number;
  ignore_tls: boolean;
  upside_down: boolean;
  accepted_status_codes: string[];
  notification_ids: number[];
  auth_method: string;
  headers_configured?: boolean;
  body_configured?: boolean;
  auth_configured?: boolean;
  headers?: string;
  body?: string;
  auth_username?: string;
  auth_password?: string;
  clear_headers?: boolean;
  clear_body?: boolean;
  clear_auth?: boolean;
};
export type KumaResourceKind = "notifications" | "maintenance" | "status-pages";
type KumaNotification = {
  name: string;
  type: string;
  default: boolean;
  active: boolean;
  endpoint: string;
  token: string;
  username: string;
  password: string;
  endpoint_configured?: boolean;
  token_configured?: boolean;
  username_configured?: boolean;
  password_configured?: boolean;
  chat_id: string;
  smtp_host: string;
  smtp_port: number;
  smtp_secure: boolean;
  from: string;
  to: string;
  topic: string;
};
type KumaMaintenance = {
  title: string;
  description: string;
  strategy: string;
  active: boolean;
  timezone: string;
  start: string;
  end: string;
  cron: string;
  duration_minutes: number;
  interval_days: number;
  start_time: string;
  end_time: string;
  weekdays: number[];
  days_of_month: number[];
  last_day?: boolean;
  monitor_ids: number[];
  status_page_ids: number[];
  status?: string;
};
type KumaPublicGroup = {
  id?: number;
  name: string;
  monitorList: { id: number; sendUrl: boolean; url?: string }[];
};
type KumaStatusPage = {
  title: string;
  slug: string;
  description: string;
  theme: string;
  footer: string;
  show_tags: boolean;
  show_powered_by: boolean;
  show_certificate_expiry: boolean;
  domains: string[];
  groups: KumaPublicGroup[];
};
export type KumaResource = {
  id: number;
  name: string;
  type: string;
  active: boolean;
  revision: string;
  association_revision: string;
  notification?: KumaNotification;
  maintenance?: KumaMaintenance;
  status_page?: KumaStatusPage;
};
export type KumaResourceList = {
  config: KumaConfig;
  items: KumaResource[];
  monitors: KumaMonitor[];
  status_pages: { id: number; title: string }[];
};
export type KumaResourceWrite = {
  action: "create" | "edit" | "delete" | "test" | "pause" | "resume";
  config_revision: number;
  revision: string;
  association_revision: string;
  notification?: KumaNotification;
  maintenance?: KumaMaintenance;
  status_page?: KumaStatusPage;
};

export type KumaMonitor = {
  target?: string;
  options?: KumaMonitorOptions;
  id: number;
  key: string;
  name: string;
  type: string;
  url: string;
  url_redacted: boolean;
  active: boolean;
  parent: number | null;
  interval: number;
  status: number | null;
  response_time: number | null;
  certificate_days: number | null;
  uptime: number | null;
  revision: string;
  template_id?: string;
  template_revision?: number;
  template_model?: string;
  template_name?: string;
  template_body_encoding?: string;
};

type KumaMonitorInput = {
  template_id?: string;
  template_revision?: number;
  template_auth_override?: boolean;
  template_settings_override?: boolean;
  template_model?: string;
  template_clear?: boolean;
  template_retain?: boolean;
  name: string;
  type: string;
  options?: KumaMonitorOptions;
  url: string;
  interval: number;
  parent: number | null;
};

export type KumaWriteInput = {
  config_revision: number;
  revision: string;
  action: "create" | "edit" | "pause" | "resume" | "delete";
  monitor?: KumaMonitorInput;
};

export type KumaSnapshot = { config: KumaConfig; monitors: KumaMonitor[]; warning: string };

export type GroupOption = Pick<
  GroupStatus,
  "id" | "name" | "platform" | "platforms" | "rate_multiplier"
>;
export type GroupMembershipOption = Pick<GroupStatus, "id" | "name" | "account_count">;
export type GroupConfiguration = Pick<
  GroupStatus,
  "id" | "name" | "strategy_source" | "participation_status" | "override"
>;
export type AccountOption = Pick<
  AccountStatus,
  "id" | "name" | "groups" | "platform" | "account_type" | "upstream_type"
>;
export type UpstreamOption = Pick<
  UpstreamSummary["hosts"][number],
  "upstream_id" | "host" | "name" | "upstream_type" | "auth_status" | "account_count"
>;

export const api = {
  groupMembershipOptions: (signal?: AbortSignal) =>
    request<GroupMembershipOption[]>("/api/catalog/group-memberships", { signal }),
  groupConfigurations: (signal?: AbortSignal) =>
    request<GroupConfiguration[]>("/api/catalog/group-configurations", { signal }),
  groupOptions: (signal?: AbortSignal) => request<GroupOption[]>("/api/catalog/groups", { signal }),
  accountOptions: (signal?: AbortSignal) =>
    request<AccountOption[]>("/api/catalog/accounts", { signal }),
  upstreamOptions: (signal?: AbortSignal) =>
    request<UpstreamOption[]>("/api/catalog/upstreams", { signal }),
  workbenchImportModel: (signal?: AbortSignal) =>
    request<ImportModelSettings>("/api/account-workbench/import-model", { signal }),
  saveWorkbenchImportModel: (value: ImportModelSettings) =>
    request<ImportModelSettings>("/api/account-workbench/import-model", {
      method: "PUT",
      body: JSON.stringify(value),
    }),
  workbenchAccounts: (signal?: AbortSignal) =>
    request<WorkbenchAccount[]>("/api/account-workbench/accounts", { signal }),
  workbenchAccountActionPreview: (id: string, signal?: AbortSignal) =>
    request<WorkbenchAccountActionPreview>(
      `/api/account-workbench/accounts/${encodeURIComponent(id)}/action-preview`,
      { signal },
    ),
  startWorkbenchAccountAction: (id: string, input: WorkbenchAccountActionInput) =>
    request<Task>(`/api/account-workbench/accounts/${encodeURIComponent(id)}/actions`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  workbenchAccountGroups: (id: string, signal?: AbortSignal) =>
    request<AccountGroupsPreview & { account_version: string }>(
      `/api/account-workbench/accounts/${encodeURIComponent(id)}/groups`,
      { signal },
    ),
  moveWorkbenchAccountGroups: (
    id: string,
    input: AccountGroupsInput & { account_version: string },
  ) =>
    request<Task>(`/api/account-workbench/accounts/${encodeURIComponent(id)}/groups`, {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  workbenchRememberedAccounts: (signal?: AbortSignal) =>
    request<RememberedAccount[]>("/api/account-workbench/remembered-accounts", { signal }),
  previewWorkbenchRememberedLogin: (id: string, revision: number, signal?: AbortSignal) =>
    request<WorkbenchPreview>(
      `/api/account-workbench/remembered-accounts/${encodeURIComponent(id)}/preview`,
      {
        method: "POST",
        body: JSON.stringify({ revision }),
        signal,
      },
    ),
  deleteWorkbenchRememberedAccount: (id: string, revision: number) =>
    request<void>(`/api/account-workbench/remembered-accounts/${encodeURIComponent(id)}`, {
      method: "DELETE",
      body: JSON.stringify({ revision }),
    }),
  workbenchPreview: (input: WorkbenchInput) =>
    request<WorkbenchPreview>("/api/account-workbench/preview", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  startWorkbenchRun: (input: { id: string; revision: number }) =>
    request<WorkbenchRun>("/api/account-workbench/runs", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  workbenchRuns: (signal?: AbortSignal) =>
    request<WorkbenchRun[]>("/api/account-workbench/runs", { signal }),
  workbenchMaintenance: (signal?: AbortSignal) =>
    request<WorkbenchMaintenance>("/api/account-workbench/maintenance", { signal }),
  previewWorkbenchMaintenance: (input: MaintenanceSettings) =>
    request<MaintenancePreview>("/api/account-workbench/maintenance/preview", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  configureWorkbenchMaintenance: (input: { id: string; revision: number }) =>
    request<WorkbenchMaintenance>("/api/account-workbench/maintenance/configure", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  workbenchMaintenanceAction: (action: "check" | "stop", revision: number) =>
    request<WorkbenchMaintenance>(`/api/account-workbench/maintenance/${action}`, {
      method: "POST",
      body: JSON.stringify({ revision }),
    }),
  workbenchRun: (id: string, signal?: AbortSignal) =>
    request<WorkbenchRun>(`/api/account-workbench/runs/${encodeURIComponent(id)}`, { signal }),
  workbenchRunAction: (
    id: string,
    action: "retry" | "enable" | "cancel" | "delete",
    input: { revision: number; ids?: string[] },
  ) =>
    request<unknown>(`/api/account-workbench/runs/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  workbenchExport: (id: string, revision: number) =>
    request<WorkbenchArtifact>(`/api/account-workbench/runs/${encodeURIComponent(id)}/export`, {
      method: "POST",
      body: JSON.stringify({ revision }),
    }),
  workbenchLoginInput: (
    id: string,
    item: string,
    input: { prompt_id: string; value: string; action?: "resend_email" },
  ) =>
    request<{ accepted: boolean }>(
      `/api/account-workbench/runs/${encodeURIComponent(id)}/login-input/${encodeURIComponent(item)}`,
      { method: "POST", body: JSON.stringify(input) },
    ),
  workbenchProxies: (signal?: AbortSignal) =>
    request<WorkbenchProxy[]>("/api/account-workbench/proxies", { signal }),
  workbenchTemplates: (signal?: AbortSignal) =>
    request<TemplateLibrary>("/api/account-workbench/templates", { signal }),
  workbenchTemplateSource: (id: string, signal?: AbortSignal) =>
    request<WorkbenchTemplate>(`/api/account-workbench/template-source/${encodeURIComponent(id)}`, {
      signal,
    }),
  saveWorkbenchTemplate: (input: {
    id?: string;
    name: string;
    source_id?: string;
    source_version?: string;
    config?: TemplateConfig;
    revision: number;
  }) =>
    request<TemplateLibrary>("/api/account-workbench/templates", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  setWorkbenchTemplate: (id: string, revision: number) =>
    request<TemplateLibrary>("/api/account-workbench/templates/preference", {
      method: "PUT",
      body: JSON.stringify({ id, revision }),
    }),
  deleteWorkbenchTemplate: (id: string, revision: number) =>
    request<TemplateLibrary>("/api/account-workbench/templates", {
      method: "DELETE",
      body: JSON.stringify({ id, revision }),
    }),
  kumaTemplatePreset: (input: { request_profile: string; model: string }) =>
    request<KumaTemplateDetail>("/api/uptime-kuma/template-preset", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  kumaTemplates: (signal?: AbortSignal) =>
    request<KumaTemplate[]>("/api/uptime-kuma/templates", { signal }),
  kumaTemplateDistribution: (id: string, revision: number, signal?: AbortSignal) =>
    request<KumaTemplateDistributionPreview>(
      `/api/uptime-kuma/templates/${id}/distribution?revision=${revision}`,
      { signal, cache: "no-store" },
    ),
  distributeKumaTemplate: (
    id: string,
    input: { revision: number; scope: string; monitor_ids: number[] },
  ) =>
    request<Task>(`/api/uptime-kuma/templates/${id}/distribution`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  kumaTemplate: (id: string, signal?: AbortSignal) =>
    request<KumaTemplateDetail>(`/api/uptime-kuma/templates/${id}`, { signal, cache: "no-store" }),
  saveKumaTemplate: (id: string | undefined, input: KumaTemplateInput) =>
    request<KumaTemplate>(`/api/uptime-kuma/templates${id ? `/${id}` : ""}`, {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(input),
    }),
  deleteKumaTemplate: (id: string, revision: number) =>
    request<{ deleted: boolean }>(`/api/uptime-kuma/templates/${id}`, {
      method: "DELETE",
      body: JSON.stringify({ revision }),
    }),
  kumaResources: (kind: KumaResourceKind, signal?: AbortSignal) =>
    request<KumaResourceList>(`/api/uptime-kuma/resources/${kind}`, { signal }),
  kumaResource: (kind: KumaResourceKind, id: number, signal?: AbortSignal) =>
    request<KumaResource>(`/api/uptime-kuma/resources/${kind}/${id}`, { signal }),
  writeKumaResource: (kind: KumaResourceKind, id: number, input: KumaResourceWrite) =>
    request<Task>(`/api/uptime-kuma/resources/${kind}/${id}`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  kumaConfig: (signal?: AbortSignal) => request<KumaConfig>("/api/uptime-kuma/config", { signal }),
  saveKumaConfig: (input: KumaConfigInput) =>
    request<Task>("/api/uptime-kuma/config", { method: "PUT", body: JSON.stringify(input) }),
  disconnectKuma: (revision: number) =>
    request<{ disconnected: boolean }>("/api/uptime-kuma/config", {
      method: "DELETE",
      body: JSON.stringify({ revision }),
    }),
  kumaMonitors: (signal?: AbortSignal) =>
    request<KumaSnapshot>("/api/uptime-kuma/monitors", { signal }),
  kumaPushURL: (id: number, revision: string) =>
    request<{ url: string }>(
      `/api/uptime-kuma/monitors/${id}/push-url?revision=${encodeURIComponent(revision)}`,
    ),
  writeKumaMonitor: (id: number, input: KumaWriteInput) =>
    request<Task>(`/api/uptime-kuma/monitors${id ? `/${id}` : ""}`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  accountResultsEventsURL: (ids: readonly string[]) =>
    apiEndpoint(`/api/accounts/results/events?account_ids=${encodeURIComponent(ids.join(","))}`),
  autoInspectionEventsURL: () => apiEndpoint("/api/inspection/automation/events"),
  taskEventsURL: (id: string) => apiEndpoint(`/api/tasks/${encodeURIComponent(id)}/events`),
  setupStatus: (signal?: AbortSignal) => request<SetupStatus>("/api/setup/status", { signal }),
  initialize: (
    payload: {
      username: string;
      password: string;
      local_export_only?: boolean;
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
  session: (signal?: AbortSignal) => request<SessionStatus>("/api/auth/session", { signal }),
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
  overview: (signal?: AbortSignal) => request<Overview>("/api/overview", { signal }),
  newAPIWorkspace: (platformId?: string, signal?: AbortSignal) => {
    const query = platformId ? `?platform_id=${encodeURIComponent(platformId)}` : "";
    return request<NewAPIWorkspace>(`/api/newapi${query}`, { signal });
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
  refreshNewAPIPlatform: (platformId: string, signal?: AbortSignal) =>
    request<NewAPIRemoteSnapshot>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/refresh`,
      { method: "POST", signal },
    ),
  newAPIRemoteGroups: (platformId: string, signal?: AbortSignal) =>
    request<{ groups: NewAPIRemoteGroup[] }>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/groups`,
      { signal },
    ),
  managementModelPrices: (platformId: string, signal?: AbortSignal) =>
    request<ModelPriceCatalog>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/management-model-prices`,
      { signal },
    ),
  refreshManagementModelPrices: (platformId: string) =>
    request<ModelPriceCatalog>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/management-model-prices/refresh`,
      { method: "POST" },
    ),
  remoteModelPricingSource: (platformId: string, signal?: AbortSignal) =>
    request<RemoteModelPricingSource>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/remote-model-prices/raw`,
      { signal },
    ),
  saveNewAPIGroupBindings: (platformId: string, bindings: NewAPIGroupBindingUpdate[]) =>
    request<NewAPIGroupBinding[]>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/group-bindings`,
      { method: "PUT", body: JSON.stringify({ bindings }) },
    ),
  navigationPreferences: (signal?: AbortSignal) =>
    request<NavigationPreferences>("/api/preferences/navigation", { signal }),
  saveNavigationPreferences: (payload: NavigationPreferences) =>
    request<NavigationPreferences>("/api/preferences/navigation", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  newAPIChannelGroups: (platformId: string, signal?: AbortSignal) =>
    request<NewAPIChannelGroups>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channel-groups`,
      { signal },
    ),
  saveNewAPIChannelGroups: (platformId: string, payload: NewAPIChannelGroups) =>
    request<NewAPIChannelGroups>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channel-groups`,
      {
        method: "PUT",
        body: JSON.stringify(payload),
      },
    ),
  batchNewAPIChannelModels: (
    platformId: string,
    payload: {
      action: "add" | "remove";
      models: string[];
      channels: Array<{ id: string; version: string }>;
    },
  ) =>
    request<Task>(`/api/newapi/platforms/${encodeURIComponent(platformId)}/channels/models/batch`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  newAPIChannelLookup: (platformId: string, ids: string[], signal?: AbortSignal) => {
    const query = new URLSearchParams();
    for (const id of ids) query.append("id", id);
    return request<NewAPIChannelLookup>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channels/lookup?${query}`,
      { signal },
    );
  },
  newAPIChannels: (
    platformId: string,
    page: number,
    pageSize = 50,
    groupId = "",
    signal?: AbortSignal,
  ) =>
    request<{ items: NewAPIChannel[]; total: number }>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channels?${new URLSearchParams({ page: String(page), page_size: String(pageSize), ...(groupId ? { group_id: groupId } : {}) })}`,
      { signal },
    ),
  changeNewAPIChannelModels: (
    platformId: string,
    channelId: string,
    payload: NewAPIChannelModelChange,
  ) =>
    request<NewAPIChannel>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channels/${encodeURIComponent(channelId)}/models`,
      { method: "PUT", body: JSON.stringify(payload) },
    ),
  createNewAPIChannel: (
    platformId: string,
    payload: {
      sub2api_group_id: string;
      key_id: string;
      base_url: string;
      models: string[];
      newapi_groups: string[];
    },
  ) =>
    request<Record<string, unknown>>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channels`,
      { method: "POST", body: JSON.stringify(payload) },
    ),
  createNewAPIChannelKey: (platformId: string, payload: NewAPIChannelKeyRequest) =>
    request<NewAPIChannelKey>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channel-key`,
      { method: "POST", body: JSON.stringify(payload) },
    ),
  fetchNewAPIChannelModels: (
    platformId: string,
    payload: { sub2api_group_id: string; key_id: string; base_url: string },
    signal?: AbortSignal,
  ) =>
    request<{ models: string[] }>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channel-models`,
      { method: "POST", body: JSON.stringify(payload), signal },
    ),
  availableNewAPIChannelModels: (
    platformId: string,
    channel: NewAPIChannel,
    signal?: AbortSignal,
  ) =>
    request<{ models: string[] }>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/channels/${encodeURIComponent(channel.id)}/models/available?${new URLSearchParams({ version: channel.version })}`,
      { signal },
    ),
  saveNewAPIModelPrices: (platformId: string, prices: NewAPIModelPrice[]) =>
    request<NewAPIRemoteSnapshot>(
      `/api/newapi/platforms/${encodeURIComponent(platformId)}/model-prices`,
      { method: "PUT", body: JSON.stringify({ prices }) },
    ),
  trafficRanking: (
    filters: {
      timeRange: "1h" | "6h" | "24h" | "7d" | "30d";
      group?: string;
      sortBy: TrafficRankingSort;
    },
    signal?: AbortSignal,
  ) => {
    const query = new URLSearchParams({
      time_range: filters.timeRange,
      sort_by: filters.sortBy,
    });
    if (filters.group) query.set("group", filters.group);
    return request<TrafficRanking>(`/api/traffic/ranking?${query.toString()}`, { signal });
  },
  config: (signal?: AbortSignal) => request<RuntimeConfig>("/api/config", { signal }),
  notificationStatus: (signal?: AbortSignal) =>
    request<NotificationStatus>("/api/notifications/status", { signal }),
  notificationQueue: (signal?: AbortSignal) =>
    request<NotificationQueueDetails>("/api/notifications/queue", { signal }),
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
  taskConcurrency: (signal?: AbortSignal) =>
    request<TaskConcurrencySettings>("/api/config/task-concurrency", { signal }),
  updateTaskConcurrency: (payload: TaskConcurrencyUpdate) =>
    request<TaskConcurrencySettings>("/api/config/task-concurrency", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  accountCreationSettings: (signal?: AbortSignal) =>
    request<AccountCreationSettings>("/api/config/account-settings", { signal }),
  updateAccountCreationSettings: (payload: AccountCreationSettings) =>
    request<AccountCreationSettings>("/api/config/account-settings", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  syncExistingAccountPoolMode: (accountIds: string[]) =>
    request<Task>("/api/config/account-settings/pool-mode/apply", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
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
  upstreamAllocationSetting: (kind: UpstreamAllocationTarget, id: string, signal?: AbortSignal) =>
    request<UpstreamAllocationSetting>(
      `/api/policy/upstream-concurrency/${kind}/${encodeURIComponent(id)}`,
      { signal },
    ),
  setUpstreamAllocationSetting: (
    kind: UpstreamAllocationTarget,
    id: string,
    payload: {
      override: boolean | null;
      expected_revision: string;
      expected_upstream_id: string;
    },
  ) =>
    request<UpstreamAllocationSetting>(
      `/api/policy/upstream-concurrency/${kind}/${encodeURIComponent(id)}`,
      {
        method: "PUT",
        body: JSON.stringify(payload),
      },
    ),
  policy: (signal?: AbortSignal) => request<PolicySnapshot>("/api/policy", { signal }),
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
  pricing: (signal?: AbortSignal) => request<PricingSnapshot>("/api/pricing", { signal }),
  pricingChanges: (signal?: AbortSignal) =>
    request<PricingChangeRecord[]>("/api/pricing/changes", { signal }),
  updatePricingConfig: (payload: PricingConfig) =>
    request<PricingSnapshot>("/api/pricing/config", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  applyPricing: () => request<Task>("/api/pricing/apply", { method: "POST" }),
  pricingBackups: (signal?: AbortSignal) =>
    request<PricingBackup[]>("/api/pricing/backups", { signal }),
  createPricingBackup: (name: string) =>
    request<PricingBackup>("/api/pricing/backups", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  restorePricingBackup: (backupId: string) =>
    request<Task>(`/api/pricing/backups/${encodeURIComponent(backupId)}/restore`, {
      method: "POST",
    }),
  deletePricingBackup: (backupId: string) =>
    request<{ id: string; deleted: boolean }>(
      `/api/pricing/backups/${encodeURIComponent(backupId)}`,
      { method: "DELETE" },
    ),
  calculateRevenue: (date: string) =>
    request<Task>("/api/pricing/revenue", {
      method: "POST",
      body: JSON.stringify({ date }),
    }),
  latestRevenue: (signal?: AbortSignal) =>
    request<Task | null>("/api/pricing/revenue/latest", { signal }),
  setAccountGroupsLocked: (id: string, enabled: boolean) =>
    request<{ groups_locked: boolean }>(`/api/accounts/${encodeURIComponent(id)}/group-lock`, {
      method: "PUT",
      body: JSON.stringify({ groups_locked: enabled }),
    }),
  accountGroups: (id: string, signal?: AbortSignal) =>
    request<AccountGroupsPreview>(`/api/accounts/${encodeURIComponent(id)}/groups`, { signal }),
  updateAccountGroups: (id: string, input: AccountGroupsInput) =>
    request<Task>(`/api/accounts/${encodeURIComponent(id)}/groups`, {
      method: "PUT",
      body: JSON.stringify(input),
    }),
  groups: (signal?: AbortSignal) => request<GroupStatus[]>("/api/groups", { signal }),
  dictionaries: (kind: DictionaryKind) =>
    request<{ items: DictionaryEntry[] }>(`/api/dictionaries?kind=${encodeURIComponent(kind)}`),
  reorderDictionaries: (kind: DictionaryKind, ids: string[]) =>
    request<void>("/api/dictionaries/reorder", {
      method: "POST",
      body: JSON.stringify({ kind, ids }),
    }),
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
  batchGroupPolicy: (groupIds: string[], action: "reset" | "exclude" | "include") =>
    request<{ updated: number }>("/api/groups/batch-policy", {
      method: "POST",
      body: JSON.stringify({ group_ids: groupIds, action }),
    }),
  upstreams: (signal?: AbortSignal) => request<UpstreamSummary>("/api/upstreams", { signal }),
  upstreamUsage: (signal?: AbortSignal) =>
    request<UpstreamUsage[]>("/api/upstreams/usage", { signal }),
  cleanupUpstreamBinding: (host: string, bindingId: number, payload: UpstreamBindingCleanup) =>
    request<{ binding_id: number }>(
      `/api/upstreams/${encodeURIComponent(host)}/bindings/${bindingId}/cleanup`,
      {
        method: "POST",
        body: JSON.stringify(payload),
      },
    ),
  upstreamConfiguration: (host: string, signal?: AbortSignal) =>
    request<UpstreamConfiguration>(`/api/upstreams/${encodeURIComponent(host)}/configuration`, {
      signal,
    }),
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
  upstreamGroupBindingAudit: (signal?: AbortSignal) =>
    request<UpstreamGroupBindingAudit>("/api/upstreams/group-bindings/audit", { signal }),
  upstreamGroupHistory: (host: string) =>
    request<UpstreamGroupChange[]>(
      `/api/upstreams/${encodeURIComponent(host)}/group-history?limit=200`,
    ),
  allUpstreamGroupHistory: (signal?: AbortSignal) =>
    request<UpstreamGroupChange[]>("/api/upstreams/group-history?limit=500", { signal }),
  clearOneUpstreamGroupHistory: (upstreamID: string) =>
    request<{ deleted: number }>(`/api/upstreams/group-history/${encodeURIComponent(upstreamID)}`, {
      method: "DELETE",
    }),
  clearUpstreamGroupHistory: () =>
    request<{ deleted: number }>("/api/upstreams/group-history", { method: "DELETE" }),
  upstreamDeletePreview: (host: string) =>
    request<UpstreamDeletePreview>(`/api/upstreams/${encodeURIComponent(host)}/delete-preview`),
  upstreamExclusionPreview: (host: string, signal?: AbortSignal) =>
    request<UpstreamExclusionPreview>(
      `/api/upstreams/${encodeURIComponent(host)}/exclusion-preview`,
      { signal },
    ),
  setUpstreamExclusion: (host: string, input: UpstreamExclusionRequest) =>
    request<Task>(`/api/upstreams/${encodeURIComponent(host)}/exclusion`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  deleteUpstream: (host: string, expectedAccountIds: string[]) =>
    request<Task>(`/api/upstreams/${encodeURIComponent(host)}/delete`, {
      method: "POST",
      body: JSON.stringify({
        confirmation_host: host,
        expected_account_ids: expectedAccountIds,
      }),
    }),
  accounts: (signal?: AbortSignal) => request<AccountStatus[]>("/api/accounts", { signal }),
  enqueueAccountModelAutomation: (payload: AccountModelAutomationTaskRequest) =>
    request<Task>("/api/management/accounts/models/automation", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  accountModelAutomationSchedules: (signal?: AbortSignal) =>
    request<AccountModelAutomationSchedule[]>(
      "/api/management/accounts/models/automation/schedules",
      { signal },
    ),
  saveAccountModelAutomationSchedule: (payload: AccountModelAutomationScheduleInput) =>
    request<AccountModelAutomationSchedule>(
      "/api/management/accounts/models/automation/schedules",
      { method: "PUT", body: JSON.stringify(payload) },
    ),
  deleteAccountModelAutomationSchedule: (id: string) =>
    request<{ deleted: boolean }>(
      `/api/management/accounts/models/automation/schedules/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    ),
  runAccountModelAutomationSchedule: (id: string) =>
    request<Task>(
      `/api/management/accounts/models/automation/schedules/${encodeURIComponent(id)}/run`,
      { method: "POST" },
    ),
  accountModelAutomationTaskGroups: (signal?: AbortSignal) =>
    request<AccountModelAutomationTaskGroup[]>(
      "/api/management/accounts/models/automation/groups",
      {
        signal,
      },
    ),
  saveAccountModelAutomationTaskGroup: (payload: AccountModelAutomationTaskGroupInput) =>
    request<AccountModelAutomationTaskGroup>("/api/management/accounts/models/automation/groups", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  deleteAccountModelAutomationTaskGroup: (id: string) =>
    request<{ deleted: boolean }>(
      `/api/management/accounts/models/automation/groups/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    ),
  runAccountModelAutomationTaskGroup: (id: string) =>
    request<Task>(
      `/api/management/accounts/models/automation/groups/${encodeURIComponent(id)}/run`,
      { method: "POST" },
    ),
  groupAllocation: (groupId: string, signal?: AbortSignal) =>
    request<GroupAllocation>(`/api/groups/${encodeURIComponent(groupId)}/allocation`, { signal }),
  groupProbeModels: (groupId: string, signal?: AbortSignal) =>
    request<GroupProbeModels>(`/api/groups/${encodeURIComponent(groupId)}/models`, { signal }),
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
  accountControlState: (accountId: string, signal?: AbortSignal) =>
    request<AccountControlState>(`/api/accounts/${encodeURIComponent(accountId)}/control-state`, {
      signal,
    }),
  account: (accountId: string, signal?: AbortSignal) =>
    request<AccountDetail>(`/api/accounts/${encodeURIComponent(accountId)}`, { signal }),
  accountDeletePreview: (accountId: string, signal?: AbortSignal) =>
    request<AccountDeletePreview>(`/api/accounts/${encodeURIComponent(accountId)}/delete-preview`, {
      signal,
    }),
  accountDeleteBatchPreview: (accountIds: string[], signal?: AbortSignal) =>
    request<AccountDeleteBatchPreview>("/api/accounts/delete-preview", {
      signal,
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
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
  deleteAccounts: (preview: AccountDeleteBatchPreview) =>
    request<Task>("/api/accounts/delete", {
      method: "POST",
      body: JSON.stringify({
        confirmations: preview.accounts.map((account) => ({
          account_id: account.account_id,
          management_base_url: account.management_base_url,
          binding: account.binding,
        })),
      }),
    }),
  setAccountControl: (accountId: string, action: AccountControlAction) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/control`, {
      method: "POST",
      body: JSON.stringify({ action }),
    }),
  accountModels: (accountId: string, signal?: AbortSignal) =>
    request<{ models: string[] }>(`/api/accounts/${encodeURIComponent(accountId)}/models`, {
      signal,
    }),
  discoverAccountModels: (accountIds: string[]) =>
    request<Task>("/api/management/accounts/models/discover", {
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  previewAccountModels: (accountIds: string[], signal?: AbortSignal) =>
    request<AccountModelSyncPreview>("/api/management/accounts/models/preview", {
      signal,
      method: "POST",
      body: JSON.stringify({ account_ids: accountIds }),
    }),
  applyAccountModels: (
    accounts: AccountModelSelection[],
    catalogFingerprint: string,
    probeModels: string[],
  ) =>
    request<Task>("/api/management/accounts/models/apply", {
      method: "POST",
      body: JSON.stringify({
        accounts,
        probe_models: probeModels,
        catalog_fingerprint: catalogFingerprint,
      }),
    }),
  accountModelSyncSettings: (signal?: AbortSignal) =>
    request<AccountModelSyncSettings>("/api/config/model-sync", { signal }),
  addAccountModelBlock: (model: string) =>
    request<AccountModelSyncSettings>("/api/config/model-sync/blocked-models", {
      method: "POST",
      body: JSON.stringify({ model }),
    }),
  updateAccountModelSyncSettings: (
    payload: AccountModelSyncSettings & { expected_patterns: string[] },
  ) =>
    request<AccountModelSyncSettings>("/api/config/model-sync", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  setAccountTestModels: (accountId: string, models: string[]) =>
    request<{ saved: boolean }>(`/api/accounts/${encodeURIComponent(accountId)}/test-models`, {
      method: "PUT",
      body: JSON.stringify({ models }),
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
  setAccountIgnoreCostWall: (accountId: string, enabled: boolean) =>
    request<{ ignore_cost_wall: boolean }>(
      `/api/accounts/${encodeURIComponent(accountId)}/cost-wall`,
      {
        method: "PUT",
        body: JSON.stringify({ ignore_cost_wall: enabled }),
      },
    ),
  saveAccountSettings: (
    accountId: string,
    payload: {
      priority: number;
      load_factor: string;
      follow_concurrency?: boolean;
      concurrency: number;
      test_models: string[];
      paused: boolean;
      excluded: boolean;
    },
  ) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/settings`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  setAccountManualPriority: (
    accountId: string,
    priority: number,
    loadFactor: string,
    concurrency: number,
    schedulable: boolean,
    syncBalanceMultiplier: boolean,
  ) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/manual-priority`, {
      method: "PUT",
      body: JSON.stringify({
        priority,
        load_factor: loadFactor,
        concurrency,
        schedulable,
        sync_balance_multiplier: syncBalanceMultiplier,
      }),
    }),
  clearAccountManualPriority: (accountId: string) =>
    request<Task>(`/api/accounts/${encodeURIComponent(accountId)}/manual-priority`, {
      method: "DELETE",
    }),
  recentEvents: (signal?: AbortSignal) => request<RunEvent[]>("/api/events?limit=8", { signal }),
  logs: (
    params: {
      kind: UnifiedLogKind;
      state: UnifiedLogState;
      level: UnifiedLogEventLevel;
      group: string;
      groupId: string;
      search: string;
      page: number;
      pageSize: number;
    },
    signal?: AbortSignal,
  ) => {
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
    return request<UnifiedLogPage>(`/api/logs?${query.toString()}`, { signal });
  },
  logCleanupStatus: (signal?: AbortSignal) =>
    request<LogCleanupStatus>("/api/config/log-cleanup", { signal }),
  updateLogCleanup: (payload: { enabled: boolean; retention_days: number }) =>
    request<LogCleanupStatus>("/api/config/log-cleanup", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  clearLogs: (retentionDays: number) =>
    request<Task>(`/api/logs?retention_days=${retentionDays}`, {
      method: "DELETE",
    }),
  runInspection: (payload?: { account_id?: string; group_name?: string }) =>
    request<Task>("/api/inspection/run", {
      method: "POST",
      body: JSON.stringify(payload ?? {}),
    }),
  autoInspection: (signal?: AbortSignal) =>
    request<AutoInspectionStatus>("/api/inspection/automation", { signal }),
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
  runActiveProbe: (payload?: {
    account_id?: string;
    account_ids?: string[];
    group_name?: string;
    platform?: string;
    model?: string;
  }) =>
    request<Task>("/api/inspection/probe", {
      method: "POST",
      body: JSON.stringify(payload ?? {}),
    }),
  animationHistory: (signal?: AbortSignal) =>
    request<Task[]>("/api/model-checks/animations", { signal }),
  animationQualitySettings: (signal?: AbortSignal) =>
    request<AnimationQualitySettings>("/api/model-checks/animation-quality/settings", { signal }),
  saveAnimationQualitySettings: (payload: AnimationQualitySettings) =>
    request<AnimationQualitySettings>("/api/model-checks/animation-quality/settings", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  animationQuality: (taskId: string, accountId: string, requestId: string, signal?: AbortSignal) =>
    request<AnimationQualityReview>(
      `/api/model-checks/animations/${encodeURIComponent(taskId)}/quality?${new URLSearchParams({ account_id: accountId, request_id: requestId })}`,
      { signal },
    ),
  saveAnimationQuality: (
    taskId: string,
    payload: {
      account_id: string;
      request_id: string;
      label: AnimationQualityPrediction["status"];
      reason: string;
      source_group: string;
      version: number;
    },
  ) =>
    request<AnimationQualityReview>(
      `/api/model-checks/animations/${encodeURIComponent(taskId)}/quality`,
      { method: "PUT", body: JSON.stringify(payload) },
    ),
  animationQualitySamples: (after: string, signal?: AbortSignal) =>
    request<{ items: AnimationQualitySample[]; next_cursor?: string }>(
      `/api/model-checks/animation-quality/samples?${new URLSearchParams({ after, limit: "50" })}`,
      { signal },
    ),
  setAnimationManualVerdict: (
    taskId: string,
    payload: { account_id: string; request_id: string; verdict: AnimationManualVerdict["verdict"] },
  ) =>
    request<Task>(`/api/model-checks/animations/${encodeURIComponent(taskId)}/manual-verdict`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  accountAnimationModels: (accountId: string, signal?: AbortSignal) =>
    request<{ models: string[] }>(
      `/api/model-checks/animations/accounts/${encodeURIComponent(accountId)}/models`,
      { signal },
    ),
  customAnimationModels: (payload: Omit<AnimationCustomEndpoint, "model">, signal?: AbortSignal) =>
    request<{ models: string[] }>("/api/model-checks/animations/models", {
      method: "POST",
      body: JSON.stringify(payload),
      signal,
    }),
  runAnimation: (payload: AnimationRequest) =>
    request<Task>("/api/model-checks/animations", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  detectionTasks: (signal?: AbortSignal) =>
    request<DetectionTask[]>("/api/model-checks/detection-tasks", { signal }),
  saveDetectionTask: (value: DetectionTask) =>
    request<DetectionTask[]>("/api/model-checks/detection-tasks", {
      method: "PUT",
      body: JSON.stringify(value),
    }),
  deleteDetectionTask: (id: string, version: number) =>
    request<DetectionTask[]>(`/api/model-checks/detection-tasks/${encodeURIComponent(id)}`, {
      method: "DELETE",
      body: JSON.stringify({ version }),
    }),
  runDetectionTask: (id: string, version: number) =>
    request<Task>(`/api/model-checks/detection-tasks/${encodeURIComponent(id)}/run`, {
      method: "POST",
      body: JSON.stringify({ version }),
    }),
  animationPrompt: (signal?: AbortSignal) =>
    request<AnimationPromptSettings>("/api/model-checks/animation-prompt", { signal }),
  saveAnimationPrompt: (payload: Omit<AnimationPromptSettings, "prompt">) =>
    request<AnimationPromptSettings>("/api/model-checks/animation-prompt", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  animationSchedules: (signal?: AbortSignal) =>
    request<AnimationSchedule[]>("/api/model-checks/animation-schedules", { signal }),
  saveAnimationSchedules: (schedules: AnimationSchedule[]) =>
    request<AnimationSchedule[]>("/api/model-checks/animation-schedules", {
      method: "PUT",
      body: JSON.stringify({ schedules }),
    }),
  saveAnimationSchedule: (payload: AnimationSchedule) =>
    request<AnimationSchedule[]>(
      `/api/model-checks/animation-schedules/${encodeURIComponent(payload.account_id)}`,
      {
        method: "PUT",
        body: JSON.stringify(payload),
      },
    ),
  modelCheckCapabilities: (signal?: AbortSignal) =>
    request<ModelCheckCapabilities>("/api/model-checks/capabilities", { signal }),
  modelCheckAccountStatuses: (signal?: AbortSignal) =>
    request<ModelCheckAccountStatus[]>("/api/model-checks/account-statuses", { signal }),
  runModelCheck: (payload: ModelCheckRequest) =>
    request<Task>("/api/model-checks", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  authRecoveryConfig: (signal?: AbortSignal) =>
    request<PrivateAuthConfigStatus>("/api/auth-recovery/config", { signal }),
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
    accept_login_agreement?: boolean;
    entry?: string;
    headers?: Record<string, string>;
    cookies?: Record<string, string>;
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
  runAuthRecovery: (host: string, entry: string, acceptLoginAgreement?: boolean) =>
    request<Task>("/api/auth-recovery/run", {
      method: "POST",
      body: JSON.stringify({
        host,
        entry,
        ...(acceptLoginAgreement ? { accept_login_agreement: true } : {}),
      }),
    }),
  runAuthRecoveryBatch: (hosts: string[]) =>
    request<Task>("/api/auth-recovery/run-batch", {
      method: "POST",
      body: JSON.stringify({ hosts }),
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
  systemLogs: (params: SystemLogSearchQuery, signal?: AbortSignal) => {
    const query = new URLSearchParams({
      request_id: params.requestId,
      page: String(params.page),
      page_size: String(params.pageSize),
    });
    return request<SystemLogPage>(`/api/ops/system-logs?${query.toString()}`, { signal });
  },
  // Keep the alert view bounded; an unbounded incident table can block the
  // browser when a long-running instance has accumulated a large history.
  alerts: (signal?: AbortSignal) => request<AlertIncident[]>("/api/alerts?limit=200", { signal }),
  clearAlerts: () => request<{ deleted: number }>("/api/alerts", { method: "DELETE" }),
  alertPolicy: (signal?: AbortSignal) => request<AlertPolicy>("/api/alerts/policy", { signal }),
  updateAlertPolicy: (payload: AlertPolicy) =>
    request<AlertPolicy>("/api/alerts/policy", {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  evaluateAlerts: () => request<Task>("/api/alerts/evaluate", { method: "POST" }),
  systemMetrics: (signal?: AbortSignal) =>
    request<SystemMetrics>("/api/system/metrics", { signal }),
  tasks: (limit = 20, signal?: AbortSignal) =>
    request<TaskSummary[]>(`/api/tasks?limit=${limit}`, { signal }),
  task: (id: string, signal?: AbortSignal) => request<Task>(`/api/tasks/${id}`, { signal }),
  cancelTask: (id: string) =>
    request<{ cancelled: boolean }>(`/api/tasks/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  onboard: (payload: OnboardingRequest) =>
    request<Task>("/api/onboarding", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  previewOnboardingConcurrency: (items: OnboardingRequest[]) =>
    request<{ items: { concurrency: number | null; waiting_for_capacity?: boolean }[] }>(
      "/api/onboarding/concurrency-preview",
      {
        method: "POST",
        body: JSON.stringify({ items }),
      },
    ),
  onboardBatch: (items: OnboardingRequest[]) =>
    request<Task>("/api/onboarding/batch", {
      method: "POST",
      body: JSON.stringify({ items }),
    }),
  prepareOnboarding: (host: string, signal?: AbortSignal) =>
    request<OnboardingContext>("/api/onboarding/prepare", {
      method: "POST",
      body: JSON.stringify({ host }),
      signal,
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
  startOnboardingProbeTask: (
    action: "models" | "model-options" | "probe" | "cleanup",
    host: string,
    groupId: string,
    model?: string,
    mode?: OnboardingProbeMode,
    animationPromptId?: string,
  ) =>
    request<Task>(`/api/onboarding/probe/tasks/${action}`, {
      method: "POST",
      body: JSON.stringify({
        host,
        group_id: groupId,
        model,
        mode,
        animation_prompt_id: animationPromptId,
      }),
    }),
  runOnboardingProbe: (
    host: string,
    groupId: string,
    model: string,
    mode: OnboardingProbeMode = "default",
  ) =>
    request<ProbeResult>("/api/onboarding/probe", {
      method: "POST",
      body: JSON.stringify({ host, group_id: groupId, model, mode }),
    }),
  cancelOnboardingProbe: (host: string, groupId: string) =>
    request<{ cancelled: boolean }>("/api/onboarding/probe/cancel", {
      method: "POST",
      body: JSON.stringify({ host, group_id: groupId }),
    }),
};
