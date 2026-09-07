import { createFileRoute } from "@tanstack/react-router";

import { RequestTracePage } from "@/App";

export const Route = createFileRoute("/trace")({ component: RequestTracePage });
