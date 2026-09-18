import { createRouter } from "@tanstack/react-router";

import { routeTree } from "./routeTree.gen";
import { parseConsoleSearch, stringifyConsoleSearch } from "./lib/search-params";

export const router = createRouter({
  routeTree,
  parseSearch: parseConsoleSearch,
  stringifySearch: stringifyConsoleSearch,
  defaultPreload: "intent",
  defaultPreloadStaleTime: 0,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
