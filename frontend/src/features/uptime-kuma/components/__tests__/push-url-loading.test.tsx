import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PushURL } from "../push-url";
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});
it("上报地址仅在请求后显示，读取中有反馈且不能重复请求，隐藏后移除地址", async () => {
  let resolve!: (value: Response) => void;
  const fetch = vi.fn(
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal("fetch", fetch);
  client = new QueryClient();
  render(
    <QueryClientProvider client={client}>
      <PushURL id={19} revision="revision-1" />
    </QueryClientProvider>,
  );
  expect(fetch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "显示上报地址" }));
  expect(await screen.findByRole("status", { name: "正在读取上报地址" })).toBeVisible();
  expect(screen.getByRole("button", { name: "显示上报地址" })).toBeDisabled();
  await act(async () => resolve(Response.json({ url: "https://kuma.example/api/push/test-only" })));
  expect(await screen.findByRole("textbox", { name: "Push 上报地址" })).toHaveValue(
    "https://kuma.example/api/push/test-only",
  );
  fireEvent.click(screen.getByRole("button", { name: "隐藏上报地址" }));
  await waitFor(() =>
    expect(screen.queryByRole("textbox", { name: "Push 上报地址" })).not.toBeInTheDocument(),
  );
});
