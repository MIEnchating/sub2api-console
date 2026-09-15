import { z } from "zod";
import { comparePricingDecimals, parsePricingDecimal } from "./pricing-decimal";

export const groupMinimumSchema = z.object({
  minimum: z
    .string()
    .trim()
    .refine((value) => value === "" || parsePricingDecimal(value) !== null, {
      message: "请输入有效的非负倍率，留空则不限制迁入",
    }),
});

export type GroupMinimumForm = z.infer<typeof groupMinimumSchema>;

export function groupMeetsMinimumCost(cost: string | null, minimum?: string): boolean {
  if (!minimum?.trim()) return true;
  const costValue = parsePricingDecimal(cost);
  const minimumValue = parsePricingDecimal(minimum);
  if (!costValue || !minimumValue) return false;
  return comparePricingDecimals(costValue, minimumValue) >= 0;
}

export function cleanGroupMinimums(
  minimums: Record<string, string> | undefined,
  groupSets: string[][],
): Record<string, string> | undefined {
  const configured = new Set(groupSets.flat());
  const entries = Object.entries(minimums ?? {}).flatMap(([groupID, minimum]) => {
    const value = minimum.trim();
    return configured.has(groupID) && value ? [[groupID, value] as const] : [];
  });
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

export function groupMinimumsEqual(
  left: Record<string, string> | undefined,
  right: Record<string, string> | undefined,
): boolean {
  const entries = Object.entries(left ?? {});
  return (
    entries.length === Object.keys(right ?? {}).length &&
    entries.every(([groupID, minimum]) => minimum === right?.[groupID])
  );
}
