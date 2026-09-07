import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { toast } from "sonner";
import { PolicyPage } from "@/App";
import type { PolicySnapshot } from "@/api";
import { Toaster } from "@/components/ui/sonner";
const policy: PolicySnapshot = {
  revision: "initial-revision",
  available: true,
  source: "console",
  mode: "完全模式",
  global_strategy: "balanced",
  group_strategies: [
    {
      id: "6",
      name: "codex",
      platforms: ["openai"],
      strategy: "price_first",
      strategy_source: "group_override",
      participation_status: "participating",
      participation_reason: null,
      account_count: 3,
    },
    {
      id: null,
      name: "缺少稳定 ID",
      platforms: [],
      strategy: "balanced",
      strategy_source: "global_default",
      participation_status: "configuration_error",
      participation_reason: "缺少稳定分组 ID",
      account_count: 1,
    },
  ],
  missing_rate_fallback: "current_cost_wall",
  change_threshold: "0.1",
  cooldown_seconds: 60,
  auto_apply: {
    schedulable: true,
    priority: true,
    load_factor: false,
    concurrency: true,
  },
  excluded_group_ids: [],
  traffic_enabled: true,
  probe_interval_seconds: 300,
  probe_model: "gpt-5.1-codex",
  traffic_lookback_minutes: 120,
  max_samples_per_account: 60,
  advanced_policy: {
    probe: {
      concurrency: 4,
      retry_enabled: true,
      retry_source: "fixed",
      retry_count: 2,
      retry_status_codes: [429, 503],
    },
    traffic: { refresh_seconds: 60 },
    weights: {},
    manual_priority: { reserved_max: 10 },
    scope: {
      manage_all_accounts: true,
      managed_group_mode: "all",
      managed_group_ids: [],
      paused_account_ids: [],
    },
    upstream_multiplier: { interval_seconds: 120 },
    writeback: { concurrency: 4, verification: false },
    scaling: {
      enabled: false,
      global_max_concurrency: 900,
      min_per_account: 3,
      max_per_account: 250,
      scale_up_ratio: 0.8,
      step_up: 5,
      step_down: 5,
      cooldown_seconds: 60,
    },
  },
  configuration_errors: [],
};

it("其他页面新增暂停账号后保存未修改的守护范围，应保留新的暂停保护", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  const initial = structuredClone(policy);
  client.setQueryData(["policy"], initial);
  client.setQueryData(["config"], { probes_enabled: true });
  let latest = structuredClone(initial);
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    if (String(input).includes("/api/policy")) {
      if (init?.method === "PUT") saved = JSON.parse(String(init.body));
      return new Response(JSON.stringify(latest), {
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response(JSON.stringify({ probes_enabled: true }), {
      headers: { "Content-Type": "application/json" },
    });
  });
  try {
    render(
      <QueryClientProvider client={client}>
        <PolicyPage />
      </QueryClientProvider>,
    );
    expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled();
    latest = {
      ...initial,
      revision: "latest-revision",
      advanced_policy: {
        ...initial.advanced_policy,
        scope: {
          ...(initial.advanced_policy.scope as Record<string, unknown>),
          paused_account_ids: ["41"],
        },
      },
    };
    await act(async () => {
      await client.refetchQueries({ queryKey: ["policy"], exact: true });
    });
    fireEvent.click(screen.getByRole("button", { name: "保存策略" }));
    await waitFor(() => expect(saved).toBeDefined());
    const advanced = saved?.advanced_policy as Record<string, unknown> | undefined;
    const scope = advanced?.scope as Record<string, unknown> | undefined;
    expect(scope?.paused_account_ids ?? ["41"]).toContain("41");
    expect(saved?.expected_revision).toBe("latest-revision");
  } finally {
    cleanup();
    toast.dismiss();
    client.clear();
    vi.unstubAllGlobals();
  }
});

it("已有本地修改时保留原版本并展示并发冲突，不用新版本覆盖旧草稿", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  const initial = structuredClone(policy);
  client.setQueryData(["policy"], initial);
  client.setQueryData(["config"], { probes_enabled: true });
  let latest = initial;
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal("fetch", async (input: string, init?: RequestInit) => {
    if (String(input).includes("/api/policy") && init?.method === "PUT") {
      saved = JSON.parse(String(init.body));
      return new Response(
        JSON.stringify({
          code: "conflict",
          detail: "策略已被其他操作修改，请刷新策略后重新应用本次改动",
        }),
        { status: 409 },
      );
    }
    return new Response(
      JSON.stringify(String(input).includes("/api/policy") ? latest : { probes_enabled: true }),
    );
  });
  try {
    render(
      <QueryClientProvider client={client}>
        <PolicyPage />
        <Toaster />
      </QueryClientProvider>,
    );
    const scheduling = screen.getByRole("switch", { name: "调度状态" });
    fireEvent.click(scheduling);
    expect(scheduling).not.toBeChecked();
    latest = {
      ...initial,
      revision: "newer-revision",
      advanced_policy: { ...initial.advanced_policy, scope: { paused_account_ids: ["41"] } },
    };
    await act(async () => {
      await client.refetchQueries({ queryKey: ["policy"], exact: true });
    });
    fireEvent.click(screen.getByRole("button", { name: "保存策略" }));

    await waitFor(() => expect(saved?.expected_revision).toBe("initial-revision"));
    expect((saved?.auto_apply as Record<string, boolean>).schedulable).toBe(false);
    expect(scheduling).not.toBeChecked();
    expect(
      await screen.findByText("策略已被其他操作修改，请刷新策略后重新应用本次改动"),
    ).toBeVisible();
  } finally {
    cleanup();
    toast.dismiss();
    client.clear();
    vi.unstubAllGlobals();
  }
});
