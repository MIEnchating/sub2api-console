import type { QueryClient } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type OnboardingContext } from "@/api";
import { boundCandidate, renderOnboarding, upstream } from "./onboarding-fixture";

let client: QueryClient | undefined;
afterEach(() => {
  cleanup();
  client?.clear();
  vi.restoreAllMocks();
});

it("直达分组编辑草稿后刷新返回相同数据，保留所选本地分组和可用状态", async () => {
  const candidate = boundCandidate("active");
  client = renderOnboarding(candidate);
  const selector = await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" });
  await waitFor(() => expect(selector).not.toHaveAttribute("aria-disabled", "true"));
  fireEvent.click(selector);
  fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
  fireEvent.keyDown(selector, { key: "Escape" });
  expect(selector).toHaveTextContent("备用分组");

  let finishRefresh!: (context: OnboardingContext) => void;
  vi.mocked(api.prepareOnboarding).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishRefresh = resolve;
      }),
  );
  let refresh: Promise<void> | undefined;
  act(() => {
    refresh = client!.refetchQueries({ queryKey: ["onboarding-preparation"] });
  });
  await waitFor(() => expect(screen.getByRole("button", { name: "同步余额" })).toBeDisabled());
  await act(async () => {
    finishRefresh({ upstream: { ...upstream }, candidates: [{ ...candidate }] });
    await refresh;
  });

  await waitFor(() => expect(screen.getByRole("button", { name: "同步余额" })).toBeEnabled());
  expect(selector).not.toHaveAttribute("aria-disabled", "true");
  expect(selector).toHaveTextContent("备用分组");
  expect(screen.getByRole("button", { name: "预览更新绑定" })).toBeEnabled();
});
