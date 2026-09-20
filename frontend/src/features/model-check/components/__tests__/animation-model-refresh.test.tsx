import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useForm } from "react-hook-form";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import type { AnimationForm } from "../../lib/animation-schema";
import { AnimationModelSettings } from "../animation-model-settings";

const clients: QueryClient[] = [];
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});

function Settings(): ReactElement {
  const form = useForm<AnimationForm>({
    defaultValues: { account_ids: ["41"], unified_model: "manual-model", timeout_seconds: 120 },
  });
  return (
    <>
      <AnimationModelSettings form={form} pending={false} />
      <button onClick={() => form.setValue("account_ids", ["42"])}>切换账号</button>
    </>
  );
}

function renderSettings(): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  client.setQueryData(["model-check-account-models", "41"], { models: ["old-management-alias"] });
  client.setQueryData(["model-animation", "account-models", "41"], {
    models: ["old-upstream-model"],
  });
  render(
    <QueryClientProvider client={client}>
      <Settings />
    </QueryClientProvider>,
  );
}

it("首次点击获取模型时重新读取实际检测接口，不复用管理列表或旧缓存", async () => {
  const user = userEvent.setup();
  const fetcher = vi
    .fn<typeof fetch>()
    .mockResolvedValue(new Response(JSON.stringify({ models: ["current-upstream-model"] })));
  vi.stubGlobal("fetch", fetcher);
  renderSettings();
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/model-checks/animations/accounts/41/models",
      expect.anything(),
    ),
  );
  const input = screen.getByRole("combobox", { name: "检测模型" });
  await user.clear(input);
  await user.click(input);
  expect(await screen.findByRole("option", { name: "current-upstream-model" })).toBeVisible();
  expect(screen.queryByRole("option", { name: /old-/ })).not.toBeInTheDocument();
});

it("切换账号时取消旧请求，旧响应不能混入新账号的模型列表", async () => {
  const user = userEvent.setup();
  let oldSignal: AbortSignal | null | undefined;
  let resolveOld: (response: Response) => void = () => {};
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>().mockImplementation((input, init) => {
      if (String(input).includes("/41/")) {
        oldSignal = init?.signal;
        return new Promise<Response>((resolve) => {
          resolveOld = resolve;
        });
      }
      return Promise.resolve(new Response(JSON.stringify({ models: ["current-account-model"] })));
    }),
  );
  renderSettings();
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(oldSignal).toBeDefined());
  await user.click(screen.getByRole("button", { name: "切换账号" }));
  await waitFor(() => expect(oldSignal?.aborted).toBe(true));
  const input = screen.getByRole("combobox", { name: "检测模型" });
  expect(input).toHaveValue("manual-model");
  await user.clear(input);
  await user.click(input);
  expect(await screen.findByRole("option", { name: "current-account-model" })).toBeVisible();
  await act(async () => resolveOld(new Response(JSON.stringify({ models: ["stale-model"] }))));
  expect(screen.queryByRole("option", { name: "stale-model" })).not.toBeInTheDocument();
});
