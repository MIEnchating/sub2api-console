import { createFileRoute, redirect } from "@tanstack/react-router";
export const Route = createFileRoute("/uptime-kuma/config")({
  beforeLoad: () => {
    throw redirect({ to: "/config", search: { tab: "monitoring" }, replace: true });
  },
});
