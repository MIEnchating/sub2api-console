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
    defaultValues: { account_ids: ["41", "42"], unified_model: "", timeout_seconds: 120 },
  });
  return <AnimationModelSettings form={form} pending={false} />;
}

it("获取共同模型后使用主题下拉展示交集，点击选项回填模型输入", async () => {
  const user = userEvent.setup();
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity } } });
  clients.push(client);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>().mockImplementation(
      async (input) =>
        new Response(
          JSON.stringify({
            models: String(input).includes("/41/") ? ["gpt-5.5", "gpt-6-astra"] : ["gpt-6-astra"],
          }),
        ),
    ),
  );
  render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );

  const input = screen.getByRole("combobox", { name: "检测模型" });
  expect(input).not.toHaveAttribute("list");
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await user.click(input);
  expect(await screen.findByRole("option", { name: "gpt-6-astra" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "gpt-5.5" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("option", { name: "gpt-6-astra" }));
  expect(input).toHaveValue("gpt-6-astra");
});
