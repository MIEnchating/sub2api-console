import { seedDictionaries } from "@/test/dictionary-fixtures";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderComponent } from "@testing-library/react";
import { renderToStaticMarkup as renderMarkup } from "react-dom/server";
import type { ReactElement } from "react";

function withDictionary(element: ReactElement): ReactElement {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  seedDictionaries(client);
  return <QueryClientProvider client={client}>{element}</QueryClientProvider>;
}

export function render(element: ReactElement) {
  return renderComponent(withDictionary(element));
}

export function renderToStaticMarkup(element: ReactElement): string {
  return renderMarkup(withDictionary(element));
}
