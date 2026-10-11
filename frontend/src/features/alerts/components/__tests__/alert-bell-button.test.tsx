import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import { AlertBellButton } from "../alert-bell-button";

it("总览缓存更新时只更新告警按钮，不重新渲染同级页面", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  client.setQueryData(["overview"], { open_alerts: 2 });
  const onClick = vi.fn();
  let pageRenders = 0;
  function Page(): ReactNode {
    pageRenders += 1;
    return <main>账号管理</main>;
  }
  const view = render(
    <QueryClientProvider client={client}>
      <AlertBellButton onClick={onClick} />
      <Page />
    </QueryClientProvider>,
  );
  try {
    expect(screen.getByRole("button", { name: "告警：2" })).toBeVisible();
    act(() => client.setQueryData(["overview"], { open_alerts: 3 }));
    expect(await screen.findByRole("button", { name: "告警：3" })).toBeVisible();
    expect(pageRenders).toBe(1);
    fireEvent.click(screen.getByRole("button", { name: "告警：3" }));
    expect(onClick).toHaveBeenCalledOnce();
  } finally {
    view.unmount();
    client.clear();
  }
});
