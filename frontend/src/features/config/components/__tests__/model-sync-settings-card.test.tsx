import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "@/api";
import { ModelSyncSettingsCard } from "../model-sync-settings-card";

afterEach(() => vi.restoreAllMocks());

function renderCard(patterns: string[]) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { enabled: false, retry: false }, mutations: { retry: false } },
  });
  queryClient.setQueryData(["account-model-sync-settings"], { blocked_patterns: patterns });
  return render(
    <QueryClientProvider client={queryClient}>
      <ModelSyncSettingsCard />
    </QueryClientProvider>,
  );
}

describe("全局屏蔽模型设置", () => {
  it("按行编辑通配符规则并提交去重后的规则数组", async () => {
    const user = userEvent.setup();
    const update = vi
      .spyOn(api, "updateAccountModelSyncSettings")
      .mockImplementation(async (payload) => payload);
    renderCard(["claude-*"]);

    const input = screen.getByLabelText("屏蔽规则");
    expect(input).toHaveValue("claude-*");
    expect(input).toHaveClass("field-sizing-fixed", "flex-1", "max-h-none", "resize-none");
    await user.clear(input);
    await user.type(input, "gemini-*\n*-image-*\nGEMINI-*");
    await user.click(screen.getByRole("button", { name: "保存屏蔽规则" }));

    await waitFor(() =>
      expect(update).toHaveBeenCalledWith({ blocked_patterns: ["*-image-*", "GEMINI-*"] }),
    );
  });

  it("超过规则上限时标记输入无效且不提交", async () => {
    const user = userEvent.setup();
    const update = vi.spyOn(api, "updateAccountModelSyncSettings");
    renderCard([]);

    const input = screen.getByLabelText("屏蔽规则");
    await user.click(input);
    await user.paste(Array.from({ length: 201 }, (_, index) => `model-${index}`).join("\n"));
    await user.click(screen.getByRole("button", { name: "保存屏蔽规则" }));

    expect(await screen.findByText("最多配置 200 条屏蔽规则")).toBeVisible();
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(update).not.toHaveBeenCalled();
  });
});
