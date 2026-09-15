import { useQuery } from "@tanstack/react-query";
import { useCallback, useId, useState } from "react";

import { api, type OnboardingContext } from "@/api";

type OnboardingPreparation = {
  load: (host: string) => void;
  reset: () => void;
  host: string | null;
  data: OnboardingContext | undefined;
  error: Error | null;
  isPending: boolean;
};

// Keep preparation subscribed through React's remount checks and cancel obsolete
// reads when the user changes upstream or leaves the page.
export function useOnboardingPreparation(): OnboardingPreparation {
  const instanceId = useId();
  const [host, setHost] = useState<string | null>(null);
  const query = useQuery({
    queryKey: ["onboarding-preparation", instanceId, host],
    queryFn: ({ signal }) => api.prepareOnboarding(host!, signal),
    enabled: host !== null,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  const load = useCallback(
    (nextHost: string) => {
      if (nextHost === host) {
        void query.refetch();
      } else {
        setHost(nextHost);
      }
    },
    [host, query.refetch],
  );
  const reset = useCallback(() => setHost(null), []);
  return {
    load,
    reset,
    host,
    data: query.data,
    error: query.error,
    isPending: query.isFetching,
  };
}
