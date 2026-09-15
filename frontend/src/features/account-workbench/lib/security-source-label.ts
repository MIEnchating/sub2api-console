import type { WorkbenchSecurityBatchSource } from "@/api";

export function securitySourceLabel(source: WorkbenchSecurityBatchSource): string {
  if ("checkpoint_id" in source) return `检查点 ${source.checkpoint_id}；版本 ${source.revision}`;
  if ("oauth_batch_id" in source)
    return `授权批次 ${source.oauth_batch_id}；原第 ${source.index + 1} 项`;
  return `授权来源 ${source.oauth_id}`;
}
