export type NewAPIManagementView = "groups" | "channels" | "prices" | "differences";

export const modelPriceSourceLabels = {
  remote: "远程价卡",
  sub2api: "Sub2API 默认",
} as const;
