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

it("取消账号选择后终止旧模型读取，迟到模型不会重新出现", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    [{ id: "41", name: "测试账号", groups: [], platform: "openai" }],
  );
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: ["gpt-6-sol"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  let signal: AbortSignal | null | undefined;
  let release!: (response: Response) => void;
  const fetcher = vi.fn<typeof fetch>().mockImplementation((_input, init) => {
    signal = init?.signal;
    return new Promise<Response>((resolve) => {
      release = resolve;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "全选账号" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith("/api/accounts/41/models", expect.anything()),
  );
  try {
    await user.click(screen.getByRole("button", { name: "清空选择" }));
    expect(signal?.aborted).toBe(true);
  } finally {
    await act(async () => release(Response.json({ models: ["gpt-6-sol"] })));
  }
  expect(screen.queryByRole("checkbox", { name: /gpt-6-sol/ })).not.toBeInTheDocument();
  expect(client.getQueryData(["model-check-account-models", "41"])).toBeUndefined();
});
