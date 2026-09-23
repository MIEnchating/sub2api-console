export type ModelSyncSupportingAccount = {
  accountId: string;
  accountName: string;
  platform: string | null;
};

export type ModelSyncModel = {
  model: string;
  accountCount: number;
  supportingAccounts: ModelSyncSupportingAccount[];
};

type ModelSyncGroup = {
  key: string;
  scopeKey: string;
  label: string;
  accountCount: number;
  accountIds: string[];
  models: ModelSyncModel[];
};

export type ModelSyncPlatformGroup = {
  key: string;
  label: string;
  accountCount: number;
  groups: ModelSyncGroup[];
};
