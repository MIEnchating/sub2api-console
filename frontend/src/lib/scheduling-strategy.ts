export const schedulingStrategyOptions = [
  {
    value: "balanced",
    label: "均衡",
    description: "质量分 = 健康门控 ×（配置占比 × 相对价格 + 其余占比 × 相对速度）",
  },
  {
    value: "price_first",
    label: "价格优先",
    description: "质量分 = 健康门控 ×（80% 相对价格 + 20% 相对速度）",
  },
  {
    value: "speed_first",
    label: "速度优先",
    description: "质量分 = 健康门控 ×（80% 相对速度 + 20% 相对价格）",
  },
  {
    value: "reliability",
    label: "稳定优先",
    description: "质量分 = 健康门控 ×（75% 健康稳定性 + 15% 相对速度 + 10% 相对价格）",
  },
] as const;

export const schedulingWeightFormula =
  "价格和速度先在组内换算为 0～1 的相对分数；最终权重 = 组内预算 × 质量分 ÷ 质量分总和";

export function schedulingStrategyDescription(value: string): string {
  return (
    schedulingStrategyOptions.find((option) => option.value === value)?.description ??
    schedulingStrategyOptions[0].description
  );
}

export function schedulingStrategyLabel(value: string): string {
  return (
    schedulingStrategyOptions.find((option) => option.value === value)?.label ??
    schedulingStrategyOptions[0].label
  );
}
