import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderComponent } from "@testing-library/react";
import { renderToStaticMarkup as renderMarkup } from "react-dom/server";
import type { ReactElement } from "react";

function withDictionary(element: ReactElement): ReactElement {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  client.setQueryData(["dictionaries", "scheduling_strategy"], { items: [] });
  return <QueryClientProvider client={client}>{element}</QueryClientProvider>;
}

export function render(element: ReactElement) {
  return renderComponent(withDictionary(element));
}

export function renderToStaticMarkup(element: ReactElement): string {
  return renderMarkup(withDictionary(element));
}
