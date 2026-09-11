import type { Sub2APIModelPrice } from "@/api";

export type NewAPIManagementView = "groups" | "channels" | "prices" | "differences";

export const modelPriceSourceLabels = {
  official: "官方价格",
  remote: "远程价卡",
  sub2api: "Sub2API 默认",
} as const satisfies Record<NonNullable<Sub2APIModelPrice["source"]>, string>;
