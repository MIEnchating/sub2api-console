import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderComponent } from "@testing-library/react";
import type { ReactElement } from "react";

export function render(element: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  seedDictionaries(client);
  return renderComponent(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}
