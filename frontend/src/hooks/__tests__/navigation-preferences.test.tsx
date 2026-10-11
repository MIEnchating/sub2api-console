import { onlineManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useNavigationPreferences } from "../use-navigation-preferences";
import { NavigationSettingsCard } from "@/features/config/components/navigation-settings-card";
import { navigationPreferencesStorageKey } from "@/lib/navigation-preferences";

const allowed = ["accounts", "config"] as const;
const locked = new Set<(typeof allowed)[number]>(["config"]);
const sections = [
  {
    label: "菜单",
    items: [
      { id: "accounts" as const, label: "账号管理", path: "/accounts" },
      { id: "config" as const, label: "系统设置", path: "/config" },
    ],
  },
];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
const clients: QueryClient[] = [];
function Harness() {
  const preferences = useNavigationPreferences(true, allowed, locked);
  return (
    <NavigationSettingsCard
      sections={sections}
      lockedItemIDs={locked}
      hiddenItemIDs={preferences.hiddenNavigationItemIDs}
      pending={preferences.navigationPending}
      loading={preferences.navigationLoading}
      readFailed={preferences.navigationReadFailed}
      onRetry={preferences.retryNavigation}
      onItemVisibilityChange={preferences.setNavigationItemVisibility}
      onReset={preferences.resetNavigation}
    />
  );
}
function mount(): QueryClient {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>,
  );
  return client;
}
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  localStorage.clear();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

it("保存中启动的旧读取被取消，迟到响应不能覆盖已确认的菜单设置", async () => {
  let finishSave!: (response: Response) => void;
  let finishRead!: (response: Response) => void;
  let readSignal: AbortSignal | null | undefined;
  let reads = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((_url: RequestInfo | URL, options?: RequestInit) => {
      if (options?.method === "PUT")
        return new Promise<Response>((resolve) => {
          finishSave = resolve;
        });
      if (++reads === 1)
        return Promise.resolve(Response.json({ hidden_item_ids: [], version: "v1" }));
      readSignal = options?.signal;
      return new Promise<Response>((resolve) => {
        finishRead = resolve;
      });
    }),
  );
  const client = mount();
  const control = screen.getByRole("switch", { name: "账号管理" });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  await userEvent.setup().click(control);
  act(() => {
    void client.refetchQueries({ queryKey: ["navigation-preferences"] });
  });
  await waitFor(() => expect(reads).toBe(2));
  await act(async () => {
    finishSave(Response.json({ hidden_item_ids: ["accounts"], version: "v2" }));
  });
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "false"));
  await act(async () => {
    finishRead(Response.json({ hidden_item_ids: [], version: "v1" }));
  });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  expect(control).toHaveAttribute("aria-checked", "false");
  expect(readSignal?.aborted).toBe(true);
});

it("浏览器标记离线但本地API可达时仍能保存菜单，不暂停等待联网", async () => {
  let stored = { hidden_item_ids: [] as string[], version: "v1" };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, options?: RequestInit) => {
      if (options?.method === "PUT")
        stored = { ...JSON.parse(String(options.body)), version: "v2" };
      return Response.json(stored);
    }),
  );
  mount();
  const control = screen.getByRole("switch", { name: "账号管理" });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  act(() => onlineManager.setOnline(false));
  await userEvent.setup().click(control);
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "false"));
  expect(stored.hidden_item_ids).toEqual(["accounts"]);
});

it("保存遇到版本冲突时读取其他页面的新设置，再次操作使用新版本", async () => {
  let stored = { hidden_item_ids: [] as string[], version: "v1" };
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, options?: RequestInit) => {
      if (options?.method === "PUT") {
        writes.push(JSON.parse(String(options.body)));
        if (writes.length === 1) {
          stored = { hidden_item_ids: ["accounts"], version: "v2" };
          return Response.json(
            { detail: "菜单设置已在其他页面修改，请重新读取后再操作" },
            { status: 409 },
          );
        }
        stored = { ...JSON.parse(String(options.body)), version: "v3" };
      }
      return Response.json(stored);
    }),
  );
  mount();
  const control = screen.getByRole("switch", { name: "账号管理" });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  await userEvent.setup().click(control);
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "false"));
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  await userEvent.setup().click(control);
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "true"));
  expect(writes).toEqual([
    { hidden_item_ids: ["accounts"], version: "v1" },
    { hidden_item_ids: [], version: "v2" },
  ]);
});

