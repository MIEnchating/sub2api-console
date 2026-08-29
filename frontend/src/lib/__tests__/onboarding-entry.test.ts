import { describe, expect, it } from "vitest";

import {
  canSubmitOnboarding,
  candidateCanCreateKey,
  candidateCreationUnavailableReason,
  composeOnboardingBaseUrl,
  localGroupSelectionLabel,
  normalizeOnboardingHost,
  onboardingCandidateStats,
  onboardingEntryKind,
  onboardingRequestHost,
  onboardingSelectionTitle,
  parseOnboardingBaseUrl,
  rechargeRatioLabel,
} from "../onboarding-entry";

describe("onboarding entry workflow", () => {
  it("uses the complete two-step flow when no Host is supplied", () => {
    expect(onboardingEntryKind(undefined, undefined)).toBe("full");
  });

  it("keeps the authorization Host independent from the accelerated Base URL", () => {
    expect(normalizeOnboardingHost("152.53.241.112:8080")).toBe("152.53.241.112:8080");
    expect(
      composeOnboardingBaseUrl({
        baseUrlProtocol: "https",
        baseUrl: "accelerated.example.test:8443/api",
      }),
    ).toBe("https://accelerated.example.test:8443/api");
    expect(
      onboardingRequestHost(
        { host: "152.53.241.112:8080", base_url: "https://accelerated.example.test:8443" },
        "fallback.example.test",
      ),
    ).toBe("152.53.241.112:8080");
  });

  it("preserves HTTP and ports when loading an existing Base URL", () => {
    expect(parseOnboardingBaseUrl("http://10.0.0.8:8080/api")).toEqual({
      baseUrlProtocol: "http",
      baseUrl: "10.0.0.8:8080/api",
    });
  });

  it("skips upstream setup when entering from a Host row", () => {
    expect(onboardingEntryKind("api.example.test", undefined)).toBe("host");
    expect(onboardingSelectionTitle("host")).toBe("选择分组并添加账号");
  });

  it("locks the upstream group only when Host and group ID are both supplied", () => {
    expect(onboardingEntryKind("api.example.test", "group-6")).toBe("group");
    expect(onboardingEntryKind(undefined, "group-6")).toBe("full");
  });

  it("uses new-Key creation capability for account onboarding", () => {
    const candidate = {
      can_create_key: false,
    };

    expect(candidateCanCreateKey(candidate)).toBe(false);
  });

  it("allows account creation when the group has no existing Key", () => {
    const candidate = {
      can_create_key: true,
    };

    expect(candidateCanCreateKey(candidate)).toBe(true);
  });

  it("formats the configured recharge rate as a ratio", () => {
    expect(rechargeRatioLabel("1")).toBe("1:1");
    expect(rechargeRatioLabel("5")).toBe("1:5");
    expect(rechargeRatioLabel(null)).toBe("未配置");
  });

  it("enables account creation only after both stable group IDs are selected", () => {
    expect(canSubmitOnboarding(false, "upstream-6", "17")).toBe(true);
    expect(canSubmitOnboarding(false, null, "17")).toBe(false);
    expect(canSubmitOnboarding(false, "upstream-6", "")).toBe(false);
    expect(canSubmitOnboarding(true, "upstream-6", "17")).toBe(false);
  });

  it("shows the backend Host reason before capability fallbacks", () => {
    expect(
      candidateCreationUnavailableReason({
        unavailable_reason: "上游余额不足",
      }),
    ).toBe("上游余额不足");
  });

  it("explains when a group cannot create a Key without a backend reason", () => {
    expect(
      candidateCreationUnavailableReason({
        unavailable_reason: null,
      }),
    ).toBe("当前无法创建 Key");
  });

  it("shows only the local group name while preserving its stable ID internally", () => {
    expect(
      localGroupSelectionLabel(
        [
          { id: "17", name: "codex" },
          { id: "18", name: "pro" },
        ],
        "17",
      ),
    ).toBe("codex");
  });

  it("does not expose the internal group ID when the selected group is unavailable", () => {
    expect(localGroupSelectionLabel([{ id: "18", name: "pro" }], "17")).toBe("所选分组不可用");
  });

  it("counts selectable and already bound upstream groups separately", () => {
    const stats = onboardingCandidateStats([
      {
        can_create_key: true,
        bound_accounts: [],
      },
      {
        can_create_key: true,
        bound_accounts: [
          {
            binding_id: 7,
            account_id: "41",
            account_name: "api-0.1",
            account_exists: true,
            binding_status: "verified",
            local_group: "codex",
            upstream_key_id: "key-1",
            upstream_key_name: "codex-key",
          },
        ],
      },
      {
        can_create_key: true,
        bound_accounts: [
          {
            binding_id: 8,
            account_id: "42",
            account_name: null,
            account_exists: false,
            binding_status: "missing",
            local_group: "pro",
            upstream_key_id: "key-2",
            upstream_key_name: "pro-key",
          },
        ],
      },
    ]);

    expect(stats).toEqual({ selectable: 3, bound: 1 });
  });
});
