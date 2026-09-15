import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type AnimationSchedule } from "@/api";
import { createConsoleQueryClient } from "@/lib/query-client";
import { AnimationCheckPanel } from "../animation-check-panel";
import { AnimationScheduleDialog } from "../animation-schedule-dialog";

const schedule: AnimationSchedule = {
  account_id: "41",
  enabled: false,
  model: "saved-model",
  interval_minutes: 60,
  timeout_seconds: 120,
  version: 1,
};
const clients: ReturnType<typeof createConsoleQueryClient>[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function client() {
  const value = createConsoleQueryClient();
  clients.push(value);
  value.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  return value;
}

it("自动检测配置后台版本更新时保留正在编辑的草稿", async () => {
  const queryClient = client();
  queryClient.setQueryData(
    ["accounts"],
    [{ id: "41", name: "测试账号", groups: [], platform: "openai" }],
  );
  queryClient.setQueryData(["model-animation", "schedules"], [schedule]);
  queryClient.setQueryData(["model-animation", "history"], []);
  render(
    <QueryClientProvider client={queryClient}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "自动检测设置" }));
  const model = screen.getByRole("textbox", { name: "检测模型" });
  fireEvent.change(model, { target: { value: "draft-model" } });
  await act(async () =>
    queryClient.setQueryData(
      ["model-animation", "schedules"],
      [{ ...schedule, enabled: true, version: 2, model: "remote-model", interval_minutes: 30 }],
    ),
  );
  await screen.findByText("每 30 分钟自动检测");
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("draft-model");
});

it("关闭自动检测的保存请求等待时不能关闭编辑，失败后恢复并保留输入", async () => {
  const queryClient = client();
  let rejectSave!: (error: Error) => void;
  vi.spyOn(api, "saveAnimationSchedule").mockReturnValue(
    new Promise((_resolve, reject) => {
      rejectSave = reject;
    }),
  );
  const close = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <AnimationScheduleDialog
        accountID="41"
        accountName="测试账号"
        model=""
        schedule={schedule}
        onClose={close}
      />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByRole("textbox", { name: "检测模型" }), {
    target: { value: "draft-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存设置" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "正在保存…" })).toBeDisabled());
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  expect(close).not.toHaveBeenCalled();
  await act(async () => rejectSave(new Error("保存失败")));
  await waitFor(() => expect(screen.getByRole("button", { name: "取消" })).toBeEnabled());
  expect(screen.getByRole("textbox", { name: "检测模型" })).toHaveValue("draft-model");
});
