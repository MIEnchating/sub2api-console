import { describe, expect, it } from "vitest";

import { normalizeConfigTab } from "../../constants";

describe("normalizeConfigTab", () => {
  it.each(["connection", "accounts", "notifications", "interface"])(
    "保留有效页签参数 %s",
    (tab) => {
      expect(normalizeConfigTab(tab)).toBe(tab);
    },
  );

  it.each([undefined, null, "unknown", 1])("无效参数 %s 回退到连接设置", (tab) => {
    expect(normalizeConfigTab(tab)).toBe("connection");
  });
});
