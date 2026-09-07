import { createFileRoute } from "@tanstack/react-router";

import { NewAPIPricesRoute } from "@/routes/newapi-routes";

export const Route = createFileRoute("/newapi/prices")({ component: NewAPIPricesRoute });
