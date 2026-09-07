export function updateSelection<Value extends string>(
  values: Value[],
  value: Value,
  checked: boolean,
): Value[] {
  if (checked) return values.includes(value) ? values : [...values, value];
  return values.filter((item) => item !== value);
}
