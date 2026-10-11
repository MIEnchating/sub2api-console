import type { QueryFilters } from "@tanstack/react-query";
import type { ConfigTab } from "../constants";

const queryRoots: Record<ConfigTab, readonly string[]> = {
  connection: ["config", "auto-inspection"],
  accounts: [
    "config",
    "account-creation-settings",
    "account-model-sync-settings",
    "groups",
    "accounts",
    "dictionaries",
  ],
  tasks: ["task-concurrency"],
  notifications: ["config", "notification-status"],
  interface: ["config", "navigation-preferences", "log-cleanup"],
  dictionaries: ["dictionaries"],
  newapi: [],
  monitoring: [],
};

export function settingsQueryScope(tab: ConfigTab): QueryFilters {
  return {
    type: "active",
    predicate: (query) => {
      const root = query.queryKey[0];
      return (
        typeof root === "string" &&
        queryRoots[tab].includes(root) &&
        (root === "dictionaries" || query.queryKey.length === 1)
      );
    },
  };
}
