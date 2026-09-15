export type PricingDecimal = { coefficient: bigint; scale: number };

export function parsePricingDecimal(value: string | null): PricingDecimal | null {
  const source = value?.trim();
  if (!source || source.length > 128) return null;
  const match = /^([+-]?)(?:(\d+)(?:\.(\d*))?|\.(\d+))(?:[eE]([+-]?\d+))?$/.exec(source);
  if (!match) return null;
  const exponent = Number(match[5] ?? "0");
  if (!Number.isInteger(exponent) || Math.abs(exponent) > 1000) return null;
  const fraction = match[3] ?? match[4] ?? "";
  const coefficient = BigInt(`${match[1]}${match[2] ?? "0"}${fraction}`);
  if (coefficient < 0n) return null;
  return { coefficient, scale: fraction.length - exponent };
}

export function comparePricingDecimals(left: PricingDecimal, right: PricingDecimal): number {
  const scale = Math.max(left.scale, right.scale);
  const difference =
    left.coefficient * 10n ** BigInt(scale - left.scale) -
    right.coefficient * 10n ** BigInt(scale - right.scale);
  if (difference < 0n) return -1;
  return difference > 0n ? 1 : 0;
}

export function pricingCostMeetsMargin(
  cost: PricingDecimal,
  rate: PricingDecimal,
  profitMargin: number,
): boolean {
  const margin = parsePricingDecimal(String(profitMargin));
  if (!margin) return false;
  const scale = Math.max(0, margin.scale);
  const multiplier =
    10n ** BigInt(scale) + margin.coefficient * 10n ** BigInt(scale - margin.scale);
  return (
    comparePricingDecimals(
      { coefficient: cost.coefficient * multiplier, scale: cost.scale + scale },
      rate,
    ) <= 0
  );
}