it("首次读取未保存的设置时导入本地显隐，后续新浏览器使用后端设置", async () => {
  let stored: { hidden_item_ids: string[] | null; version: string } = {
    hidden_item_ids: null,
    version: "",
  };
  localStorage.setItem(navigationPreferencesStorageKey, '["accounts"]');
  const fetch = vi.fn(async (_url: RequestInfo | URL, options?: RequestInit) => {
    if (options?.method === "PUT")
      stored = { ...JSON.parse(String(options.body)), version: "saved" };
    return Response.json(stored);
  });
  vi.stubGlobal("fetch", fetch);
  mount();
  await waitFor(() => expect(stored.hidden_item_ids).toEqual(["accounts"]));
  await waitFor(() =>
    expect(screen.getByRole("switch", { name: "账号管理" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    ),
  );
  cleanup();
  localStorage.clear();
  mount();
  await waitFor(() =>
    expect(screen.getByRole("switch", { name: "账号管理" })).toHaveAttribute(
      "aria-checked",
      "false",
    ),
  );
  expect(fetch.mock.calls.filter((call) => call[1]?.method === "PUT")).toHaveLength(1);
});

it("保存未完成时锁定开关且保留已确认设置，失败后恢复操作", async () => {
  let complete: (response: Response) => void = () => undefined;
  const pending = new Promise<Response>((resolve) => {
    complete = resolve;
  });
  const fetch = vi.fn((_url: RequestInfo | URL, options?: RequestInit) =>
    options?.method === "PUT"
      ? pending
      : Promise.resolve(Response.json({ hidden_item_ids: [], version: "v1" })),
  );
  vi.stubGlobal("fetch", fetch);
  mount();
  const control = screen.getByRole("switch", { name: "账号管理" });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  await userEvent.setup().click(control);
  expect(control).toHaveAttribute("aria-disabled", "true");
  expect(control).toHaveAttribute("aria-checked", "true");
  complete(Response.json({ detail: "保存失败，请重试" }, { status: 500 }));
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  expect(control).toHaveAttribute("aria-checked", "true");
  expect(
    JSON.parse(String(fetch.mock.calls.find((call) => call[1]?.method === "PUT")?.[1]?.body)),
  ).toEqual({ hidden_item_ids: ["accounts"], version: "v1" });
});

it("读取失败时禁止覆盖设置，重试成功后可通过键盘恢复默认", async () => {
  let failed = true;
  let stored = { hidden_item_ids: ["accounts"], version: "v1" };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: RequestInfo | URL, options?: RequestInit) => {
      if (failed) return Response.json({ detail: "读取失败" }, { status: 500 });
      if (options?.method === "PUT")
        stored = { ...JSON.parse(String(options.body)), version: "v2" };
      return Response.json(stored);
    }),
  );
  mount();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("switch", { name: "账号管理" })).toHaveAttribute("aria-disabled", "true");
  failed = false;
  await userEvent.setup().click(retry);
  const reset = screen.getByRole("button", { name: "恢复默认" });
  await waitFor(() => expect(reset).toBeEnabled());
  reset.focus();
  await userEvent.setup().keyboard("{Enter}");
  await waitFor(() => expect(stored.hidden_item_ids).toEqual([]));
  expect(screen.getByRole("switch", { name: "系统设置" })).toHaveAttribute("aria-disabled", "true");
});

it("后台读取失败后重试保留菜单，并在读取期间禁止重复重试", async () => {
  let reads = 0;
  let finishRead!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      reads++;
      if (reads === 1) return Response.json({ hidden_item_ids: ["accounts"], version: "v1" });
      if (reads === 2) return Response.json({ detail: "读取失败" }, { status: 500 });
      return new Promise<Response>((resolve) => {
        finishRead = resolve;
      });
    }),
  );
  const client = mount();
  const control = screen.getByRole("switch", { name: "账号管理" });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  await act(async () => {
    await client.refetchQueries({ queryKey: ["navigation-preferences"] });
  });
  const retry = await screen.findByRole("button", { name: "重新读取" });
  await userEvent.setup().click(retry);
  expect(retry).toBeDisabled();
  expect(control).toHaveAttribute("aria-checked", "false");
  expect(control).toHaveAttribute("aria-disabled", "true");
  await userEvent.setup().click(retry);
  expect(reads).toBe(3);
  await act(async () => {
    finishRead(Response.json({ hidden_item_ids: [], version: "v2" }));
  });
  await waitFor(() => expect(control).toHaveAttribute("aria-checked", "true"));
  expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
});
