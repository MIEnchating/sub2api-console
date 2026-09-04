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

  it("notifies the application when an authenticated request finds an expired session", async () => {
    const browser = new EventTarget();
    const expired = vi.fn();
    browser.addEventListener("sub2api-console:session-expired", expired);
    vi.stubGlobal("window", browser);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ detail: "请先登录控制台" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(api.overview()).rejects.toMatchObject({ message: "登录已过期，请重新登录" });
    expect(expired).toHaveBeenCalledOnce();
  });

  it("keeps a rejected login in the login form instead of treating it as session expiry", async () => {
    const browser = new EventTarget();
    const expired = vi.fn();
    browser.addEventListener("sub2api-console:session-expired", expired);
    vi.stubGlobal("window", browser);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ detail: "账号或密码错误" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    await expect(api.login({ username: "xiaoge", password: "wrong" })).rejects.toMatchObject({
      message: "账号或密码错误",
    });
    expect(expired).not.toHaveBeenCalled();
  });
});

describe("setup initialization request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("sends the setup token only through its dedicated header", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          initialized: true,
          target_configured: true,
          setup_token_required: false,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const payload = {
      username: "operator",
      password: "long-console-password",
      admin_base_url: "https://sub2api.example.test",
      admin_key: "admin-key",
    };

    await api.initialize(payload, "setup-token-secret");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/setup/initialize");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(new Headers(request.headers).get("X-Setup-Token")).toBe("setup-token-secret");
    expect(JSON.parse(String(request.body))).toEqual(payload);
    expect(String(request.body)).not.toContain("setup-token-secret");
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

describe("history request contracts", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reads the latest revenue report and encoded upstream group history", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response("null", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.latestRevenue()).resolves.toBeNull();
    await expect(api.upstreamGroupHistory("api/example")).resolves.toEqual([]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/pricing/revenue/latest");
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "/api/upstreams/api%2Fexample/group-history?limit=200",
    );
  });
});

describe("manual priority request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("assigns and clears one account through the dedicated resource", async () => {
    const taskResponse = () =>
      new Response(JSON.stringify({ id: "task-1", status: "queued", result: {} }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(taskResponse())
      .mockResolvedValueOnce(taskResponse());
    vi.stubGlobal("fetch", fetchMock);

    await api.setAccountManualPriority("account/41", 3, "100", 100, true);
    await expect(api.clearAccountManualPriority("account/41")).resolves.toMatchObject({
      id: "task-1",
      status: "queued",
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/accounts/account%2F41/manual-priority");
    const assign = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(assign.method).toBe("PUT");
    expect(JSON.parse(String(assign.body))).toEqual({
      priority: 3,
      load_factor: "100",
      concurrency: 100,
      sync_balance_multiplier: true,
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/accounts/account%2F41/manual-priority");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("DELETE");
  });
});

describe("account deletion request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("confirms the exact account binding and upstream key IDs", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "delete-task", status: "queued", result: {} }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.deleteAccount({
      account_id: "37",
      account_name: "special-key",
      groups: ["special"],
      management_base_url: "https://management.example.test",
      binding: {
        id: 91,
        upstream_id: "upstream-1",
        upstream_host: "https://upstream.example.test",
        auth_host: "upstream.example.test",
        upstream_key_id: "key-8",
        upstream_key_name: "special-key",
      },
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/accounts/37/delete");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(JSON.parse(String(request.body))).toEqual({
      confirmation_account_id: "37",
      expected_management_base_url: "https://management.example.test",
      expected_binding_id: 91,
      expected_upstream_id: "upstream-1",
      expected_upstream_host: "https://upstream.example.test",
      expected_auth_host: "upstream.example.test",
      expected_upstream_key_id: "key-8",
    });
  });

  it("omits every upstream key field for a management-only deletion", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "delete-task", status: "queued", result: {} }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.deleteAccount({
      account_id: "174",
      account_name: "星筱AI-0.125",
      groups: [],
      management_base_url: "https://management.example.test",
      binding: null,
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      confirmation_account_id: "174",
      expected_management_base_url: "https://management.example.test",
    });
  });

  it("previews and confirms every stable scope for a batch deletion", async () => {
    const preview = {
      accounts: [
        {
          account_id: "37",
          account_name: "bound",
          groups: ["codex"],
          management_base_url: "https://management.example.test",
          binding: {
            id: 91,
            upstream_id: "upstream-1",
            upstream_host: "upstream.example.test",
            auth_host: "auth.example.test",
            upstream_key_id: "key-8",
            upstream_key_name: "key",
          },
        },
        {
          account_id: "38",
          account_name: "unbound",
          groups: [],
          management_base_url: "https://management.example.test",
          binding: null,
        },
      ],
      account_count: 2,
      upstream_key_count: 1,
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify(preview), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: "delete-batch", status: "queued", result: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.accountDeleteBatchPreview(["37", "38"]);
    await api.deleteAccounts(preview);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/accounts/delete-preview");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      account_ids: ["37", "38"],
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/accounts/delete");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      confirmations: preview.accounts.map((account) => ({
        account_id: account.account_id,
        management_base_url: account.management_base_url,
        binding: account.binding,
      })),
    });
  });
});

