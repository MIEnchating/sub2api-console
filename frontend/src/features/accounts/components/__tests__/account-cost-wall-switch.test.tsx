import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { api } from "@/api";
import { AccountCostWallSwitch } from "../account-cost-wall-switch";

let client: QueryClient;
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderSwitch(enabled?: boolean): void {
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AccountCostWallSwitch accountId="41" enabled={enabled} />
    </QueryClientProvider>,
  );
}

it("默认关闭，键盘开启后保存稳定账号ID并显示开启状态", async () => {
  const save = vi
    .spyOn(api, "setAccountIgnoreCostWall")
    .mockResolvedValue({ ignore_cost_wall: true });
  renderSwitch();
  const control = screen.getByRole("switch", { name: "无视成本墙" });
  expect(control).not.toBeChecked();
  expect(control).toHaveAccessibleDescription(/切换后立即保存/);
  await userEvent.tab();
  expect(control).toHaveFocus();
  await userEvent.keyboard(" ");
  await waitFor(() => expect(control).toBeChecked());
  expect(save).toHaveBeenCalledWith("41", true);
});

it("已开启的账号回显开关，点击后可恢复成本墙控制", async () => {
  const save = vi
    .spyOn(api, "setAccountIgnoreCostWall")
    .mockResolvedValue({ ignore_cost_wall: false });
  renderSwitch(true);
  const control = screen.getByRole("switch", { name: "无视成本墙" });
  expect(control).toBeChecked();
  await userEvent.click(control);
  await waitFor(() => expect(control).not.toBeChecked());
  expect(save).toHaveBeenCalledWith("41", false);
});

it("保存期间禁用重复切换，失败时保留原状态并提示原因", async () => {
  let reject: (error: Error) => void = () => undefined;
  vi.spyOn(api, "setAccountIgnoreCostWall").mockImplementation(
    () =>
      new Promise((_resolve, fail) => {
        reject = fail;
      }),
  );
  const notify = vi.spyOn(toast, "error").mockImplementation(() => "test-toast");
  renderSwitch();
  const control = screen.getByRole("switch", { name: "无视成本墙" });
  await userEvent.click(control);
  expect(control).toHaveAttribute("aria-disabled", "true");
  expect(control).toHaveAttribute("aria-busy", "true");
  reject(new Error("写入失败，请重试"));
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  expect(control).not.toBeChecked();
  expect(notify).toHaveBeenCalled();
});
