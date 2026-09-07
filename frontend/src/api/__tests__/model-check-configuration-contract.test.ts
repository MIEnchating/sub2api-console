import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type ModelCheckProfilePayload } from "../../api";

const payload: ModelCheckProfilePayload = {
  claude_profiles: {},
  sol_profile: {
    candidate_models: [],
    quick: [],
    reserve: [],
    thresholds: {
      quick: {
        sol_accept_min: 0.6,
        non_sol_accept_max: 0.4,
        subtype_accept_min: 0.6,
        min_coverage: 0.6,
        min_evidence_coverage: 0.5,
      },
      full: {
        sol_accept_min: 0.6,
        non_sol_accept_max: 0.4,
        subtype_accept_min: 0.6,
        min_coverage: 0.6,
        min_evidence_coverage: 0.5,
      },
    },
  },
};

describe("模型检测画像 API 契约", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("保存草稿时发送内容与并发控制指纹", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ active: {}, draft: null, history: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.saveModelCheckDraft({
      expected_fingerprint: "current-fingerprint",
      note: "更新题库",
      payload,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/model-checks/configuration/draft");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("PUT");
    expect(request.credentials).toBe("include");
    expect(JSON.parse(String(request.body))).toEqual({
      expected_fingerprint: "current-fingerprint",
      note: "更新题库",
      payload,
    });
  });

  it("发布和恢复版本时保留目标版本与指纹", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ active: {}, draft: null, history: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.publishModelCheckDraft("draft-fingerprint");
    await api.restoreModelCheckVersion("profile-old", "active-fingerprint", "恢复旧版");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/model-checks/configuration/publish");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      expected_fingerprint: "draft-fingerprint",
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/model-checks/configuration/restore");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      version_id: "profile-old",
      expected_fingerprint: "active-fingerprint",
      note: "恢复旧版",
    });
  });
});
