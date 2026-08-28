export type TaskResultLeaf = { path: string[]; value: string };

function primitiveText(value: unknown): string | null {
  if (value === null) return "空值";
  if (typeof value === "string") return value === "" ? "空值" : value;
  if (typeof value === "boolean") return value ? "开启" : "关闭";
  if (typeof value === "number" || typeof value === "bigint") return String(value);
  if (value === undefined) return "未提供";
  return null;
}

export function flattenTaskResult(value: unknown, path: string[] = []): TaskResultLeaf[] {
  const primitive = primitiveText(value);
  if (primitive !== null) return [{ path, value: primitive }];
  if (Array.isArray(value))
    return value.length
      ? value.flatMap((item, index) => flattenTaskResult(item, [...path, String(index + 1)]))
      : [{ path, value: "空列表" }];
  if (typeof value === "object") {
    const entries = Object.entries(value as Record<string, unknown>);
    return entries.length
      ? entries.flatMap(([key, item]) => flattenTaskResult(item, [...path, key]))
      : [{ path, value: "空对象" }];
  }
  return [{ path, value: String(value) }];
}
