export const navigationPreferencesStorageKey = "sub2api-console-hidden-navigation-items";

type NavigationStorage = Pick<Storage, "getItem" | "setItem">;

export function readHiddenNavigationItemIDs<T extends string>(
  storage: Pick<NavigationStorage, "getItem">,
  allowedItemIDs: readonly T[],
  lockedItemIDs: readonly T[],
): Set<T> {
  try {
    const raw = storage.getItem(navigationPreferencesStorageKey);
    if (!raw) return new Set();
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    const allowed = new Set<string>(allowedItemIDs);
    const locked = new Set<string>(lockedItemIDs);
    return new Set(
      parsed.flatMap((value) =>
        typeof value === "string" && allowed.has(value) && !locked.has(value) ? [value as T] : [],
      ),
    );
  } catch {
    return new Set();
  }
}

export function writeHiddenNavigationItemIDs<T extends string>(
  storage: Pick<NavigationStorage, "setItem">,
  hiddenItemIDs: ReadonlySet<T>,
  allowedItemIDs: readonly T[],
): void {
  const orderedHiddenItemIDs = allowedItemIDs.filter((itemID) => hiddenItemIDs.has(itemID));
  try {
    storage.setItem(navigationPreferencesStorageKey, JSON.stringify(orderedHiddenItemIDs));
  } catch {
    // Browser storage can be unavailable in private or restricted contexts.
  }
}

export function visibleNavigationSections<T extends string>(
  sections: ReadonlyArray<{ label: string; itemIDs: readonly T[] }>,
  hiddenItemIDs: ReadonlySet<T>,
): Array<{ label: string; itemIDs: T[] }> {
  return sections.flatMap((section) => {
    const itemIDs = section.itemIDs.filter((itemID) => !hiddenItemIDs.has(itemID));
    return itemIDs.length > 0 ? [{ label: section.label, itemIDs }] : [];
  });
}
