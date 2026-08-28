import type { VaultEntryIndex } from "@/api";

export type VaultEntryLabelInput = {
  entry: string;
};

/** Show the operator-defined credential name. */
export function vaultEntryLabel(item: VaultEntryLabelInput): string {
  return item.entry;
}

function normalizeHost(value: string): string {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/$/, "");
  return normalized.startsWith("www.") ? normalized.slice(4) : normalized;
}

function vaultEntryMatchesHost(item: VaultEntryIndex, host: string | null | undefined): boolean {
  if (!host) return false;
  const normalizedHost = normalizeHost(host);
  return item.hosts.some((itemHost) => normalizeHost(itemHost) === normalizedHost);
}

export function defaultVaultEntryForHost(
  entries: VaultEntryIndex[],
  host: string | null | undefined,
): string {
  return entries.find((item) => vaultEntryMatchesHost(item, host))?.entry ?? "";
}

/** List every usable entry, placing entries associated with the current Host first. */
export function vaultEntriesForHost(
  entries: VaultEntryIndex[],
  host: string | null | undefined,
  options?: { requireEmail?: boolean },
): VaultEntryIndex[] {
  const unique = new Map<string, VaultEntryIndex>();
  for (const item of entries) {
    const key = item.entry;
    if (
      item.has_username &&
      item.has_password &&
      (!options?.requireEmail || item.username_is_email) &&
      !unique.has(key)
    ) {
      unique.set(key, item);
    }
  }
  const usable = [...unique.values()];
  if (!host) return usable;
  const normalizedHost = normalizeHost(host);
  const rank = (item: VaultEntryIndex): number => {
    const matched = vaultEntryMatchesHost(item, normalizedHost);
    if (matched) return 0;
    if (item.hosts.length === 0) return 1;
    return 2;
  };
  return usable
    .map((item, index) => ({ item, index, rank: rank(item) }))
    .sort((left, right) => left.rank - right.rank || left.index - right.index)
    .map(({ item }) => item);
}
