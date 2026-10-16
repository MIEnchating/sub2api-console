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
  const current = Array.isArray(result.current_probes)
    ? verificationRows(result.current_probes)
    : [verificationRecord(result.current_probe)];
  const byKey = new Map(completed.map((probe) => [verificationProbeKey(probe), probe]));
  if (active) {
    for (const probe of current) {
      if (probe.id) byKey.set(verificationProbeKey(probe), probe);
    }
  }
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

export function verificationProbeGroups(probes: VerificationRecord[]): {
  label: string;
  probes: VerificationRecord[];
}[] {
  return [
    {
      label: "能力检测",
      probes: probes.filter(
        (probe) =>
          !["fingerprint", "behavior_fingerprint", "public_reference"].includes(
            verificationText(probe.category),
          ),
      ),
    },
    {
      label: "公开参考对比",
      probes: probes.filter((probe) => probe.category === "public_reference"),
    },
    { label: "分词指纹", probes: probes.filter((probe) => probe.category === "fingerprint") },
    {
      label: "行为指纹",
      probes: probes.filter((probe) => probe.category === "behavior_fingerprint"),
    },
  ].filter((group) => group.probes.length > 0);
}
