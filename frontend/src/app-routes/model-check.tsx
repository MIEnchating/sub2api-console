import { createFileRoute } from "@tanstack/react-router";

import { ModelCheckPage } from "@/features/model-check/components/model-check-page";

export const Route = createFileRoute("/model-check")({ component: ModelCheckPage });
