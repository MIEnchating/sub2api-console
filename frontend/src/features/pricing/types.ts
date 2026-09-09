import type { PricingConfig } from "@/api";

export type PricingConfigDraft = Omit<
  PricingConfig,
  "profit_margin" | "interval_seconds" | "write_concurrency"
> & {
  profit_margin: number | null;
  interval_seconds: number | null;
  write_concurrency: number | null;
};
