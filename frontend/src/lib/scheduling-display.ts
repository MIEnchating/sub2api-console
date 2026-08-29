export function schedulingMetric(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return "—";
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}
