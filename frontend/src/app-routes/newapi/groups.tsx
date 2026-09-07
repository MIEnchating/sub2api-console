import { createFileRoute } from "@tanstack/react-router";

import { NewAPIGroupsRoute } from "@/routes/newapi-routes";

export const Route = createFileRoute("/newapi/groups")({ component: NewAPIGroupsRoute });
