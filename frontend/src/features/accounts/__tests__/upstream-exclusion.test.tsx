import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from "@tanstack/react-router";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AccountsPage } from "@/App";
import { seedDictionaries } from "@/test/dictionary-fixtures";
import { account, policy } from "./fixtures";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
it("账号列表全选跳过排除上游，所有行操作禁用，刷新排除状态时移除已选账号", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("scrollTo", vi.fn());
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json([])),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  seedDictionaries(client);
  const excluded = { ...account, name: "排除账号", upstream_excluded: true };
  const available = { ...account, id: "42", name: "可用账号" };
  client.setQueryData(["accounts"], [excluded, available]);
  client.setQueryData(["policy"], policy);
  client.setQueryData(["model-check-account-statuses"], []);
  const router = createRouter({
    routeTree: createRootRoute({ component: AccountsPage }),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  await router.load();
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  const checkbox = await screen.findByRole("checkbox", { name: "选择账号 排除账号（#41）" });
  expect(checkbox).toHaveAttribute("aria-disabled", "true");
  expect(screen.getByRole("switch", { name: "锁定分组：排除账号" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  const row = screen.getByRole("row", { name: /排除账号/ });
  for (const button of within(within(row).getByRole("group", { name: "账号操作" })).getAllByRole(
    "button",
  ))
    expect(button).toBeDisabled();
  await userEvent.setup().click(screen.getByRole("checkbox", { name: "选择当前页账号" }));
  expect(checkbox).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: "选择账号 可用账号（#42）" })).toBeChecked();
  await act(async () =>
    client.setQueryData(["accounts"], [excluded, { ...available, upstream_excluded: true }]),
  );
  await waitFor(() =>
    expect(screen.getByRole("checkbox", { name: "选择账号 可用账号（#42）" })).not.toBeChecked(),
  );
  expect(screen.getByRole("checkbox", { name: "选择当前页账号" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
});
