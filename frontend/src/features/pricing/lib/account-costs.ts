import type { PricingDecision } from "@/api";
import {
  comparePricingDecimals,
  parsePricingDecimal,
  type PricingDecimal,
} from "./pricing-decimal";

export type GroupAccountCosts = { accounts: number; values: string[] };
export const emptyGroupAccountCosts: GroupAccountCosts = { accounts: 0, values: [] };

export function accountCostValue(decision: PricingDecision): PricingDecimal | null {
  const value = parsePricingDecimal(decision.cost_multiplier);
  return value && value.coefficient > 0n ? value : null;
}

export function groupAccountCostsByID(
  decisions: PricingDecision[],
): Map<string, GroupAccountCosts> {
  const groups = new Map<
    string,
    {
      accounts: number;
      values: Map<string, { value: PricingDecimal; label: string }>;
    }
  >();
  for (const decision of decisions) {
    const value = accountCostValue(decision);
    let key = "";
    if (value) {
      const digits = String(value.coefficient);
      const coefficient = digits.replace(/0+$/, "");
      key = `${coefficient}:${value.scale - (digits.length - coefficient.length)}`;
    }
    for (const groupID of new Set(decision.current_group_ids)) {
      let group = groups.get(groupID);
      if (!group) {
        group = { accounts: 0, values: new Map() };
        groups.set(groupID, group);
      }
      group.accounts += 1;
      if (value && decision.cost_multiplier) {
        group.values.set(key, { value, label: decision.cost_multiplier });
      }
    }
  }
  return new Map(
    [...groups].map(([groupID, group]) => [
      groupID,
      {
        accounts: group.accounts,
        values: [...group.values.values()]
          .sort((left, right) => comparePricingDecimals(left.value, right.value))
          .map((cost) => cost.label),
      },
    ]),
  );
}
