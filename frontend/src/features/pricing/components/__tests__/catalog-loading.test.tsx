import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { PricingPage } from "../pricing-page";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

function renderPage(): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["pricing-backups"], []);
  render(
    <QueryClientProvider client={client}>
      <PricingPage />
    </QueryClientProvider>,
  );
}

it("价格目录首次读取时保留独立筛选栏，正文只显示一个与目录一致的五列表格骨架", () => {
  vi.stubGlobal("fetch", () => new Promise<Response>(() => {}));
  renderPage();
  const search = screen.getByPlaceholderText("搜索分组、ID 或平台");
  expect(search.closest('[data-slot="page-content"]')).toBeNull();
  const loading = screen.getByRole("status", { name: "正在读取价格目录" });
  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(loading).toHaveAttribute("aria-busy", "true");
  expect(loading.querySelectorAll("thead th")).toHaveLength(5);
  expect(loading.querySelector('[data-slot="table-container"]')).toHaveClass("overflow-auto");
  for (const placeholder of loading.querySelectorAll('[data-slot="skeleton"]')) {
    expect(placeholder.closest('[aria-hidden="true"]')).not.toBeNull();
  }
});

it("价格目录首次读取失败时显示重新读取入口而非空目录", async () => {
  vi.stubGlobal("fetch", async () => Response.json({ detail: "目录暂时不可用" }, { status: 503 }));
  renderPage();
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.queryByRole("status", { name: "正在读取价格目录" })).not.toBeInTheDocument();
  expect(screen.queryByText("当前没有价格分组")).not.toBeInTheDocument();
});

it("读取价格目录期间输入的筛选条件在请求成功后继续生效", async () => {
  let resolve!: (response: Response) => void;
  vi.stubGlobal(
    "fetch",
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  renderPage();
  fireEvent.change(screen.getByPlaceholderText("搜索分组、ID 或平台"), {
    target: { value: "标准" },
  });
  await act(async () =>
    resolve(
      Response.json({
        config: {
          enabled: false,
          profit_margin: 0.2,
          interval_seconds: 120,
          write_concurrency: 4,
          exchange_group_sets: [],
          exchange_group_set_names: [],
        },
        groups: [
          {
            id: "1",
            name: "标准分组",
            platform: "openai",
            status: "active",
            available: true,
            rate_multiplier: "0.2",
          },
          {
            id: "2",
            name: "备用分组",
            platform: "openai",
            status: "active",
            available: true,
            rate_multiplier: "0.3",
          },
        ],
        decisions: [],
        accounts: 0,
        changes: 0,
        skipped: 0,
        generated_at: "2026-09-14T00:00:00Z",
      }),
    ),
  );
  await waitFor(() => expect(screen.getByText("标准分组")).toBeVisible());
  expect(screen.queryByText("备用分组")).not.toBeInTheDocument();
  expect(screen.getByPlaceholderText("搜索分组、ID 或平台")).toHaveValue("标准");
});
