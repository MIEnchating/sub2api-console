import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useForm } from "react-hook-form";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationForm } from "../../lib/animation-schema";
import { AnimationModelSettings } from "../animation-model-settings";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function Settings(): ReactElement {
  const form = useForm<AnimationForm>({
    defaultValues: {
      account_ids: ["1", "2", "3", "4", "5", "6"],
      unified_model: "",
      timeout_seconds: 120,
    },
  });
  return <AnimationModelSettings form={form} pending={false} />;
}

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  const pending = new Map<string, { signal: AbortSignal; resolve: (response: Response) => void }>();
  const fetcher = vi.fn<typeof fetch>().mockImplementation((input, init) => {
    const id = String(input).split("/").at(-2)!;
    const signal = init!.signal!;
    return new Promise<Response>((resolve, reject) => {
      pending.set(id, { signal, resolve });
      signal.addEventListener("abort", () => reject(signal.reason), { once: true });
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const view = render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );
  return { ...view, pending, fetcher, user: userEvent.setup() };
}

it("六个账号获取共同模型时最多同时读取四个，慢请求不阻塞空闲位置且结果包含全部账号", async () => {
  const view = setup();
  expect(view.fetcher).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  expect(screen.getByRole("button", { name: "获取模型" })).toBeDisabled();
  await act(async () =>
    view.pending.get("2")!.resolve(Response.json({ models: ["common", "partial"] })),
  );
  await waitFor(() => expect(view.pending.has("5")).toBe(true));
  await act(async () =>
    view.pending.get("3")!.resolve(Response.json({ models: ["common", "partial"] })),
  );
  await waitFor(() => expect(view.pending.has("6")).toBe(true));
  await act(async () => {
    for (const id of ["1", "4", "5"])
      view.pending.get(id)!.resolve(Response.json({ models: ["common", "partial"] }));
    view.pending.get("6")!.resolve(Response.json({ models: ["common"] }));
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled());
  await view.user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(await screen.findByRole("option", { name: "common" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "partial" })).not.toBeInTheDocument();
  expect(view.fetcher).toHaveBeenCalledTimes(6);
});

it("模型响应体仍在读取时保留并发位置，JSON 完成后再启动排队账号", async () => {
  const view = setup();
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const stream = new ReadableStream<Uint8Array>({
    start: (controller) => {
      body = controller;
    },
  });
  await act(async () => view.pending.get("1")!.resolve(new Response(stream)));
  expect(view.fetcher).toHaveBeenCalledTimes(4);
  await act(async () => {
    body.enqueue(new TextEncoder().encode('{"models":["common"]}'));
    body.close();
  });
  await waitFor(() => expect(view.pending.has("5")).toBe(true));
});

it("离开模型选择时中止在途请求并移除排队账号，不再向其发送请求", async () => {
  const view = setup();
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () => view.unmount());
  expect([...view.pending.values()].every((request) => request.signal.aborted)).toBe(true);
  expect(view.fetcher).toHaveBeenCalledTimes(4);
});

it("一个账号读取失败后继续读取剩余账号，重新获取可恢复完整共同模型", async () => {
  const view = setup();
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(4));
  await act(async () =>
    view.pending.get("1")!.resolve(Response.json({ detail: "读取失败" }, { status: 503 })),
  );
  await waitFor(() => expect(view.pending.has("5")).toBe(true));
  await act(async () => view.pending.get("2")!.resolve(Response.json({ models: ["common"] })));
  await waitFor(() => expect(view.pending.has("6")).toBe(true));
  await act(async () => {
    for (const id of ["3", "4", "5", "6"])
      view.pending.get(id)!.resolve(Response.json({ models: ["common"] }));
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled());
  await view.user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(screen.queryByRole("option", { name: "common" })).not.toBeInTheDocument();
  await view.user.keyboard("{Escape}");
  view.pending.clear();
  fireEvent.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(10));
  await act(async () => {
    for (const request of view.pending.values())
      request.resolve(Response.json({ models: ["common"] }));
  });
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledTimes(12));
  await act(async () => {
    for (const id of ["5", "6"])
      view.pending.get(id)!.resolve(Response.json({ models: ["common"] }));
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled());
  await view.user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(await screen.findByRole("option", { name: "common" })).toBeVisible();
});
