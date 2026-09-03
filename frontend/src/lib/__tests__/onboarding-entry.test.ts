import { describe, expect, it } from "vitest";

import {
  adjacentOnboardingUpstreams,
  canSubmitOnboarding,
  candidateBoundAccountIDs,
  candidateBoundBaseURLs,
  candidateBoundLocalGroupIDs,
  candidateBoundLocalGroups,
  candidateCanCreateKey,
  candidateCreationUnavailableReason,
  candidateHasExistingBinding,
  candidateHasOnboardingChange,
  candidateUsesAccountBaseURL,
  compatibleOnboardingLocalGroups,
  composeOnboardingBaseUrl,
  localGroupSelectionLabel,
  localGroupMultiplierLabel,
  normalizeOnboardingHost,
  onboardingCandidateStats,
  onboardingEntryKind,
  onboardingRequestHost,
  onboardingSelectionTitle,
  onboardingUpstreamRequest,
  parseOnboardingBaseUrl,
  rechargeRatioLabel,
  sameOnboardingGroupSelection,
  upstreamHostFromBaseUrl,
} from "../onboarding-entry";

describe("onboarding entry workflow", () => {
  it("only offers local groups matching the upstream group platform", () => {
    const candidate = { platform: "zhipu" };
    const groups = [
      { id: "27", name: "国模-平价", platform: "openai" },
      { id: "28", name: "GLM 专用", platform: "zhipu" },
      { id: "29", name: "未标注", platform: null },
    ];

    expect(compatibleOnboardingLocalGroups(candidate, groups)).toEqual([groups[1]]);
  });

  it("offers typed local groups when the upstream catalog omits its platform", () => {
    const groups = [
      { id: "6", name: "codex-平价", platform: "openai" },
      { id: "22", name: "Gemini", platform: "gemini" },
      { id: "27", name: "kiro-旗舰", platform: "anthropic" },
      { id: "30", name: "未标注", platform: null },
    ];

    expect(compatibleOnboardingLocalGroups({ platform: null }, groups)).toEqual(groups.slice(0, 3));
  });

  it("finds the previous and next upstream in management-list order", () => {
    const upstreams = [
      { host: "a.example", name: "A", upstream_type: "sub2api" },
      { host: "b.example", name: "B", upstream_type: "newapi" },
      { host: "c.example", name: "C", upstream_type: "sub2api" },
    ];

    expect(adjacentOnboardingUpstreams(upstreams, "B.EXAMPLE")).toEqual({
      previous: upstreams[0],
      next: upstreams[2],
    });
    expect(adjacentOnboardingUpstreams(upstreams, "a.example").previous).toBeNull();
    expect(adjacentOnboardingUpstreams(upstreams, "c.example").next).toBeNull();
    expect(adjacentOnboardingUpstreams(upstreams, "missing.example")).toEqual({
      previous: null,
      next: null,
    });
  });

  it("uses the complete two-step flow when no Host is supplied", () => {
    expect(onboardingEntryKind(undefined, undefined)).toBe("full");
  });

  it("derives the internal Host from the complete upstream address", () => {
    expect(upstreamHostFromBaseUrl("https://Origin.Example.test:8443/api")).toBe(
      "origin.example.test:8443",
    );
    expect(upstreamHostFromBaseUrl("origin.example.test/api")).toBe("");
  });

  it("derives the internal Host and request address from one protocol-aware field", () => {
    const address = composeOnboardingBaseUrl({
      baseUrlProtocol: "https",
      baseUrl: "accelerated.example.test:8443/api",
    });

    expect(normalizeOnboardingHost("192.0.2.44:8080")).toBe("192.0.2.44:8080");
    expect(address).toBe("https://accelerated.example.test:8443/api");
    expect(
      onboardingRequestHost(
        { host: "192.0.2.44:8080", base_url: "https://accelerated.example.test:8443" },
        "fallback.example.test",
      ),
    ).toBe("192.0.2.44:8080");
    expect(onboardingUpstreamRequest(address)).toEqual({
      host: "accelerated.example.test:8443",
      baseUrl: "https://accelerated.example.test:8443/api",
    });
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

  it("prevents duplicate account creation and returns existing local groups", () => {
    const candidate = {
      can_create_key: true,
      bound: true,
      bound_accounts: [
        {
          binding_id: 1,
          account_id: "41",
          account_name: "existing",
          base_url: "https://account-api.example/v1/",
          account_exists: true,
          binding_status: "verified",
          local_group: "codex",
          local_groups: [
            { id: "6", name: "codex" },
            { id: "7", name: "pro" },
          ],
          upstream_key_id: "key-1",
          upstream_key_name: "existing-key",
        },
      ],
    };

    expect(candidateHasExistingBinding(candidate)).toBe(true);
    expect(candidateCanCreateKey(candidate)).toBe(false);
    expect(candidateBoundLocalGroups(candidate)).toEqual(["codex", "pro"]);
    expect(candidateBoundLocalGroupIDs(candidate)).toEqual(["6", "7"]);
    expect(candidateBoundAccountIDs(candidate)).toEqual(["41"]);
    expect(candidateBoundBaseURLs(candidate)).toEqual(["https://account-api.example/v1"]);
    expect(candidateUsesAccountBaseURL(candidate, "https://account-api.example/v1/")).toBe(true);
    expect(candidateUsesAccountBaseURL(candidate, "https://other.example/v1")).toBe(false);
    expect(sameOnboardingGroupSelection(["7", "6"], ["6", "7"])).toBe(true);
  });

  it("marks a bound row changed when only its edited account Base URL differs", () => {
    const candidate = {
      bound: true,
      bound_accounts: [
        {
          binding_id: 1,
          account_id: "41",
          account_name: "existing",
          base_url: "https://account-api.example/v1",
          account_exists: true,
          binding_status: "verified",
          local_group: "codex",
          local_groups: [{ id: "6", name: "codex" }],
          upstream_key_id: "key-1",
          upstream_key_name: "key-codex",
        },
      ],
    };

    expect(candidateHasOnboardingChange(candidate, ["6"], "https://other.example/v1", true)).toBe(
      true,
    );
    expect(candidateHasOnboardingChange(candidate, ["6"], "https://other.example/v1", false)).toBe(
      false,
    );
  });

  it("shows selected local-group multipliers without confusing them with account rates", () => {
    expect(
      localGroupMultiplierLabel([
        { rate_multiplier: "0.08" },
        { rate_multiplier: "0.08" },
        { rate_multiplier: "0.12" },
      ]),
    ).toBe("0.08、0.12");
    expect(localGroupMultiplierLabel([{ rate_multiplier: null }])).toBe("未设置");
    expect(localGroupMultiplierLabel([])).toBe("—");
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

    expect(stats).toEqual({ selectable: 2, bound: 1 });
  });
});
