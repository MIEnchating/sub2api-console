import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { shareAccountHealthSnapshots } from "../lib/account-health-snapshot";

export function useAccountCollection(paused: boolean) {
  const checks = useQuery({
    queryKey: ["model-check-account-statuses"],
    queryFn: ({ signal }) => api.modelCheckAccountStatuses(signal),
    enabled: !paused,
    staleTime: 30_000,
    refetchInterval: 30_000,
  });
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: ({ signal }) => api.accounts(signal),
    enabled: !paused,
    staleTime: 30_000,
    structuralSharing: shareAccountHealthSnapshots,
    refetchInterval: 30_000,
  });
  return { accounts, checks };
}
