import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { PlatformProbeModelsForm } from "../platform-probe-models-form";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
});

it("平台字典含原型属性名时不把未知平台注册成内置模型字段", () => {
  client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
  client.setQueryData(["dictionaries", "platform"], {
    items: [{ value: "constructor", name: "未知平台", enabled: true }],
  });
  render(
    <QueryClientProvider client={client}>
      <PlatformProbeModelsForm models={{}} pending={false} onSubmit={vi.fn()} />
    </QueryClientProvider>,
  );

  expect(screen.queryByRole("textbox", { name: "未知平台 默认探活模型" })).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "OpenAI 默认探活模型" })).toBeEnabled();
});

it.each(["new-model", ""])(
  "将 OpenAI 默认模型改成 %s 时保留未在表单列出的平台配置",
  async (model) => {
    client = new QueryClient({ defaultOptions: { queries: { enabled: false } } });
    const onSubmit = vi.fn();
    render(
      <QueryClientProvider client={client}>
        <PlatformProbeModelsForm
          models={{ openai: "old-model", "custom-platform": "custom-model" }}
          pending={false}
          onSubmit={onSubmit}
        />
      </QueryClientProvider>,
    );
    fireEvent.change(screen.getByRole("textbox", { name: "OpenAI 默认探活模型" }), {
      target: { value: model },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存默认探活模型" }));
    const expected: Record<string, string> = { "custom-platform": "custom-model" };
    if (model) expected.openai = model;
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith(expected));
  },
);
