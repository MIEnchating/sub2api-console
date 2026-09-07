export function formatHealthScore(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return "—";
  return String(Math.round(Math.min(100, Math.max(0, value))));
}
