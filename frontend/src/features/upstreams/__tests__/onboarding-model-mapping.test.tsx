import type { QueryClient } from "@tanstack/react-query";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
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

it.each([true, false])(
  "直达分组=%s 时将模型映射传到创建请求，提交失败后保留映射",
  async (directGroup) => {
    vi.stubGlobal("PointerEvent", MouseEvent);
    const submit = vi
      .spyOn(api, directGroup ? "onboard" : "onboardBatch")
      .mockRejectedValue(new Error("isolated submission failure"));
    vi.spyOn(api, "previewOnboardingConcurrency").mockResolvedValue({
      items: [{ concurrency: 1 }],
    });
    client = renderOnboarding(
      { ...boundCandidate("active"), bound: false, bound_accounts: [], can_create_key: true },
      directGroup,
    );
    client.setQueryData(["onboarding-model-options", "api.example.test", "7"], ["upstream"]);
    const groups = await screen.findByRole("combobox", { name: "已有绑定分组 本地分组" });
    fireEvent.click(groups);
    fireEvent.click(await screen.findByRole("option", { name: /备用分组/ }));
    fireEvent.keyDown(groups, { key: "Escape" });
    fireEvent.click(
      screen.getByRole("button", { name: directGroup ? "预览添加账号" : "预览 1 项变更" }),
    );
    const dialog = within(await screen.findByRole("dialog", { name: "确认账号绑定变更" }));
    fireEvent.click(dialog.getByRole("button", { name: "添加模型映射" }));
    fireEvent.change(dialog.getByRole("textbox", { name: "请求模型" }), {
      target: { value: "alias" },
    });
    fireEvent.click(dialog.getByRole("combobox", { name: "上游模型" }));
    fireEvent.click(await screen.findByRole("option", { name: "upstream" }));
    fireEvent.click(dialog.getByRole("button", { name: "确认提交 1 项变更" }));
    const expected = expect.objectContaining({ model_mapping: { alias: "upstream" } });
    await waitFor(() => expect(submit).toHaveBeenCalledWith(directGroup ? expected : [expected]));
    await waitFor(() =>
      expect(dialog.getByRole("button", { name: "确认提交 1 项变更" })).toBeEnabled(),
    );
    expect(dialog.getByRole("textbox", { name: "请求模型" })).toHaveValue("alias");
    expect(dialog.getByRole("combobox", { name: "上游模型" })).toHaveTextContent("upstream");
  },
);
