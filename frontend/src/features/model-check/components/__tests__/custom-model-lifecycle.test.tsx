import { zodResolver } from "@hookform/resolvers/zod";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useForm } from "react-hook-form";
import type { ReactElement } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { customAnimationSchema, type CustomAnimationForm } from "../../lib/animation-schema";
import { CustomAnimationModelField } from "../custom-animation-model-field";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function Settings(): ReactElement {
  const form = useForm<CustomAnimationForm>({
    resolver: zodResolver(customAnimationSchema),
    defaultValues: {
      base_url: "https://models.example.invalid/v1",
      api_key: "fixture-key",
      platform: "openai",
      model: "",
      timeout_seconds: 120,
    },
  });
  return (
    <>
      <label>
        Base URL
        <input {...form.register("base_url")} />
      </label>
      <CustomAnimationModelField form={form} disabled={false} />
    </>
  );
}

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  let signal: AbortSignal | null | undefined;
  let resolve!: (response: Response) => void;
  const fetcher = vi.fn<typeof fetch>((_input, init) => {
    signal = init?.signal;
    return new Promise<Response>((release) => {
      resolve = release;
    });
  });
  vi.stubGlobal("fetch", fetcher);
  const view = render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );
  return {
    ...view,
    client,
    fetcher,
    signal: () => signal,
    release: (value: Response) => resolve(value),
    user: userEvent.setup(),
  };
}

it("读取自定义模型期间切换地址会中止旧请求，迟到结果不抢焦点或成为新地址的选项", async () => {
  const view = setup();
  await view.user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledOnce());
  const address = screen.getByRole("textbox", { name: "Base URL" });
  await view.user.clear(address);
  await view.user.type(address, "https://new.example.invalid/v1");
  expect(view.signal()?.aborted).toBe(true);
  await act(async () => view.release(Response.json({ models: ["old-model"] })));
  expect(address).toHaveFocus();
  await view.user.click(screen.getByRole("combobox", { name: "检测模型" }));
  expect(screen.queryByRole("option", { name: "old-model" })).not.toBeInTheDocument();
  expect(
    JSON.stringify(
      view.client
        .getQueryCache()
        .getAll()
        .map((query) => query.queryKey),
    ),
  ).not.toContain("fixture-key");
});

it("离开自定义模型输入会取消读取，迟到成功结果不恢复已卸载表单", async () => {
  const view = setup();
  await view.user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalledOnce());
  view.unmount();
  expect(view.signal()?.aborted).toBe(true);
  await act(async () => view.release(Response.json({ models: ["old-model"] })));
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
});
