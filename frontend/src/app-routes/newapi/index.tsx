import { createFileRoute } from "@tanstack/react-router";

import { NewAPIPlatformRoute } from "@/routes/newapi-routes";

export const Route = createFileRoute("/newapi/")({ component: NewAPIPlatformRoute });
