const PRICE_DECIMAL_PLACES = 8;

export function formatModelPriceNumber(value: unknown): string {
  if (value === "" || value === null || value === undefined || value === false) return "";
  const number = Number(value);
  if (!Number.isFinite(number)) return "";
  return Number.parseFloat(number.toFixed(PRICE_DECIMAL_PLACES)).toString();
}

export function modelPriceNumbersEqual(
  left: string | undefined,
  right: string | undefined,
): boolean {
  const normalizedLeft = left?.trim() ?? "";
  const normalizedRight = right?.trim() ?? "";
  if (!normalizedLeft || !normalizedRight) return normalizedLeft === normalizedRight;
  const formattedLeft = formatModelPriceNumber(normalizedLeft);
  const formattedRight = formatModelPriceNumber(normalizedRight);
  if (!formattedLeft || !formattedRight) return normalizedLeft === normalizedRight;
  return formattedLeft === formattedRight;
}
