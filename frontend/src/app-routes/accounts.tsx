import { createFileRoute } from "@tanstack/react-router";

import { AccountsPage } from "@/App";

export const Route = createFileRoute("/accounts")({ component: AccountsPage });
