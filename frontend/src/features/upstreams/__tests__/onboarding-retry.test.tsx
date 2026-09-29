import type { QueryClient } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Task } from "@/api";
import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function makeTask(id: string, status: Task["status"]): Task {
  let message = "完成";
  let result: Record<string, unknown> = {};
  if (status === "failed") message = "开户失败";
  else if (status === "cancelled") {
    message = "开户已取消";
    result = { cancelled: true };
  }
  if (status === "failed") result = { error: "续办参数与首次冻结的开户意图不一致，已拒绝远端写入" };
  return {
    id,
    skill: "onboarding",
    operation: "onboard",
    status,
    progress: 100,
    message,
    result,
    created_at: "2026-09-28T00:00:00Z",
    updated_at: "2026-09-28T00:00:00Z",
  };
}

it.each(["failed", "cancelled"] as const)(
  "首次开户任务%s后按原参数重试时复用完整请求",
  async (status) => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    let completeRetry!: (value: Task) => void;
    const retryResponse = new Promise<Task>((resolve) => {
      completeRetry = resolve;
    });
    const submit = vi
      .spyOn(api, "onboard")
      .mockResolvedValueOnce(makeTask("first", "running"))
      .mockReturnValueOnce(retryResponse);
    const preview = vi.spyOn(api, "previewOnboardingConcurrency").mockResolvedValue({
      items: [{ concurrency: 7 }],
    });
    vi.spyOn(api, "task").mockImplementation(async (id) =>
      makeTask(id, id === "first" ? status : "succeeded"),
    );
    const candidate = {
      ...boundCandidate("active"),
      bound: false,
      bound_accounts: [],
      can_create_key: true,
      can_bind_existing_key: false,
      key_present: false,
      upstream_key_id: null,
      upstream_key_name: null,
    };
    client = renderOnboarding({
      ...candidate,
    });
    const groups = await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" });
    fireEvent.click(groups);
    fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
    fireEvent.keyDown(groups, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "预览添加账号" }));
    const confirmation = within(await screen.findByRole("dialog", { name: "确认账号绑定变更" }));
    fireEvent.click(confirmation.getByRole("button", { name: "确认提交 1 项变更" }));
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
    const firstRequest = submit.mock.calls[0]?.[0];
    expect(firstRequest).toMatchObject({ concurrency: 7, local_group_ids: [2] });
    const taskDialog = within(await screen.findByRole("dialog", { name: "账号绑定变更" }));
    fireEvent.click(await taskDialog.findByRole("button", { name: "按原参数重试" }));
    await waitFor(() => expect(submit).toHaveBeenCalledTimes(2));
    expect(taskDialog.getByRole("button", { name: "正在重试" })).toBeDisabled();
    expect(taskDialog.getByRole("button", { name: "完成" })).toBeDisabled();
    expect(taskDialog.queryByRole("button", { name: "取消任务" })).not.toBeInTheDocument();
    if (status === "failed") {
      expect(taskDialog.getByText(/取消或失败后，系统会保留首次开户记录/)).toBeInTheDocument();
    }
    expect(submit.mock.calls[1]?.[0]).toEqual(firstRequest);
    expect(preview).toHaveBeenCalledTimes(1);
    await act(async () => completeRetry(makeTask("retry", "running")));
  },
);
