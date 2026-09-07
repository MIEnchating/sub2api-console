import { createFileRoute } from "@tanstack/react-router";

import { NewAPIChannelsRoute } from "@/routes/newapi-routes";

export const Route = createFileRoute("/newapi/channels")({ component: NewAPIChannelsRoute });
