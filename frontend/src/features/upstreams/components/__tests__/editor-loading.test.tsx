import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("上游配置读取中显示轻量提示，禁止保存且可用键盘取消", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog host="api.example.test" onOpenChange={onOpenChange} onSaved={vi.fn()} />
    </QueryClientProvider>,
  );
  const status = screen.getByRole("status", { name: "正在读取上游配置" });
  expect(status).toHaveTextContent("正在读取上游配置");
  expect(screen.getByRole("dialog").querySelector('[data-slot="skeleton"]')).toBeNull();
  expect(screen.getByRole("button", { name: "保存并重算" })).toBeDisabled();
  screen.getByRole("button", { name: "取消" }).focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(onOpenChange).toHaveBeenCalledWith(false);
});