describe("account control request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("creates a dedicated account task without creating an inspection task", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "control-1", status: "queued", result: {} }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(api.setAccountControl("account/41", "pause")).resolves.toMatchObject({
      id: "control-1",
      status: "queued",
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/accounts/account%2F41/control");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(JSON.parse(String(request.body))).toEqual({ action: "pause" });
  });
});

describe("QQBot target discovery request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("starts and cancels the authenticated target discovery task", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "qqbot-target-1",
            status: "queued",
            result: {},
          }),
          {
            status: 202,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ cancelled: true }), {
          status: 202,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.discoverNotificationTarget({
      app_id: "app-1",
      client_secret: "",
      target_type: "c2c",
    });
    await api.cancelNotificationTargetDiscovery("target/id");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/notifications/target-discovery");
    const start = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(start.method).toBe("POST");
    expect(JSON.parse(String(start.body))).toEqual({
      app_id: "app-1",
      client_secret: "",
      target_type: "c2c",
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/notifications/target-discovery/target%2Fid");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("DELETE");
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
    expect(api.taskEventsURL("task / 1")).toBe("/api/tasks/task%20%2F%201/events");
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

  it("reads persisted model check status for the account selector", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.modelCheckAccountStatuses();

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/model-checks/account-statuses");
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

describe("system log search contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("forwards only request_id and pagination parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ items: [], total: 0, page: 2, page_size: 20 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.systemLogs({
      requestId: "req-42",
      page: 2,
      pageSize: 20,
    });

    const url = new URL(String(fetchMock.mock.calls[0]?.[0]), "http://console.local");
    expect(url.pathname).toBe("/api/ops/system-logs");
    expect(Object.fromEntries(url.searchParams)).toMatchObject({
      request_id: "req-42",
      page: "2",
      page_size: "20",
    });
    expect([...url.searchParams.keys()]).toEqual(["request_id", "page", "page_size"]);
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
      local_group_id: 3,
      upstream_group_id: "codex",
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const body = JSON.parse(String(request.body));
    expect(body).not.toHaveProperty("credentials");
    expect(body).not.toHaveProperty("account_mode");
    expect(body).not.toHaveProperty("multiplier");
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
        local_group_ids: [3, 5],
        upstream_group_id: "group-a",
        platform: "openai",
        account_ids: ["77"],
      },
      {
        host: "https://upstream.test",
        upstream_type: "sub2api",
        local_group_id: 4,
        upstream_group_id: "group-b",
      },
    ]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/onboarding/batch");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    const items = JSON.parse(String(request.body)).items;
    expect(items).toHaveLength(2);
    expect(items[0].local_group_ids).toEqual([3, 5]);
    expect(items[0].platform).toBe("openai");
    expect(items[0].account_ids).toEqual(["77"]);
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
      host: "origin.example.test:8080",
      name: "Upstream",
      base_url: "https://accelerated.example.test:8443/api",
      account_base_url: "https://account-api.example.test/v1",
      upstream_type: "sub2api",
      auth_mode: "sub2api_user_token",
      recharge_rate: "1",
      access_token: "token",
      refresh_token: "refresh",
    });
    await api.prepareOnboarding("upstream.test");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/upstreams");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toMatchObject({
      host: "origin.example.test:8080",
      base_url: "https://accelerated.example.test:8443/api",
      account_base_url: "https://account-api.example.test/v1",
    });
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
    expect(JSON.parse(String(request.body))).toEqual({
      base_url: "https://api.example.test",
    });
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
    expect(JSON.parse(String(init.body))).toEqual({
      account_ids: ["744", "745"],
    });
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
    });

    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      priority: 120,
      load_factor: "2.5",
      concurrency: 3000,
    });
  });

  it("keeps account Host and Base URL as independent editable fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "task-connection" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.syncAccount("42", {
      upstream_host: "upstream.example.test",
      base_url: "https://account-api.example.test/v1",
    });

    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      upstream_host: "upstream.example.test",
      base_url: "https://account-api.example.test/v1",
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
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ cancelled: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.onboardingProbeModels("api.example", "6");
    await api.runOnboardingProbe("api.example", "6", "gpt-5.2");
    await api.cancelOnboardingProbe("api.example", "6");

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
    expect(fetchMock.mock.calls[2]?.[0]).toBe("/api/onboarding/probe/cancel");
    expect(JSON.parse(String((fetchMock.mock.calls[2]?.[1] as RequestInit).body))).toEqual({
      host: "api.example",
      group_id: "6",
    });
  });

  it("previews and submits unbound upstream keys by stable id", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ host: "api.example", keys: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ id: "cleanup-1", status: "queued", result: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await api.previewUnboundUpstreamKeys("api.example");
    await api.cleanupUnboundUpstreamKeys("api.example", ["key-17", "token-a"]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/onboarding/keys/cleanup-preview");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      host: "api.example",
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/onboarding/keys/cleanup");
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      host: "api.example",
      key_ids: ["key-17", "token-a"],
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

  it("reads probe models through the stable group ID endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ group_id: "6", models: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.groupProbeModels("6");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/groups/6/models");
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
    expect(JSON.parse(String(request.body))).toEqual({
      host: "https://upstream.test",
    });
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
    expect(JSON.parse(String(request.body))).toEqual({
      host: "https://upstream.test",
      key_id: "",
    });
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

describe("account rate sync request contract", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("submits the current filtered stable account IDs", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: "account-rate-task" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.syncAccountRates(["11", "12"]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/management/accounts/rates/sync");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      account_ids: ["11", "12"],
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
      account_base_url: "https://account-api.example.test/v1",
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
      account_base_url: "https://account-api.example.test/v1",
      upstream_type: "newapi",
      auth_mode: "newapi_admin_key",
      recharge_rate: "5",
      admin_key: "secret",
      user_id: "9",
    });
  });
});

