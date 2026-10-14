import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { AnimationQualitySettingsDialog } from "../animation-quality-settings";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
function setup(): void {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <AnimationQualitySettingsDialog />
    </QueryClientProvider>,
  );
}
it("启用自动标注后要求账号和模型，保存稳定账号及配置版本", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === "PUT") {
        const body = JSON.parse(String(init.body));
        writes.push(body);
        return Response.json({ ...body, version: 4 });
      }
      if (url.endsWith("/accounts")) return Response.json([{ id: "41", name: "评审账号甲" }]);
      return Response.json({
        enabled: false,
        account_id: "",
        model: "",
        timeout_seconds: 60,
        version: 3,
      });
    }),
  );
  setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  const account = await screen.findByLabelText("评审账号");
  expect(account).toBeDisabled();
  await user.click(screen.getByRole("checkbox", { name: "生成后自动标注质量" }));
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  expect(await screen.findByText("请选择评审账号")).toBeVisible();
  expect(account).toHaveAttribute("aria-invalid", "true");
  expect(writes).toEqual([]);
  await user.selectOptions(account, "41");
  await user.type(screen.getByLabelText("评审模型"), "review-model");
  await user.click(screen.getByRole("button", { name: "保存设置" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(writes).toEqual([
    { enabled: true, account_id: "41", model: "review-model", timeout_seconds: 60, version: 3 },
  ]);
});
it("设置读取失败时保留取消和重试，数据就绪前不可保存", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ detail: "失败" }, { status: 500 })),
  );
  setup();
  await user.click(screen.getByRole("button", { name: "自动标注设置" }));
  expect(await screen.findByRole("button", { name: "重新读取" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "保存设置" })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
