import { QueryClientProvider, type QueryClient } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Toaster, toast } from "sonner";
import { createConsoleQueryClient } from "@/lib/query-client";
import { PricingPage } from "../pricing-page";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  toast.dismiss();
  vi.unstubAllGlobals();
});

it("价格分组请求失败时只显示悬浮提示，不重复显示页内错误", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      Response.json({ detail: "服务暂时不可用", code: "unavailable" }, { status: 503 }),
    ),
  );
  client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false } });
  render(
    <QueryClientProvider client={client}>
      <Toaster />
      <PricingPage />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("服务暂时不可用")).toBeVisible();
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(screen.getAllByText("服务暂时不可用")).toHaveLength(1);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(document.querySelectorAll("[data-sonner-toast]")).toHaveLength(1);
});
