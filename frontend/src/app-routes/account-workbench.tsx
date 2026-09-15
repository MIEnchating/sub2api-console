import { createFileRoute } from "@tanstack/react-router";
import { AccountWorkbenchPage } from "@/features/account-workbench/components/account-workbench-page";

export const Route = createFileRoute("/account-workbench")({ component: AccountWorkbenchPage });
