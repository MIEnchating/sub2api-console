import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { RegularCheckPanel } from "../regular-check-panel";

const clients: QueryClient[] = [];

afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

it("选择 OAuth 账号和模型后按稳定 ID 提交检测且不要求前端凭据", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("EventSource", undefined);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  client.setQueryData(
    ["accounts"],
    [{ id: "41", name: "OAuth 账号", groups: [], platform: "openai", account_type: "oauth" }],
  );
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: [],
    astra_models: ["gpt-6-astra"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "41"], { models: ["gpt-6-astra"] });
  const submitted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request, init?: RequestInit): Promise<Response> => {
      if (String(input) === "/api/model-checks" && init?.method === "POST") {
        submitted.push(JSON.parse(String(init.body)));
        return Response.json({
          id: "oauth-model-check",
          status: "succeeded",
          result: { tests: [] },
        });
      }
      if (String(input).includes("account-statuses")) return Response.json([]);
      if (String(input) === "/api/accounts")
        return Response.json([
          { id: "41", name: "OAuth 账号", groups: [], platform: "openai", account_type: "oauth" },
        ]);
      return Response.json({ categories: [] });
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <RegularCheckPanel />
    </QueryClientProvider>,
  );

  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  fireEvent.click(await screen.findByRole("checkbox", { name: /gpt-6-astra/ }));
  fireEvent.click(screen.getByRole("button", { name: /开始检测/ }));

  await waitFor(() => {
    expect(submitted).toEqual([
      { account_ids: ["41"], models: ["gpt-6-astra"], rounds: 1, timeout_seconds: 45 },
    ]);
  });
  expect(await screen.findByRole("button", { name: "查看检测结果" })).toBeEnabled();
  expect(screen.queryByLabelText(/Access Token|API Key|接口地址/)).not.toBeInTheDocument();
});
