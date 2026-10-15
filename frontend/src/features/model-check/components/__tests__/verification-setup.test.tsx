import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ModelCheckPage } from "../model-check-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("移除官方对照入口且清空账号或模型时不显示至少选择提示并阻止空任务", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(
    ["accounts"],
    [
      {
        id: "1",
        name: "官方账号",
        platform: "openai",
        groups: [],
        base_url: "https://api.openai.com/v1",
        account_type: "apikey",
      },
    ],
  );
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "1"], { models: ["new-model"] });
  const fetcher = vi.fn<typeof fetch>(async (input, init) => {
    if (String(input) === "/api/model-checks" && init?.method === "POST")
      return Response.json({ id: "created", status: "queued", result: {} });
    return Response.json([]);
  });
  vi.stubGlobal("fetch", fetcher);
  render(
    <QueryClientProvider client={client}>
      <ModelCheckPage />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  expect(screen.queryByText(/官方 API 对照|本次未选择官方对照/)).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "全选账号" }));
  const model = await screen.findByRole("checkbox", { name: /选择模型 new-model/ });
  await user.click(model);
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeEnabled();
  await user.click(model);
  await waitFor(() => expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled());
  expect(screen.queryByText(/请选择至少一个模型/)).not.toBeInTheDocument();
  await user.click(model);
  await user.click(screen.getByRole("button", { name: "清空选择" }));
  expect(screen.queryByText(/请选择至少一个账号|请选择至少一个模型/)).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  expect(fetcher.mock.calls.some((call) => String(call[0]) === "/api/model-checks")).toBe(false);
  await user.click(screen.getByRole("button", { name: "全选账号" }));
  await user.click(await screen.findByRole("checkbox", { name: /选择模型 new-model/ }));
  expect(screen.getByText(/计划 17 项检测/)).toBeVisible();
  await user.click(screen.getByRole("button", { name: /开始检测/ }));
  await waitFor(() =>
    expect(fetcher.mock.calls.some((call) => String(call[0]) === "/api/model-checks")).toBe(true),
  );
  const call = fetcher.mock.calls.find((call) => String(call[0]) === "/api/model-checks")!;
  expect(JSON.parse(String(call[1]?.body))).toMatchObject({
    account_ids: ["1"],
    models: ["new-model"],
    fingerprint: true,
  });
});
