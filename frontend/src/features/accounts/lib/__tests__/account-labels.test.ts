import { describe, expect, it } from "vitest";

import { accountTypeLabel, accountTypeOptions, accountTypeValue } from "../account-labels";

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
