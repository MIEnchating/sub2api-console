import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { api } from "@/api";
import { ModelCheckPage } from "../model-check-page";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("刷新共同模型期间保留列表节点与选择，失败时展示原因并禁止检测", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(
    ["accounts"],
    [
      {
        id: "41",
        name: "测试账号",
        groups: [],
        platform: "openai",
        account_type: "apikey",
        health: "healthy",
        schedulable: true,
      },
    ],
  );
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: ["gpt-5.6-sol"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "41"], { models: ["gpt-5.6-sol"] });
  let rejectModels: (error: Error) => void = () => {};
  const request = vi.spyOn(api, "accountModels").mockImplementation(
    () =>
      new Promise((_resolve, reject) => {
        rejectModels = reject;
      }),
  );
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckPage />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  const model = await screen.findByRole("checkbox", { name: /gpt-5.6-sol/ });
  fireEvent.click(model);
  const list = screen.getByRole("list", { name: "可检测模型" });
  fireEvent.click(screen.getByRole("button", { name: "刷新模型" }));
  await waitFor(() => expect(request).toHaveBeenCalledOnce());
  expect(screen.getByRole("list", { name: "可检测模型" })).toBe(list);
  expect(model).toBeChecked();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  await act(async () => rejectModels(new Error("模型接口暂时不可用")));
  expect(await screen.findByRole("alert")).toHaveTextContent("模型接口暂时不可用");
  expect(screen.getByRole("list", { name: "可检测模型" })).toBe(list);
  expect(model).toBeChecked();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeDisabled();
  view.unmount();
  client.clear();
});
