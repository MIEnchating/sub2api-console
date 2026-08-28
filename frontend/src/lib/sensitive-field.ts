export const configuredSecretPlaceholder = "已配置，留空则不修改";

export function sensitiveFieldPlaceholder(configured: boolean, emptyPlaceholder: string): string {
  return configured ? configuredSecretPlaceholder : emptyPlaceholder;
}
