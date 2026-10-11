export function formatUsageAmount(value: string): string {
  const parts = value.split(".");
  const integer = parts[0].replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const fraction = (parts[1] ?? "").slice(0, 6).replace(/0+$/, "");
  if (integer === "0" && !fraction && /[1-9]/.test(parts[1] ?? "")) return "<0.000001";
  return fraction ? `${integer}.${fraction}` : integer;
}