describe("pricing backup request contracts", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("creates a named backup and restores or deletes it by encoded stable ID", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ id: "backup/1" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.createPricingBackup("调价前");
    await api.restorePricingBackup("backup/1");
    await api.deletePricingBackup("backup/1");

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/pricing/backups");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      name: "调价前",
    });
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/pricing/backups/backup%2F1/restore");
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).method).toBe("POST");
    expect(fetchMock.mock.calls[2]?.[0]).toBe("/api/pricing/backups/backup%2F1");
    expect((fetchMock.mock.calls[2]?.[1] as RequestInit).method).toBe("DELETE");
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

    await api.configureVaultEntry({
      entry: "operator",
      username: "user",
      password: "secret",
    });
    await api.runAuthRecovery("api.example.test", "operator", true);
    await api.runAuthRecoveryBatch(["api.example.test", "backup.example.test"]);

    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      entry: "operator",
      username: "user",
      password: "secret",
    });
    expect(JSON.parse(String((fetchMock.mock.calls[1]?.[1] as RequestInit).body))).toEqual({
      host: "api.example.test",
      entry: "operator",
      accept_login_agreement: true,
    });
    expect(fetchMock.mock.calls[2]?.[0]).toBe("/api/auth-recovery/run-batch");
    expect(JSON.parse(String((fetchMock.mock.calls[2]?.[1] as RequestInit).body))).toEqual({
      hosts: ["api.example.test", "backup.example.test"],
    });
  });
});

