import { rawPricingModeLabels } from "../constants";
import { pricePerMillion } from "./pricing-number";

export type RawPricingEntry = { model: string; fields: Record<string, unknown> };
export type RawPricingDocument = { valid: boolean; entries: RawPricingEntry[] };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function parseRawPricingSource(content: string): RawPricingDocument {
  try {
    const value: unknown = JSON.parse(content);
    if (!isRecord(value)) return { valid: false, entries: [] };
    const entries = Object.entries(value)
      .filter(
        (entry): entry is [string, Record<string, unknown>] =>
          entry[0] !== "sample_spec" && isRecord(entry[1]),
      )
      .map(([model, fields]) => ({ model, fields }))
      .sort((left, right) => left.model.localeCompare(right.model));
    return { valid: true, entries };
  } catch {
    return { valid: false, entries: [] };
  }
}

export function rawPricingText(value: unknown): string {
  if (value === null || value === undefined || value === "") return "未提供";
  if (typeof value === "boolean") return value ? "支持" : "不支持";
  if (typeof value === "number")
    return value.toLocaleString("zh-CN", { maximumFractionDigits: 20 });
  if (typeof value === "string") return value;
  if (Array.isArray(value) && value.every((item) => typeof item === "string"))
    return value.join("、") || "未提供";
  return JSON.stringify(value, null, 2);
}

export function rawPricingMode(value: unknown): string {
  if (typeof value !== "string") return "未提供";
  return Object.hasOwn(rawPricingModeLabels, value) ? rawPricingModeLabels[value] : value;
}

export function rawPricingPerMillion(value: unknown): string {
  if (typeof value !== "number" && typeof value !== "string") return "未提供";
  return pricePerMillion(String(value)) || "未提供";
}
