import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AccountDetail } from "@/api";
import { account, policy } from "../../__tests__/fixtures";
import { AccountSettingsPanel } from "../account-settings-panel";

let client: QueryClient;
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (!init?.method && String(input) === "/api/policy") return Response.json(policy);
      throw new Error("校验失败不得提交请求");
    }),
  );
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.unstubAllGlobals();
});

it.each([
  { label: "优先级", error: "请输入正整数" },
  { label: "并发上限", error: "请输入正整数" },
  { label: "负载因子", error: "负载因子必须大于或等于 1" },
])("清空 $label 后提交会关联错误并标记无效，修正后清除", async ({ label, error }) => {
  const user = userEvent.setup();
  const detail: AccountDetail = {
    ...account,
    metadata: {},
    group_rates: {},
    group_ids: {},
    bindings: [],
    test_models: ["probe-model"],
  };
  render(
    <QueryClientProvider client={client}>
      <AccountSettingsPanel
        accountId={detail.id}
        query={{ data: detail, isLoading: false, isError: false, error: null }}
        onCancel={vi.fn()}
        onSaved={vi.fn()}
      />
    </QueryClientProvider>,
  );
  const input = screen.getByRole("spinbutton", { name: label });
  await user.clear(input);
  await user.click(screen.getByRole("button", { name: "保存" }));
  expect(await screen.findByText(error)).toBeVisible();
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAccessibleDescription(error);
  expect(
    vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method && init.method !== "GET"),
  ).toEqual([]);
  await user.type(input, "2");
  await waitFor(() => expect(screen.queryByText(error)).not.toBeInTheDocument());
  expect(input).toHaveAttribute("aria-invalid", "false");
  expect(input).not.toHaveAccessibleDescription(error);
});
