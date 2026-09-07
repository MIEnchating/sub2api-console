export const configTabs = [
  { value: "connection", label: "连接设置" },
  { value: "accounts", label: "账号设置" },
  { value: "notifications", label: "通知设置" },
  { value: "interface", label: "界面与日志" },
] as const;

export type ConfigTab = (typeof configTabs)[number]["value"];

export function normalizeConfigTab(value: unknown): ConfigTab {
  if (typeof value !== "string") return "connection";
  const matchedTab = configTabs.find((tab) => tab.value === value);
  return matchedTab?.value ?? "connection";
}
