import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RegularCheckPanel } from "../regular-check-panel";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("账号支持 Astra 且检测能力已加载时可选择模型并开始检测", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
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
    sol_models: [],
    astra_models: ["gpt-6-astra"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "41"], { models: ["gpt-6-astra"] });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ categories: [] })),
  );
  render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("checkbox", { name: /选择账号 测试账号/ }));
  fireEvent.click(await screen.findByRole("checkbox", { name: /gpt-6-astra/ }));
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeEnabled();
});
