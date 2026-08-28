import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../../api";

describe("API error detail contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves an explicitly empty detail instead of using the status fallback", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ detail: "" }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(api.setupStatus()).rejects.toMatchObject({ message: "" });
  });

  it("reports an explicit null detail as an empty value instead of treating it as absent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ detail: null }), {
          status: 409,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(api.setupStatus()).rejects.toMatchObject({ message: "空值" });
  });
});

describe("alert request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("clears the complete alert history through the alerts endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ deleted: 3 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.clearAlerts()).resolves.toEqual({ deleted: 3 });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/alerts");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("DELETE");
  });
});

describe("automatic inspection control contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses dedicated endpoints to stop and resume background scheduling", async () => {
    const status = { enabled: false };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ canceled: true, status }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ enabled: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.cancelAutoInspection();
    await api.resumeAutoInspection();

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/inspection/automation/cancel");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/inspection/automation/resume");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("POST");
  });

  it("builds the scheduler event stream through the shared API base", () => {
    expect(api.autoInspectionEventsURL()).toBe("/api/inspection/automation/events");
  });
});

describe("model check request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("submits the selected account and model matrix to the model check task endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "model-check-1", status: "queued" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.runModelCheck({
      account_ids: ["41", "42"],
      models: ["claude-opus-5", "gpt-5.6-sol"],
      rounds: 2,
      timeout_seconds: 45,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/model-checks");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({
      account_ids: ["41", "42"],
      models: ["claude-opus-5", "gpt-5.6-sol"],
      rounds: 2,
      timeout_seconds: 45,
    });
  });
});

describe("request trace contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves the complete request ID as one encoded path segment", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "client:abc/def",
          matched: false,
          account_id: null,
          account_name: null,
          records: [],
          recent_errors: [],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.requestTrace("client:abc/def");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/usage/trace/client%3Aabc%2Fdef");
  });
});

describe("unified log request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("sends log type, state, search and server-side pagination explicitly", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [],
          total: 0,
          page: 2,
          page_size: 20,
          counts: {},
          truncated: false,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.logs({
      kind: "change",
      state: "failed",
      level: "all",
      group: "",
      groupId: "",
      search: "demo account",
      page: 2,
      pageSize: 20,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/api/logs?kind=change&state=failed&level=all&group=&group_id=&search=demo+account&page=2&page_size=20",
    );
  });
});

describe("onboarding request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not expose a manual credentials field for new-account onboarding", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.onboard({
      host: "https://upstream.test",
      upstream_type: "sub2api",
      multiplier: "1",
      local_group_id: 3,
      upstream_group_id: "codex",
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const body = JSON.parse(String(request.body));
    expect(body).not.toHaveProperty("credentials");
    expect(body).not.toHaveProperty("account_mode");
  });

  it("submits multiple explicit group bindings through one batch task", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "batch-task-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.onboardBatch([
      {
        host: "https://upstream.test",
        upstream_type: "sub2api",
        multiplier: "0.2",
        local_group_id: 3,
        upstream_group_id: "group-a",
      },
      {
        host: "https://upstream.test",
        upstream_type: "sub2api",
        multiplier: "0.3",
        local_group_id: 4,
        upstream_group_id: "group-b",
      },
    ]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/onboarding/batch");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(JSON.parse(String(request.body)).items).toHaveLength(2);
  });

  it("creates and verifies the upstream before preparing account candidates", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ host: "upstream.test", candidates: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.createUpstream({
      host: "https://upstream.test",
      name: "Upstream",
      base_url: "https://upstream.test",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
      access_token: "token",
      refresh_token: "refresh",
    });
    await api.prepareOnboarding("upstream.test");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/onboarding/prepare");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      host: "upstream.test",
    });
  });

  it("detects an upstream through the read-only public metadata endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ upstream_type: "newapi", name: "Example API" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.detectUpstream("https://api.example.test");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams/detect");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(JSON.parse(String(request.body))).toEqual({ base_url: "https://api.example.test" });
  });
});

describe("account maintenance request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("submits stable account IDs to the missing-binding cleanup endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "cleanup-1", status: "queued", result: {} }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.cleanupMissingBindings(["744", "745"]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/management/accounts/missing-bindings/cleanup");
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ account_ids: ["744", "745"] });
  });
});

describe("Sub2API connection request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("allows a blank saved Key while sending the configured request timeout", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({}), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.setAdminTarget({
      admin_base_url: "https://sub2api.example.test",
      admin_key: "",
      request_timeout_seconds: 60,
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      admin_base_url: "https://sub2api.example.test",
      admin_key: "",
      request_timeout_seconds: 60,
    });
  });
});

