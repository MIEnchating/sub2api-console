import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AnimationQualitySettingsDialog } from "../animation-quality-settings";

let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.unstubAllGlobals();
});

function setup(readModels: () => Promise<Response>): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      if (url.endsWith("/models")) return readModels();
      if (url.endsWith("/accounts"))
        return Response.json([
          { id: "41", name: "评审账号甲" },
          { id: "42", name: "评审账号乙" },
        ]);
      return Response.json({
        enabled: true,
        account_id: "41",
        model: "",
        timeout_seconds: 120,
        version: 0,
      });
    }),
  );
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AnimationQualitySettingsDialog />
    </QueryClientProvider>,
  );
}

it("获取模型后可用键盘选择，切换账号清除旧模型并等待重新获取", async () => {
  setup(async () => Response.json({ models: ["review-a", "review-b"] }));
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  const model = await screen.findByRole("combobox", { name: "评审模型" });
  expect(model).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(model).toBeEnabled());
  model.focus();
  await user.keyboard("{ArrowDown}");
  expect(await screen.findByRole("option", { name: "review-a" })).toBeVisible();
  expect(model).toHaveAttribute("aria-expanded", "true");
  await user.keyboard("{Enter}");
  expect(model).toHaveTextContent("review-a");
  expect(model).toHaveAttribute("aria-expanded", "false");
  await user.click(screen.getByRole("combobox", { name: "评审账号" }));
  await user.click(screen.getByRole("option", { name: "评审账号乙 · 42" }));
  expect(model).toBeDisabled();
  expect(model).toHaveTextContent("获取模型后选择评审模型");
});

it.each(["empty", "failure"])("模型列表为 %s 时保持不可选并允许重新获取", async (result) => {
  setup(async () =>
    result === "empty"
      ? Response.json({ models: [] })
      : Response.json({ detail: "上游账号余额不足" }, { status: 502 }),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  const model = await screen.findByRole("combobox", { name: "评审模型" });
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "获取模型" })).toBeEnabled());
  expect(model).toBeDisabled();
  if (result === "empty")
    expect(screen.getByText("该账号未返回可选模型，可重新获取或更换账号。")).toBeVisible();
});
