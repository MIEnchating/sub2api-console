import { createFileRoute } from "@tanstack/react-router";

import { PolicyPage } from "@/App";

export const Route = createFileRoute("/policy")({ component: PolicyPage });
