import type { QueryClient } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "@/api";
import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("新增账号默认留空并发并由后端预览分配，确认前展示实际份额", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const preview = vi
    .spyOn(api, "previewOnboardingConcurrency")
    .mockResolvedValue({ items: [{ concurrency: 7 }] });
  client = renderOnboarding({
    ...boundCandidate("active"),
    bound: false,
    bound_accounts: [],
    can_create_key: true,
  });
  const concurrency = await screen.findByRole("spinbutton", { name: "并发" });
  expect(concurrency).toHaveValue(null);
  expect(concurrency).toHaveAttribute("placeholder", "自动平分剩余额度");
  fireEvent.click(await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" }));
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "已有绑定分组 本地分组" }), {
    key: "Escape",
  });
  fireEvent.click(screen.getByRole("button", { name: "预览添加账号" }));
  const dialog = await screen.findByRole("dialog", { name: "确认账号绑定变更" });
  expect(preview.mock.calls[0]?.[0][0]?.concurrency).toBeUndefined();
  expect(within(dialog).getByText("7", { exact: true })).toBeVisible();
});

it("上游额度已满时预览展示停用等待状态并允许确认添加", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const submit = vi.spyOn(api, "onboard").mockRejectedValue(new Error("isolated submission"));
  vi.spyOn(api, "previewOnboardingConcurrency").mockResolvedValue({
    items: [{ concurrency: 1, waiting_for_capacity: true }],
  });
  client = renderOnboarding({
    ...boundCandidate("active"),
    bound: false,
    bound_accounts: [],
    can_create_key: true,
  });
  fireEvent.click(await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" }));
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(screen.getByRole("combobox", { name: "已有绑定分组 本地分组" }), {
    key: "Escape",
  });
  fireEvent.click(screen.getByRole("button", { name: "预览添加账号" }));
  const dialog = await screen.findByRole("dialog", { name: "确认账号绑定变更" });
  expect(within(dialog).getByText("等待并发额度")).toBeVisible();
  expect(within(dialog).getByText(/保持停用/)).toBeVisible();
  expect(within(dialog).getByRole("button", { name: "确认提交 1 项变更" })).toBeEnabled();
  fireEvent.click(within(dialog).getByRole("button", { name: "确认提交 1 项变更" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ concurrency: 1, waiting_for_capacity: true, schedulable: false }),
    ),
  );
});

it.each([true, false])(
  "直达分组=%s 时预览仅在按钮显示忙碌状态，失败后保留输入",
  async (directGroup) => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    let rejectPreview: (reason: Error) => void = () => {};
    const preview = vi.spyOn(api, "previewOnboardingConcurrency").mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectPreview = reject;
        }),
    );
    client = renderOnboarding(
      {
        ...boundCandidate("active"),
        bound: false,
        bound_accounts: [],
        can_create_key: true,
      },
      directGroup,
    );
    const concurrency = await screen.findByRole("spinbutton", { name: "并发" });
    fireEvent.change(concurrency, { target: { value: "20" } });
    fireEvent.click(await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" }));
    fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
    fireEvent.keyDown(screen.getByRole("combobox", { name: "已有绑定分组 本地分组" }), {
      key: "Escape",
    });
    const previewLabel = directGroup ? "预览添加账号" : "预览 1 项变更";
    fireEvent.click(screen.getByRole("button", { name: previewLabel }));
    const pendingButton = await screen.findByRole("button", { name: "正在预览" });
    expect(pendingButton).toBeDisabled();
    expect(pendingButton).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByText("正在核对并分配共享并发")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "正在提交" })).not.toBeInTheDocument();
    expect(preview.mock.calls[0]?.[0][0]?.concurrency).toBe(20);
    await act(async () => rejectPreview(new Error("手动并发合计超过上游剩余额度 10")));
    expect(screen.queryByRole("dialog", { name: "确认账号绑定变更" })).not.toBeInTheDocument();
    expect(concurrency).toHaveValue(20);
    await waitFor(() => expect(screen.getByRole("button", { name: previewLabel })).toBeEnabled());
    expect(screen.getByRole("button", { name: previewLabel })).not.toHaveAttribute(
      "aria-busy",
      "true",
    );
  },
);
