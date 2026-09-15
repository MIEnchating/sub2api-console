import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderComponent } from "@testing-library/react";
import type { ReactElement } from "react";

export function render(element: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  client.setQueryData(["dictionaries", "kuma_monitor_type"], { items: [] });
  return renderComponent(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}