describe("New API management request contracts", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses the stable platform ID for refresh and group binding writes", async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve(
        new Response(JSON.stringify({ groups: [], models: [], references: [], differences: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.refreshNewAPIPlatform("production/main");
    await api.saveNewAPIGroupBindings("production/main", [
      {
        newapi_group_id: "vip",
        newapi_group_name: "VIP",
        sub2api_group_id: "6",
        sync_ratio: true,
      },
    ]);

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/newapi/platforms/production%2Fmain/refresh");
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).method).toBe("POST");
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      "/api/newapi/platforms/production%2Fmain/group-bindings",
    );
    const request = fetchMock.mock.calls[1]?.[1] as RequestInit;
    expect(request.method).toBe("PUT");
    expect(JSON.parse(String(request.body))).toEqual({
      bindings: [
        {
          newapi_group_id: "vip",
          newapi_group_name: "VIP",
          sub2api_group_id: "6",
          sync_ratio: true,
        },
      ],
    });
  });

  it("通过受控后端接口读取远程价卡原始文件", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ content: "{}" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.remoteModelPricingSource("production/main");

    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/api/newapi/platforms/production%2Fmain/remote-model-prices/raw",
    );
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).credentials).toBe("include");
  });

  it("keeps the Sub2API service key inside the channel creation request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ id: 9 }), {
        status: 201,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.createNewAPIChannel("platform-1", {
      sub2api_group_id: "6",
      key_id: "key-7",
      base_url: "https://edge.example",
      models: ["gpt-5"],
      newapi_groups: ["default", "vip"],
    });

    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(request.method).toBe("POST");
    expect(JSON.parse(String(request.body))).toEqual({
      sub2api_group_id: "6",
      key_id: "key-7",
      base_url: "https://edge.example",
      models: ["gpt-5"],
      newapi_groups: ["default", "vip"],
    });
  });

  it("creates and references a Sub2API key without exposing its secret", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ key_id: "key-7", name: "标准", group_id: "6" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.createNewAPIChannelKey("platform-1", {
      sub2api_group_id: "6",
      credential_source: "vault",
      vault_entry: "运营账号",
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/newapi/platforms/platform-1/channel-key");
    expect(JSON.parse(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body))).toEqual({
      sub2api_group_id: "6",
      credential_source: "vault",
      vault_entry: "运营账号",
    });
    expect(String((fetchMock.mock.calls[0]?.[1] as RequestInit).body)).not.toContain("secret");
  });

  it("sends only the Sub2API key ID in the model preview body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ models: ["gpt-5"] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await api.fetchNewAPIChannelModels("platform-1", {
      sub2api_group_id: "6",
      key_id: "key-7",
      base_url: "https://edge.example",
    });

    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/newapi/platforms/platform-1/channel-models");
    const request = fetchMock.mock.calls[0]?.[1] as RequestInit;
    expect(JSON.parse(String(request.body))).toEqual({
      sub2api_group_id: "6",
      key_id: "key-7",
      base_url: "https://edge.example",
    });
  });
});
