import type { ReactNode } from "react";
import type { OfficialPriceTier } from "@/api";
import { pricePerMillion } from "../lib/pricing-number";

type PriceField = "input_price" | "output_price" | "cache_read_price" | "cache_write_price";

export function OfficialTierValues(props: {
  tiers?: OfficialPriceTier[];
  field: PriceField;
  children: ReactNode;
}): ReactNode {
  if (!props.tiers?.length) return props.children;
  if (
    props.tiers.every(
      (tier) =>
        !tier[props.field] && !(props.field === "cache_write_price" && tier.cache_write_1h_price),
    )
  )
    return "-";
  return (
    <div className="flex flex-col gap-1 whitespace-nowrap">
      {props.tiers.map((tier) => (
        <span key={tier.condition}>
          {tier.label} {tier[props.field] ? pricePerMillion(tier[props.field]) : "-"}
          {props.field === "cache_write_price" && tier.cache_write_1h_price
            ? ` / 1 小时 ${pricePerMillion(tier.cache_write_1h_price)}`
            : null}
        </span>
      ))}
    </div>
  );
}
