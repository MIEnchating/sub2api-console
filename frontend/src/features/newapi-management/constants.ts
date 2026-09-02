export type NewAPIManagementView = "groups" | "channels" | "prices" | "differences";

export const newAPIManagementViews: Array<{
  value: NewAPIManagementView;
  label: string;
}> = [
  { value: "groups", label: "分组绑定" },
  { value: "channels", label: "渠道" },
  { value: "prices", label: "模型价格" },
  { value: "differences", label: "价格差异" },
];
