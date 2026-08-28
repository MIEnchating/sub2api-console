import { createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import App from "./App";

const rootRoute = createRootRoute({ component: App });
const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: "/" });
const accountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/accounts",
});
const upstreamsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/upstreams",
});
const groupsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/groups",
});
const autoInspectionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto-inspection",
});
const modelCheckRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/model-check",
});
const logsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/logs",
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
});
const alertPolicyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/alert-policy",
});
const onboardingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/onboarding",
  validateSearch: (search: Record<string, unknown>) => ({
    host: typeof search.host === "string" ? search.host : undefined,
    upstream_type: typeof search.upstream_type === "string" ? search.upstream_type : undefined,
    group_id: typeof search.group_id === "string" ? search.group_id : undefined,
  }),
});
const traceRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/trace",
});
const configRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/config",
});
const vaultRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/vault",
});
const profileRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/profile",
});
const policyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/policy",
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  accountsRoute,
  upstreamsRoute,
  groupsRoute,
  autoInspectionRoute,
  modelCheckRoute,
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
