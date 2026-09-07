import { createFileRoute } from "@tanstack/react-router";

import { AlertPolicyRoute } from "@/routes/alert-policy-route";

export const Route = createFileRoute("/alert-policy")({ component: AlertPolicyRoute });
