import { describe, expect, it } from "vitest";

import type { GroupStatus } from "@/api";

import {
  accountTypeLabel,
  accountTypeOptions,
  accountTypeValue,
  groupPlatformSummary,
} from "../account-labels";

const group: GroupStatus = {
  name: "platform-label",
  id: "1",
  account_count: 0,
  scheduling_open: 0,
  scheduling_closed: 0,
  scheduling_unknown: 0,
  strategy: "balanced",
  strategy_source: "global_default",
  participation_status: "participating",
  participation_reason: null,
  status: "empty",
  override: null,
};

describe("account type dictionary", () => {
  it("keeps account type options fixed when no accounts are loaded", () => {
    expect(accountTypeOptions.map((option) => option.value)).toEqual([
      "apikey",
      "oauth",
      "sub2api",
      "newapi",
      "oneapi",
    ]);
  });

  it("normalizes API Key aliases to the canonical filter value", () => {
    expect(accountTypeValue("api_key")).toBe("apikey");
    expect(accountTypeValue("api-key")).toBe("apikey");
  });

  it("marks unknown account types instead of treating them as dictionary values", () => {
    expect(accountTypeLabel("future-account")).toBe("未知类型（future-account）");
  });
});

describe("group platform dictionary", () => {
  it.each([
    ["anthropic", "Anthropic"],
    ["openai", "OpenAI"],
    ["gemini", "Gemini"],
    ["antigravity", "Antigravity"],
    ["grok", "Grok"],
    ["kimi", "Kimi"],
    ["zhipu", "Zhipu GLM"],
    ["deepseek", "DeepSeek"],
    ["composite", "Composite"],
  ])("displays Sub2API platform %s as %s", (platform, label) => {
    expect(groupPlatformSummary({ ...group, platform })).toBe(label);
  });
});
