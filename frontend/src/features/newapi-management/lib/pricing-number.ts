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

export function pricePerMillion(value?: string): string {
  const normalized = value?.trim() ?? "";
  const match = /^(\+?)(\d+)(?:\.(\d*))?(?:e([+-]?\d+))?$/i.exec(normalized);
  if (!match) return "";
  const whole = match[2];
  const fraction = match[3] ?? "";
  const exponent = Number(match[4] ?? "0");
  if (!Number.isSafeInteger(exponent) || Math.abs(exponent) > 100) return "";
  const digits = `${whole}${fraction}`;
  if (!/[1-9]/.test(digits)) return "0";
  const decimalPosition = whole.length + exponent + 6;
  let result: string;
  if (decimalPosition <= 0) {
    result = `0.${"0".repeat(-decimalPosition)}${digits}`;
  } else if (decimalPosition >= digits.length) {
    result = `${digits}${"0".repeat(decimalPosition - digits.length)}`;
  } else {
    result = `${digits.slice(0, decimalPosition)}.${digits.slice(decimalPosition)}`;
  }
  const parts = result.split(".");
  const normalizedWhole = parts[0].replace(/^0+(?=\d)/, "");
  const normalizedFraction = (parts[1] ?? "").replace(/0+$/, "");
  const shifted = normalizedFraction ? `${normalizedWhole}.${normalizedFraction}` : normalizedWhole;
  return formatModelPriceNumber(shifted);
}
