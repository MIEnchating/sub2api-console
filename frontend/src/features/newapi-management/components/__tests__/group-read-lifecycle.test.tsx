import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { NewAPIManagementPage } from "../newapi-management-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
const workspace = {
  platforms: [
    {
      id: "primary",
      name: "平台",
      base_url: "https://newapi.example.test",
      user_id: "1",
      admin_key_configured: true,
      updated_at: "",
    },
  ],
  local_groups: [{ id: "6", name: "标准", ratio: "0.5" }],
  bindings: [
    {
      platform_id: "primary",
      newapi_group_id: "vip",
      newapi_group_name: "VIP",
      sub2api_group_id: "6",
      sync_ratio: false,
    },
  ],
};
const groups = [{ id: "vip", name: "VIP", ratio: "1" }];
function mount(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <NewAPIManagementPage view="groups" />
    </QueryClientProvider>,
  );
}

it("分组页只请求远端分组，刷新同时读取本地倍率且等待两端完成", async () => {
  const requests: string[] = [];
  let refreshing = false;
  let finish!: (value: Response) => void;
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url = String(input);
    requests.push(url);
    if (url.endsWith("/groups") || url.endsWith("/refresh"))
      return Promise.resolve(Response.json({ groups }));
    if (url.includes("/dictionaries")) return Promise.resolve(Response.json({ items: [] }));
    if (refreshing)
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    return Promise.resolve(Response.json(workspace));
  });
  mount();
  const ratio = await screen.findByRole("textbox", { name: "VIP 的 Sub2API 管理平台倍率" });
  expect(ratio).toHaveValue("0.5");
  expect(requests.some((url) => url.endsWith("/refresh"))).toBe(false);
  refreshing = true;
  fireEvent.click(screen.getByRole("button", { name: "刷新 New API 数据" }));
  await waitFor(() => expect(finish).toBeDefined());
  expect(screen.getByRole("button", { name: "刷新 New API 数据" })).toBeDisabled();
  expect(ratio).toHaveValue("0.5");
  await act(async () =>
    finish(
      Response.json({
        ...workspace,
        local_groups: [{ ...workspace.local_groups[0], ratio: "0.8" }],
      }),
    ),
  );
  await waitFor(() => expect(ratio).toHaveValue("0.8"));
});

it("分组页离开时取消本地工作区与远端分组读取", async () => {
  const signals: (AbortSignal | null | undefined)[] = [];
  vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input) === "/api/newapi") return Promise.resolve(Response.json(workspace));
    signals.push(init?.signal);
    return new Promise<Response>(() => {});
  });
  mount();
  await waitFor(() => expect(signals).toHaveLength(2));
  cleanup();
  for (const signal of signals) expect(signal?.aborted).toBe(true);
});
