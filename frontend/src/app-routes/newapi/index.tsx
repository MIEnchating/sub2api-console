import { createFileRoute, redirect } from "@tanstack/react-router";
export const Route = createFileRoute("/newapi/")({
  beforeLoad: () => {
    throw redirect({ to: "/config", search: { tab: "newapi" }, replace: true });
  },
});
