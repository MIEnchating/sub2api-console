import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";

export function renderWithRouter(content: ReactNode): void {
  const root = createRootRoute();
  const page = createRoute({ getParentRoute: () => root, path: "/", component: () => content });
  const destination = createRoute({
    getParentRoute: () => root,
    path: "/newapi/account-model-automation",
    component: () => null,
  });
  const router = createRouter({
    routeTree: root.addChildren([page, destination]),
    history: createMemoryHistory(),
  });
  render(<RouterProvider router={router} />);
}
