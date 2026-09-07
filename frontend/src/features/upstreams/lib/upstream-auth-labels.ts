import { authMethodLabel } from "./upstream-edit-schema";

const recoveryMethodLabels: Record<string, string> = {
  refresh_token: "刷新 Token",
  refresh: "刷新 Token",
  vault: "密码箱",
};

export function authMethodSummary(
  authMethod: string | null | undefined,
  recoveryMethod: string | null | undefined,
): string {
  const methodLabel = authMethod === "sub2api_user_token" ? "Token" : authMethodLabel(authMethod);
  if (!recoveryMethod) return methodLabel;
  const recoveryLabel = recoveryMethodLabels[recoveryMethod] ?? recoveryMethod;
  return `${methodLabel} · 恢复：${recoveryLabel}`;
}
