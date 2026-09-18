export type WorkbenchAccount = {
  id: string;
  name: string;
  email: string;
  status: string;
  schedulable: boolean;
  plan: string;
  groups: { id: string; name: string }[];
  proxy_name: string;
  concurrency: string;
  load_factor: string;
  rate_multiplier: string;
  model_mapping: Record<string, string>;
  fingerprint: string;
};

export type AccountFilter = { query: string; status: string; group: string };

export type TemplateConfig = {
  concurrency: number;
  priority: number;
  rate_multiplier: string;
  load_factor: string | null;
  proxy_id: string | null;
  group_ids: string[];
  auto_pause_on_expired: boolean;
  upstream_billing_probe_enabled?: boolean;
  confirm_mixed_channel_risk?: boolean;
  expires_at: number | null;
  notes: string;
  credential_extras: Record<string, unknown>;
  extra: Record<string, unknown>;
};
export type WorkbenchTemplate = {
  id: string;
  name: string;
  source_id: string;
  source_name: string;
  source_version: string;
  synced_at: string;
  revision: number;
  config: TemplateConfig;
  summary: WorkbenchAccount;
};
export type TemplateLibrary = {
  revision: number;
  preferred_id: string;
  items: WorkbenchTemplate[];
};

type WorkbenchInputItem = {
  id: string;
  index: number;
  kind: string;
  name: string;
  email: string;
  plan: string;
  identity_source: string;
};
export type WorkbenchPreview = {
  id: string;
  revision: number;
  expires_at: string;
  action: "import" | "export";
  check: boolean;
  promote: boolean;
  template: WorkbenchTemplate | null;
  items: WorkbenchInputItem[];
  errors: Array<{ index: number; message: string }>;
  duplicate_count: number;
};
export type WorkbenchRunItem = WorkbenchInputItem & {
  status: string;
  message: string;
  account_id?: string;
  import_action?: string;
  template_name: string;
  check?: Record<string, unknown>;
  manual_enabled?: boolean;
  browser_ready?: boolean;
};
export type WorkbenchRun = {
  id: string;
  revision: number;
  task_id: string;
  status: string;
  action: "import" | "export";
  created_at: string;
  updated_at: string;
  expires_at: string;
  duplicate_count: number;
  items: WorkbenchRunItem[];
};
export type WorkbenchArtifact = { id: string; count: number; expires_at: string };
export type WorkbenchInput = {
  content: string;
  action: "import" | "export";
  template_id: string;
  check: boolean;
  promote: boolean;
  model: string;
  proxy_enabled: boolean;
  proxy_url: string;
};

export type MaintenanceSettings = {
  enabled: boolean;
  interval_minutes: number;
  cooldown_minutes: number;
  check_after_repair: boolean;
  group_ids: string[];
};
type MaintenanceResult = {
  account_id: string;
  email: string;
  action: string;
  status: string;
  reason: string;
  next_eligible_at?: string;
  check?: Record<string, unknown>;
};
export type WorkbenchMaintenance = MaintenanceSettings & {
  revision: number;
  running: boolean;
  task_id: string;
  last_check_at: string | null;
  next_check_at: string | null;
  message: string;
  results: MaintenanceResult[];
};
export type MaintenancePreview = {
  id: string;
  revision: number;
  expires_at: string;
  settings: MaintenanceSettings;
  accounts: WorkbenchAccount[];
};
