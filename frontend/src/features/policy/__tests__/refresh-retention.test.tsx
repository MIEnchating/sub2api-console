import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PolicyPage } from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

it.each(["background", "manual"])(
  "%s 刷新失败保留策略草稿并禁止保存，后续后台恢复后仍保留草稿",
  async (mode) => {
    let failed = true;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        failed
          ? Response.json({ detail: "策略暂时不可用" }, { status: 503 })
          : Response.json(policy),
      ),
    );
    client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    client.setQueryData(["policy"], policy);
    client.setQueryData(["config"], { probes_enabled: true });
    client.setQueryData(["dictionaries", "scheduling_strategy"], { items: [] });
    render(
      <QueryClientProvider client={client}>
        <PolicyPage />
      </QueryClientProvider>,
    );
    const field = screen.getByRole("spinbutton", { name: "每组总权重预算" });
    fireEvent.change(field, { target: { value: "500" } });
    if (mode === "manual") fireEvent.click(screen.getByRole("button", { name: "刷新策略" }));
    else
      await act(async () => {
        await client.refetchQueries({ queryKey: ["policy"], exact: true });
      });
    await waitFor(() => expect(client.getQueryState(["policy"])?.status).toBe("error"));
    expect(screen.getByRole("spinbutton", { name: "每组总权重预算" })).toHaveValue(500);
    expect(screen.getByRole("button", { name: "保存策略" })).toBeDisabled();
    failed = false;
    await act(async () => {
      await client.refetchQueries({ queryKey: ["policy"], exact: true });
    });
    expect(screen.getByRole("spinbutton", { name: "每组总权重预算" })).toHaveValue(500);
    await waitFor(() => expect(screen.getByRole("button", { name: "保存策略" })).toBeEnabled());
  },
);
