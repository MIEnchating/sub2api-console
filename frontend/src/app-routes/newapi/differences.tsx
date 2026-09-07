import { createFileRoute } from "@tanstack/react-router";

import { NewAPIDifferencesRoute } from "@/routes/newapi-routes";

export const Route = createFileRoute("/newapi/differences")({ component: NewAPIDifferencesRoute });
