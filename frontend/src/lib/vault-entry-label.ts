import type { VaultEntryIndex } from "@/api";

export type VaultEntryLabelInput = {
  entry: string;
};

/** Show the operator-defined credential name. */
export function vaultEntryLabel(item: VaultEntryLabelInput): string {
  return item.entry;
}

function normalizeHost(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/$/, "");
}

function vaultEntryHostRank(item: VaultEntryIndex, normalizedHost: string): number {
  const hosts = item.hosts.map(normalizeHost);
  if (normalizedHost && hosts.includes(normalizedHost)) return 0;
  const alias = normalizedHost.replace(/^www\./, "");
  if (alias && hosts.some((host) => host.replace(/^www\./, "") === alias)) return 1;
  return item.hosts.length === 0 ? 2 : 3;
}

export function defaultVaultEntryForHost(
  entries: VaultEntryIndex[],
  host: string | null | undefined,
): string {
  if (!host) return "";
  const normalizedHost = normalizeHost(host);
  const exact = entries.find((item) => vaultEntryHostRank(item, normalizedHost) === 0);
  if (exact) return exact.entry;
  return entries.find((item) => vaultEntryHostRank(item, normalizedHost) === 1)?.entry ?? "";
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
  return usable
    .map((item, index) => ({ item, index, rank: vaultEntryHostRank(item, normalizedHost) }))
    .sort((left, right) => left.rank - right.rank || left.index - right.index)
    .map(({ item }) => item);
}
