import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
} from "@tanstack/react-router";

import App, {
  AccountsPage,
  AlertsPage,
  AutoInspectionPage,
  GroupsPage,
  OnboardingPage,
  PolicyPage,
  RequestTracePage,
  UpstreamsPage,
} from "./App";

const rootRoute = createRootRoute({ component: App });
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: lazyRouteComponent(() => import("./routes/overview-route"), "OverviewRoute"),
});
const accountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/accounts",
  component: AccountsPage,
});
const upstreamsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/upstreams",
  component: UpstreamsPage,
});
const groupsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/groups",
  component: GroupsPage,
});
const newAPIRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/newapi",
  component: lazyRouteComponent(() => import("./routes/newapi-routes"), "NewAPIPlatformRoute"),
});
const newAPIGroupsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/newapi/groups",
  component: lazyRouteComponent(() => import("./routes/newapi-routes"), "NewAPIGroupsRoute"),
});
const newAPIChannelsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/newapi/channels",
  component: lazyRouteComponent(() => import("./routes/newapi-routes"), "NewAPIChannelsRoute"),
});
const newAPIPricesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/newapi/prices",
  component: lazyRouteComponent(() => import("./routes/newapi-routes"), "NewAPIPricesRoute"),
});
const newAPIDifferencesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/newapi/differences",
  component: lazyRouteComponent(() => import("./routes/newapi-routes"), "NewAPIDifferencesRoute"),
});
const pricingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/pricing",
  component: lazyRouteComponent(() => import("./routes/pricing-routes"), "PricingRoute"),
});
const revenueAnalysisRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/revenue-analysis",
  component: lazyRouteComponent(() => import("./routes/pricing-routes"), "RevenueAnalysisRoute"),
});
const pricingConfigRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/pricing-config",
  component: lazyRouteComponent(() => import("./routes/pricing-routes"), "PricingConfigRoute"),
});
const autoInspectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto-inspection",
  component: AutoInspectionPage,
});
const modelCheckRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/model-check",
  component: lazyRouteComponent(
    () => import("./features/model-check/components/model-check-page"),
    "ModelCheckPage",
  ),
});
const trafficRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/traffic",
  component: lazyRouteComponent(
    () => import("./features/traffic-ranking/components/traffic-ranking-page"),
    "TrafficRankingPage",
  ),
});
const logsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/logs",
  component: lazyRouteComponent(
    () => import("./features/logs/components/logs-center-page"),
    "LogsCenterPage",
  ),
  validateSearch: (search: Record<string, unknown>) => ({
    kind:
      typeof search.kind === "string" && ["all", "task", "event", "change"].includes(search.kind)
        ? search.kind
        : "all",
  }),
});
const alertsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/alerts",
  component: AlertsPage,
});
const alertPolicyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/alert-policy",
  component: lazyRouteComponent(() => import("./routes/alert-policy-route"), "AlertPolicyRoute"),
});
const onboardingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/onboarding",
  component: OnboardingPage,
  validateSearch: (search: Record<string, unknown>) => ({
    host: typeof search.host === "string" ? search.host : undefined,
    upstream_type: typeof search.upstream_type === "string" ? search.upstream_type : undefined,
    group_id: typeof search.group_id === "string" ? search.group_id : undefined,
  }),
});
const traceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/trace",
  component: RequestTracePage,
});
const configRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/config",
  component: lazyRouteComponent(() => import("./routes/config-route"), "ConfigRoute"),
});
const vaultRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/vault",
  component: lazyRouteComponent(
    () => import("./features/vault/components/vault-page"),
    "VaultPage",
  ),
});
const profileRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/profile",
  component: lazyRouteComponent(
    () => import("./features/profile/components/profile-page"),
    "ProfilePage",
  ),
});
const policyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/policy",
  component: PolicyPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  accountsRoute,
  upstreamsRoute,
  groupsRoute,
  newAPIRoute,
  newAPIGroupsRoute,
  newAPIChannelsRoute,
  newAPIPricesRoute,
  newAPIDifferencesRoute,
  pricingRoute,
  revenueAnalysisRoute,
  pricingConfigRoute,
  autoInspectionRoute,
  modelCheckRoute,
  trafficRoute,
  logsRoute,
  alertsRoute,
  alertPolicyRoute,
  onboardingRoute,
  traceRoute,
  vaultRoute,
  profileRoute,
  configRoute,
  policyRoute,
]);
export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
