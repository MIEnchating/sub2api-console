import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterContextProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { UpstreamsPage } from "@/App";
import { router } from "@/router";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("余额下限超过上限时在字段旁提示并标记无效，修正后清除且不弹 message", () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  const notify = vi.spyOn(toast, "error");
  try {
    render(
      <QueryClientProvider client={client}>
        <RouterContextProvider router={router}>
          <UpstreamsPage />
        </RouterContextProvider>
      </QueryClientProvider>,
    );
    const filter = screen.getByRole("form", { name: "上游筛选" });
    const minimum = within(filter).getByRole("spinbutton", { name: "最低余额" });
    const maximum = within(filter).getByRole("spinbutton", { name: "最高余额" });
    fireEvent.change(minimum, { target: { value: "20" } });
    fireEvent.change(maximum, { target: { value: "10" } });
    fireEvent.submit(filter);

    expect(within(filter).getByRole("alert")).toHaveTextContent("最低余额不能大于最高余额");
    expect(minimum).toHaveAttribute("aria-invalid", "true");
    expect(maximum).toHaveAttribute("aria-invalid", "true");
    expect(minimum).toHaveAccessibleDescription("最低余额不能大于最高余额");
    expect(notify).not.toHaveBeenCalled();

    fireEvent.change(maximum, { target: { value: "30" } });
    expect(within(filter).queryByRole("alert")).not.toBeInTheDocument();
    expect(minimum).not.toHaveAttribute("aria-invalid", "true");
    expect(maximum).not.toHaveAttribute("aria-invalid", "true");
  } finally {
    cleanup();
    client.clear();
  }
});
