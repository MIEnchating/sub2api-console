// Compare valid timestamps without losing Go's RFC3339Nano precision. Date
// handles UTC offsets; fractional digits break ties within the same millisecond.
export function compareTimestamps(first: string, second: string): number {
  const milliseconds = Date.parse(first) - Date.parse(second);
  if (milliseconds !== 0) return milliseconds;
  const firstFraction = fractionalSeconds(first).padEnd(9, "0");
  const secondFraction = fractionalSeconds(second).padEnd(9, "0");
  if (firstFraction < secondFraction) return -1;
  if (firstFraction > secondFraction) return 1;
  return 0;
}

export function fractionalSeconds(value: string): string {
  return /\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/.exec(value)?.[1] ?? "";
}
