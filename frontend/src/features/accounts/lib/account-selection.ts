export function updateAccountSelection(
  current: ReadonlySet<string>,
  accountIds: readonly string[],
  selected: boolean,
): Set<string> {
  const next = new Set(current);
  for (const accountId of accountIds) {
    if (selected) next.add(accountId);
    else next.delete(accountId);
  }
  return next;
}

export function pruneAccountSelection(
  current: ReadonlySet<string>,
  availableAccountIds: readonly string[],
): Set<string> {
  const available = new Set(availableAccountIds);
  return new Set([...current].filter((accountId) => available.has(accountId)));
}

export function sameAccountSelection(
  left: ReadonlySet<string>,
  right: ReadonlySet<string>,
): boolean {
  if (left.size !== right.size) return false;
  return [...left].every((accountId) => right.has(accountId));
}
