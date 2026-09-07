import { z } from "zod";

import type { ModelPriceAdjustmentDirection } from "./model-price-adjustment";

export function modelPriceAdjustmentSchema(direction: ModelPriceAdjustmentDirection) {
  const maximum = direction === "decrease" ? 99.99 : 1000;
  return z.object({
    percentage: z
      .number({ message: "请输入调整百分比" })
      .finite("请输入有效百分比")
      .min(0.01, "调整百分比必须大于 0")
      .max(maximum, `调整百分比不能超过 ${maximum}%`),
  });
}

export type ModelPriceAdjustmentValues = z.infer<ReturnType<typeof modelPriceAdjustmentSchema>>;
