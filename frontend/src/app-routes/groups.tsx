import { createFileRoute } from "@tanstack/react-router";

import { GroupsPage } from "@/App";

export const Route = createFileRoute("/groups")({ component: GroupsPage });
