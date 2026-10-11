import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { RegularCheckPanel } from "../regular-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    ["1", "2", "3", "4", "5", "6"].map((id) => ({
      id,
      name: `账号 ${id}`,
      groups: [],
      platform: "openai",
    })),
  );
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: ["gpt-6-sol", "gpt-6-sol-mini"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  const pending = new Map<string, { signal: AbortSignal; resolve: (response: Response) => void }>();
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    const id = String(input).split("/").at(-2)!;
    const signal = init!.signal!;
    return new Promise<Response>((resolve, reject) => {
      pending.set(id, { signal, resolve });
      signal.addEventListener("abort", () => reject(signal.reason), { once: true });
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const view = render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel />
    </QueryClientProvider>,
  );
  return { ...view, pending, fetcher, user: userEvent.setup() };
}

it("常规检测全选六个账号后最多同时读取四个模型列表，全部返回后才显示共同模型", async () => {
  const view = setup();
  await view.user.click(screen.getByRole("button", { name: "全选账号" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () => {
    for (const request of view.pending.values())
      request.resolve(Response.json({ models: ["gpt-6-sol", "gpt-6-sol-mini"] }));
  });
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(6));
  expect(screen.queryByRole("checkbox", { name: /gpt-6-sol(?!-)/ })).not.toBeInTheDocument();
  await act(async () => {
    for (const id of ["5", "6"])
      view.pending.get(id)!.resolve(Response.json({ models: ["gpt-6-sol"] }));
  });
  expect(await screen.findByRole("checkbox", { name: /gpt-6-sol(?!-)/ })).toBeEnabled();
  expect(screen.queryByRole("checkbox", { name: /gpt-6-sol-mini/ })).not.toBeInTheDocument();
});

it("常规模型读取期间清空选择会中止在途请求，排队账号不再发起请求", async () => {
  const view = setup();
  await view.user.click(screen.getByRole("button", { name: "全选账号" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await view.user.click(screen.getByRole("button", { name: "清空选择" }));
  expect([...view.pending.values()].every((request) => request.signal.aborted)).toBe(true);
  expect(view.fetcher).toHaveBeenCalledTimes(4);
});
