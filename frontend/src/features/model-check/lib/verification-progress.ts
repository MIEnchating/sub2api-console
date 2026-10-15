export type VerificationRecord = Record<string, unknown>;

export function verificationRecord(value: unknown): VerificationRecord {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as VerificationRecord)
    : {};
}

export function verificationRows(value: unknown): VerificationRecord[] {
  return Array.isArray(value)
    ? value.filter(
        (item): item is VerificationRecord =>
          item !== null && typeof item === "object" && !Array.isArray(item),
      )
    : [];
}

export function verificationText(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function verificationNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function verificationProbeKey(probe: VerificationRecord): string {
  return `${verificationNumber(probe.round) ?? 1}-${verificationText(probe.id)}`;
}

export function verificationProgressProbes(
  result: VerificationRecord,
  active: boolean,
): VerificationRecord[] {
  const completed = verificationRows(result.probes);
  const current = verificationRecord(result.current_probe);
  const byKey = new Map(completed.map((probe) => [verificationProbeKey(probe), probe]));
  if (active && current.id) byKey.set(verificationProbeKey(current), current);
  const plan = verificationRows(result.probe_plan);
  if (plan.length === 0) return [...byKey.values()];
  return plan.map(
    (probe) =>
      byKey.get(verificationProbeKey(probe)) ?? {
        ...probe,
        status: active ? "pending" : "skipped",
      },
  );
}
