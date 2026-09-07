import { createFileRoute } from "@tanstack/react-router";

import { UpstreamsPage } from "@/App";

export const Route = createFileRoute("/upstreams")({ component: UpstreamsPage });
