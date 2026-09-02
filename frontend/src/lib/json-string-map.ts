export function parseJsonStringMap(value: string, label: string): Record<string, string> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error(`${label} 必须是有效的 JSON 对象`);
  }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
    throw new Error(`${label} 必须是 JSON 对象`);
  }
  const entries = Object.entries(parsed);
  if (entries.some(([key, item]) => !key.trim() || typeof item !== "string")) {
    throw new Error(`${label} 的名称不能为空，值必须是字符串`);
  }
  return Object.fromEntries(entries) as Record<string, string>;
}
