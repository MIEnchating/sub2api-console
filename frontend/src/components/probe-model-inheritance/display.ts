import type { GroupStatus } from "@/api";

/** 只展示模型配置的来源，实际探活与跳过判定仍由后端执行。 */
export function probeFallbackLabel(metadata?: Record<string, unknown>): string {
  if (!metadata) return "账号创建并同步模型后，使用首个可用模型";
  const models = (value: unknown): string[] =>
    Array.isArray(value)
      ? value.filter((item): item is string => typeof item === "string" && Boolean(item.trim()))
      : [];
  const known = models(metadata.known_models);
  const enabled = models(metadata.enabled_models);
  const knownAt = Date.parse(String(metadata.known_models_synced_at ?? ""));
  const enabledAt = Date.parse(String(metadata.enabled_models_synced_at ?? ""));
  let catalog = enabled.length ? enabled : known;
  if (known.length && enabled.length && knownAt > enabledAt) catalog = known;
  return catalog.length
    ? `已同步的首个可用模型：${catalog[0].trim()}`
    : "暂无已同步的可用模型，请先同步账号模型";
}

export function inheritedProbeDescription(
  globalModel: string | null,
  fallback: string,
  group?: GroupStatus,
): string {
  const model = group?.override?.probe_model?.trim();
  let description = `全局未指定模型，${fallback}`;
  if (model) description = `继承分组模型：${model}`;
  else if (globalModel?.trim()) description = `继承全局模型：${globalModel.trim()}`;
  if (group?.override?.enabled === false) description += "（分组未参与调度）";
  else if (group?.override?.probe_enabled === false) description += "（分组定时探活已关闭）";
  return description;
}
