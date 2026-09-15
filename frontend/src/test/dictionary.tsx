import { afterEach } from "vitest";
import { renderToStaticMarkup as renderMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as renderComponent } from "@testing-library/react";
import type { ReactElement } from "react";
import type { DictionaryKind } from "@/api";

const clients: QueryClient[] = [];
afterEach(() => clients.splice(0).forEach((client) => client.clear()));

// Seed the real query cache so consumers exercise ordering without external requests.
export function render(
  element: ReactElement,
  dictionaries: Partial<Record<DictionaryKind, Array<{ value: string; name?: string }>>> = {},
) {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  clients.push(client);
  for (const [kind, entries] of Object.entries(dictionaries)) {
    client.setQueryData(["dictionaries", kind], {
      items: entries.map((entry) => ({ ...entry, enabled: true })),
    });
  }
  const view = renderComponent(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>,
  );
  return {
    ...view,
    rerender: (next: ReactElement) =>
      view.rerender(<QueryClientProvider client={client}>{next}</QueryClientProvider>),
    client,
  };
}

export function renderToStaticMarkup(element: ReactElement): string {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  clients.push(client);
  return renderMarkup(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}
