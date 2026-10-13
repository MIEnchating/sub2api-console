import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useForm } from "react-hook-form";
import { afterEach, expect, it, vi } from "vitest";
import type { AnimationForm } from "../../lib/animation-schema";
import { AnimationModelSettings } from "../animation-model-settings";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
function Settings() {
  const form = useForm<AnimationForm>({
    defaultValues: { account_ids: ["41"], unified_model: "gpt-6-astra", timeout_seconds: 120 },
  });
  return <AnimationModelSettings form={form} pending={false} />;
}
it("获取模型后在同一下拉框连续多选，关闭后紧凑显示数量且不增加输入行", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ models: ["gpt-6-astra", "second-model", "third-model"] })),
  );
  render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  expect(screen.queryByRole("button", { name: "添加模型" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await user.click(screen.getByRole("combobox", { name: "检测模型" }));
  await user.click(await screen.findByRole("option", { name: "second-model" }));
  expect(screen.getByRole("option", { name: "second-model" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await user.click(screen.getByRole("option", { name: "third-model" }));
  await user.keyboard("{Escape}");
  const trigger = screen.getByRole("combobox", { name: "检测模型" });
  expect(trigger).toHaveTextContent("+1");
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(trigger).toHaveFocus();
});
