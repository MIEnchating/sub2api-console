import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  defaultStringifySearch,
  RouterProvider,
} from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { vi } from "vitest";

import { OnboardingPage } from "@/App";
import { api, type GroupStatus, type OnboardingCandidate, type UpstreamConfiguration } from "@/api";

export const upstream: UpstreamConfiguration = {
  upstream_id: "upstream-1",
  host: "api.example.test",
  name: "测试上游",
  base_url: "https://api.example.test",
  account_base_url: "https://api.example.test",
  upstream_type: "sub2api",
  auth_mode: "sub2api_user_token",
  recharge_rate: "1",
  raw_balance: "10",
  balance: "10",
  has_access_token: true,
  has_refresh_token: false,
  has_admin_key: false,
  has_user_id: false,
  headers: {},
  header_names: [],
  cookie_names: [],
  groups: [],
};

export function boundCandidate(status: string): OnboardingCandidate {
  return {
    number: 1,
    upstream_id: upstream.upstream_id,
    host: upstream.host,
    upstream_name: upstream.name,
    group_id: "7",
    group_name: "已有绑定分组",
    description: null,
    platform: "openai",
    status,
    multiplier: "0.5",
    recommended_binding: "",
    bindable: false,
    can_create_key: false,
    can_bind_existing_key: true,
    bound: true,
    key_present: true,
    upstream_key_id: "key-7",
    upstream_key_name: "已有 Key",
    recharge_rate: "1",
    unavailable_reason: null,
    bound_accounts: [
      {
        binding_id: 1,
        account_id: "41",
        account_name: "绑定账号",
        account_exists: true,
        binding_status: "bound",
        local_group: "原分组",
        local_groups: [{ id: "1", name: "原分组" }],
        upstream_key_id: "key-7",
        upstream_key_name: "已有 Key",
      },
    ],
  };
}

export function renderOnboarding(
  candidate?: OnboardingCandidate,
  directGroup = true,
  groupOverrides?: GroupStatus[],
): QueryClient {
  // JSDOM 26 recurses while matching top-layer selectors; these tests use ordinary popups.
  const matches = Element.prototype.matches;
  vi.spyOn(Element.prototype, "matches").mockImplementation(function (
    this: Element,
    selector: string,
  ) {
    if ([":fullscreen", ":popover-open", ":modal"].includes(selector)) return false;
    return matches.call(this, selector);
  });
  const getComputedStyle = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element, pseudoElement) => {
    if (element instanceof HTMLSelectElement) {
      const style = document.createElement("div").style;
      style.display = "none";
      return style;
    }
    return getComputedStyle(element, pseudoElement);
  });
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  const groups: GroupStatus[] = ["原分组", "备用分组"].map((name, index) => ({
    id: String(index + 1),
    name,
    platform: "openai",
    account_count: 1,
    scheduling_open: 1,
    scheduling_closed: 0,
    scheduling_unknown: 0,
    strategy: "balanced",
    strategy_source: "global_default",
    participation_status: "participating",
    participation_reason: null,
    status: "healthy",
  }));
  vi.spyOn(api, "groups").mockResolvedValue(groupOverrides ?? groups);
  vi.spyOn(api, "config").mockResolvedValue({
    database_available: true,
    data_database_available: true,
    mode: "完全模式",
    config_keys: [],
    secret_values_hidden: true,
    probes_enabled: false,
    admin_base_url: "https://management.example.test",
    request_timeout_seconds: 30,
    account_default_concurrency: 10,
    account_default_priority: 1,
    initialized: true,
    target_configured: true,
    console_username: "tester",
    configuration_errors: [],
  });
  vi.spyOn(api, "upstreams").mockResolvedValue({
    hosts: [],
    total_hosts: 0,
    authenticated_hosts: 0,
    recovery_required: 0,
    source: "console",
  });
  vi.spyOn(api, "authRecoveryConfig").mockResolvedValue({ auth_records: [], vault_entries: [] });
  vi.spyOn(api, "upstreamConfiguration").mockResolvedValue(upstream);
  vi.spyOn(api, "prepareOnboarding").mockResolvedValue({
    upstream,
    candidates: candidate ? [candidate] : [],
  });
  const root = createRootRoute();
  const route = createRoute({
    getParentRoute: () => root,
    path: "/onboarding",
    component: OnboardingPage,
    validateSearch: (search: Record<string, unknown>) => ({
      host: typeof search.host === "string" ? search.host : undefined,
      upstream_type: typeof search.upstream_type === "string" ? search.upstream_type : undefined,
      group_id: typeof search.group_id === "string" ? search.group_id : undefined,
    }),
  });
  const search = defaultStringifySearch({
    host: upstream.host,
    group_id: directGroup ? "7" : undefined,
  });
  const url = candidate ? `/onboarding${search}` : "/onboarding";
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [url] }),
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return client;
}