describe("account field request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("preserves an explicit null note clear instead of omitting or stringifying it", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-2" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.syncAccount("42", { notes: null });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({ notes: null });
  });

  it("preserves an explicit empty note independently from null", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-3" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.syncAccount("42", { notes: "" });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({ notes: "" });
  });

  it("writes routing parameters with stable numeric and decimal field types", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-4" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.syncAccount("42", {
      priority: 120,
      load_factor: "2.5",
      concurrency: 3000,
      multiplier: "0.08",
    });

    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      priority: 120,
      load_factor: "2.5",
      concurrency: 3000,
      multiplier: "0.08",
    });
  });

  it("uses dedicated account model endpoints", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ models: ["gpt-5.1-codex"] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ saved: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.accountModels("42");
    await api.setAccountTestModel("42", "gpt-5.1-codex");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/accounts/42/models");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/accounts/42/test-model");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("PUT");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      model: "gpt-5.1-codex",
    });
  });

  it("probes an onboarding group without a local account id", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ models: ["gpt-5.2"] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            status: "passed",
            message: "ok",
            request_model: "gpt-5.2",
            actual_model: "gpt-5.2",
            latency_ms: 80,
            http_status: 200,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.onboardingProbeModels("api.example", "6");
    await api.runOnboardingProbe("api.example", "6", "gpt-5.2");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/onboarding/probe/models");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      host: "api.example",
      group_id: "6",
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/onboarding/probe");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      host: "api.example",
      group_id: "6",
      model: "gpt-5.2",
    });
  });
});

describe("group allocation request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reads allocation through the stable group ID endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ group_id: "6", channels: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.groupAllocation("6");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/groups/6/allocation");
  });
});

describe("rate sync request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("omits key_id when syncing an entire host", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-rate-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.runRateSync("https://upstream.test");

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({ host: "https://upstream.test" });
  });

  it("preserves an explicitly empty key_id so the server can reject it", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-rate-2" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.runRateSync("https://upstream.test", "");

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({ host: "https://upstream.test", key_id: "" });
  });

  it("includes a concrete key_id when syncing one key", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-rate-3" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.runRateSync("https://upstream.test", "key-7");

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      host: "https://upstream.test",
      key_id: "key-7",
    });
  });
});

describe("upstream operator action contracts", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reads balance through the dedicated Host endpoint without a Key payload", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-balance" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.runBalanceSync("api.example.test");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams/api.example.test/balance-sync");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(request.body).toBeUndefined();
  });

  it("starts unified upstream synchronization without exposing credentials in the request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-upstream-sync" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.runUpstreamSync();

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams/sync");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(request.body).toBeUndefined();
  });

  it("uses Host-scoped batch endpoints for top-level balance and group sync", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ id: "task-upstream-scope" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.syncUpstreamBalances();
    await api.syncUpstreamGroups();

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams/balances/sync");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/upstreams/groups/sync");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).body).toBeUndefined();
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).body).toBeUndefined();
  });

  it("submits only manually entered credential fields for server-side verification", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ host: "api.example.test", verified: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.verifyManualAuth({
      host: "api.example.test",
      admin_key: "secret",
      user_id: "9",
      headers: { "X-CF-Access": "signed" },
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      host: "api.example.test",
      admin_key: "secret",
      user_id: "9",
      headers: { "X-CF-Access": "signed" },
    });
  });

  it("deletes an upstream using the exact previewed stable account IDs", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-delete" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.deleteUpstream("api.example.test", ["41", "42"]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams/api.example.test/delete");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      confirmation_host: "api.example.test",
      expected_account_ids: ["41", "42"],
    });
  });

  it("updates one upstream with explicit auth and rate mapping fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ host: "api.example.test" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.updateUpstreamConfiguration("api.example.test", {
      base_url: "https://api.example.test",
      upstream_type: "newapi",
      auth_mode: "newapi_admin_key",
      recharge_rate: "5",
      admin_key: "secret",
      user_id: "9",
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams/api.example.test/configuration");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("PUT");
    expect(JSON.parse(String(request.body))).toEqual({
      base_url: "https://api.example.test",
      upstream_type: "newapi",
      auth_mode: "newapi_admin_key",
      recharge_rate: "5",
      admin_key: "secret",
      user_id: "9",
    });
  });
});

describe("password vault request contracts", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("deletes one exact password entry using encoded identity parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ entry: "operator/primary", deleted: true }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.deleteVaultEntry("operator/primary");

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/api/auth-recovery/vault-entry?entry=operator%2Fprimary",
    );
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("DELETE");
    expect(request.body).toBeUndefined();
  });

  it("creates and selects credentials by entry name without a group field", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ entry: "operator", configured: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.configureVaultEntry({ entry: "operator", username: "user", password: "secret" });
    await api.runAuthRecovery("api.example.test", "operator");

    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      entry: "operator",
      username: "user",
      password: "secret",
    });
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      host: "api.example.test",
      entry: "operator",
    });
  });
});
