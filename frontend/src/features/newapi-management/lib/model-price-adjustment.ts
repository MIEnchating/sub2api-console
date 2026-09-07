import type { NewAPIModelPrice } from "@/api";

import { formatModelPriceNumber } from "./pricing-number";

export type ModelPriceAdjustmentDirection = "increase" | "decrease";

const expressionPriceVariablePattern = "(?:cc1h|img_o|p|c|cr|cc|img|ai|ao)";
const expressionNumberPattern = "-?(?:\\d+(?:\\.\\d*)?|\\.\\d+)(?:[eE][+-]?\\d+)?";

function scaleDecimal(value: string, factor: number): string {
  const number = Number(value);
  if (!Number.isFinite(number)) return value;
  return formatModelPriceNumber(number * factor);
}

function scaleOptionalDecimal(value: string | undefined, factor: number): string | undefined {
  if (value === undefined || value.trim() === "") return value;
  return scaleDecimal(value, factor);
}

function scaleBillingExpression(expression: string, factor: number): string {
  const variable = `\\b${expressionPriceVariablePattern}\\b`;
  const token = `(?:${variable}|${expressionNumberPattern})`;
  const products = new RegExp(
    `"(?:\\\\.|[^"\\\\])*"|'(?:\\\\.|[^'\\\\])*'|${token}(?:\\s*\\*\\s*${token})+`,
    "gi",
  );
  const priceVariable = new RegExp(variable, "i");
  return expression.replace(products, (product) => {
    if (product.startsWith('"') || product.startsWith("'") || !priceVariable.test(product)) {
      return product;
    }
    const parts = product.split(/(\s*\*\s*)/);
    for (let index = 0; index < parts.length; index += 2) {
      if (!Number.isFinite(Number(parts[index]))) continue;
      parts[index] = scaleDecimal(parts[index], factor);
      break;
    }
    return parts.join("");
  });
}

export function adjustNewAPIModelPrice(
  price: NewAPIModelPrice,
  direction: ModelPriceAdjustmentDirection,
  percentage: number,
): NewAPIModelPrice {
  const factor = direction === "increase" ? 1 + percentage / 100 : 1 - percentage / 100;
  const adjusted: NewAPIModelPrice = { ...price };

  if (price.model_price?.trim()) {
    adjusted.model_price = scaleDecimal(price.model_price, factor);
    return adjusted;
  }

  if (price.input_ratio.trim()) {
    adjusted.input_ratio = scaleDecimal(price.input_ratio, factor);
  }
  adjusted.input_price = scaleOptionalDecimal(price.input_price, factor);
  adjusted.completion_price = scaleOptionalDecimal(price.completion_price, factor);
  adjusted.cache_create_price = scaleOptionalDecimal(price.cache_create_price, factor);
  adjusted.cache_read_price = scaleOptionalDecimal(price.cache_read_price, factor);

  if (price.billing_mode === "tiered_expr" && price.billing_expr?.trim()) {
    adjusted.billing_expr = scaleBillingExpression(price.billing_expr, factor);
  }

  return adjusted;
}
