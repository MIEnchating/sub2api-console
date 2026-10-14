import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { RegularCheckPanel } from "../regular-check-panel";
import { toast, Toaster } from "sonner";

const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  toast.dismiss();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(accountsCached = true, capabilitiesCached = true) {
  const client = createConsoleQueryClient();
  clients.push(client);
  client.setQueryData(["dictionaries", "group"], { items: [] });
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  if (accountsCached)
    client.setQueryData(
      ["accounts"],
      [{ id: "41", name: "测试账号", groups: [], platform: "openai" }],
    );
  if (capabilitiesCached)
    client.setQueryData(["model-check-capabilities"], {
      claude_standards: [],
      sol_models: ["gpt-5.6-sol"],
    });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "41"], { models: ["gpt-5.6-sol"] });
  const view = render(
    <QueryClientProvider client={client}>
      <Toaster />
      <RegularCheckPanel />
    </QueryClientProvider>,
  );
  return { client, ...view };
}

it("账号后台刷新失败时继续展示缓存账号和已有选择", async () => {
  vi.spyOn(api, "accounts").mockRejectedValue(new Error("账号暂时不可用"));
  const view = setup();
  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  await act(() => view.client.refetchQueries({ queryKey: ["accounts"] }));
  const table = screen.getByTestId("model-check-account-desktop-table");
  expect(within(table).getByRole("checkbox", { name: /测试账号/ })).toBeChecked();
  view.unmount();
  view.client.clear();
});

it("账号首次读取失败时提供重新读取入口并在恢复后展示账号", async () => {
  const accounts = vi.spyOn(api, "accounts").mockRejectedValue(new Error("账号暂时不可用"));
  const view = setup(false);
  const retry = await screen.findByRole("button", { name: "重新读取" });
  accounts.mockResolvedValue([]);
  fireEvent.click(retry);
  expect(await screen.findByText("没有匹配的账号")).toBeVisible();
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  view.unmount();
  view.client.clear();
});

it("旧画像能力接口不可用时仍可选择账号支持的新模型", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const capabilities = vi
    .spyOn(api, "modelCheckCapabilities")
    .mockRejectedValue(new Error("旧接口不可用"));
  vi.spyOn(api, "accountModels").mockResolvedValue({ models: ["new-model"] });
  const view = setup(true, false);
  fireEvent.click(screen.getByRole("button", { name: "全选账号" }));
  fireEvent.click(screen.getByRole("button", { name: "刷新模型" }));
  const model = await screen.findByRole("checkbox", { name: /new-model/ });
  fireEvent.click(model);
  expect(model).toBeChecked();
  expect(screen.getByRole("button", { name: /开始检测/ })).toBeEnabled();
  expect(capabilities).not.toHaveBeenCalled();
  expect(screen.getByText(/计划 7 项检测/)).toBeVisible();
  view.unmount();
});
