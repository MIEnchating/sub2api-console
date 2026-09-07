import type { QueryClient } from "@tanstack/react-query";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Task } from "@/api";
import { boundCandidate, renderOnboarding } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  client?.clear();
  vi.restoreAllMocks();
});

it("开户页维护任务已创建而首次状态尚未返回时持续展示加载状态", async () => {
  vi.spyOn(api, "revalidateAccounts").mockResolvedValue({
    id: "onboarding-revalidate",
    operation: "account-revalidate",
    skill: "accounts",
    status: "queued",
    progress: 0,
    message: "",
    result: {},
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:00Z",
  });
  const readTask = vi.spyOn(api, "task").mockReturnValue(new Promise<Task>(() => {}));
  client = renderOnboarding(boundCandidate("active"));
  await screen.findByRole("button", { name: "预览更新绑定" });
  fireEvent.click(screen.getByRole("button", { name: "复验绑定" }));
  await waitFor(() => expect(readTask).toHaveBeenCalledWith("onboarding-revalidate"));
  expect(within(screen.getByRole("dialog")).getByRole("status")).toBeVisible();
});
