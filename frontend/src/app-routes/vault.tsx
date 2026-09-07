import { createFileRoute } from "@tanstack/react-router";

import { VaultPage } from "@/features/vault/components/vault-page";

export const Route = createFileRoute("/vault")({ component: VaultPage });
