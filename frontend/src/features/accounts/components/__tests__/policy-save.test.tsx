import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, type AccountDetail } from "@/api";
import { account, task } from "../../__tests__/fixtures";
import { AccountSettingsPanel } from "../account-settings-panel";

let client: QueryClient;
const setting = {
  revision: "v1",
  target_id: "41",
  upstream_id: "upstream-1",
  override: null,
  selected: false,
  effective: false,
  global_enabled: true,
  source: "policy",
} as const;
const detail: AccountDetail = {
  ...account,
  upstream_type: "sub2api",
  ignore_cost_wall: false,
  metadata: {},
  group_rates: {},
  group_ids: {},
  bindings: [],
  test_models: [],
};
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue(setting);
  vi.spyOn(api, "setUpstreamAllocationSetting").mockResolvedValue({
    ...setting,
    override: true,
    selected: true,
    effective: true,
  });
  vi.spyOn(api, "setAccountIgnoreCostWall").mockResolvedValue({ ignore_cost_wall: true });
  vi.spyOn(api, "saveAccountSettings").mockResolvedValue(task("saved"));
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(): { onSaved: ReturnType<typeof vi.fn>; onCancel: ReturnType<typeof vi.fn> } {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const onSaved = vi.fn();
  const onCancel = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <AccountSettingsPanel
        accountId="41"
        query={{ data: detail, isLoading: false, isError: false, error: null }}
        onSaved={onSaved}
        onCancel={onCancel}
      />
    </QueryClientProvider>,
  );
  return { onSaved, onCancel };
}
it("切换独立策略后取消，不写入账号、成本墙或共享并发设置", async () => {
  const state = mount();
  await userEvent.click(await screen.findByRole("switch", { name: "上游共享并发分配" }));
  await userEvent.click(screen.getByRole("switch", { name: "无视成本墙" }));
  expect(screen.getByRole("switch", { name: "上游共享并发分配" })).toBeChecked();
  expect(screen.getByRole("switch", { name: "无视成本墙" })).toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(state.onCancel).toHaveBeenCalled();
  expect(api.setUpstreamAllocationSetting).not.toHaveBeenCalled();
  expect(api.setAccountIgnoreCostWall).not.toHaveBeenCalled();
  expect(api.saveAccountSettings).not.toHaveBeenCalled();
});
it("切换独立策略后点击保存，提交当前草稿并在全部成功后结束编辑", async () => {
  const state = mount();
  await userEvent.click(await screen.findByRole("switch", { name: "上游共享并发分配" }));
  await userEvent.click(screen.getByRole("switch", { name: "无视成本墙" }));
  expect(api.setUpstreamAllocationSetting).not.toHaveBeenCalled();
  expect(api.setAccountIgnoreCostWall).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
  expect(api.setUpstreamAllocationSetting).toHaveBeenCalledWith("accounts", "41", {
    override: true,
    expected_revision: "v1",
    expected_upstream_id: "upstream-1",
  });
  expect(api.setAccountIgnoreCostWall).toHaveBeenCalledWith("41", true);
  expect(api.saveAccountSettings).toHaveBeenCalled();
});
it("共享并发保存失败时保留草稿，不关闭编辑且可重试", async () => {
  vi.mocked(api.setUpstreamAllocationSetting).mockRejectedValueOnce(
    new Error("版本已变化，请刷新后重试"),
  );
  const notify = vi.spyOn(toast, "error").mockReturnValue("error");
  const state = mount();
  await userEvent.click(await screen.findByRole("switch", { name: "上游共享并发分配" }));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(notify).toHaveBeenCalled());
  expect(state.onSaved).not.toHaveBeenCalled();
  expect(screen.getByRole("switch", { name: "上游共享并发分配" })).toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
});
it("未更改策略时保存基础设置，不写入独立覆盖", async () => {
  const state = mount();
  await screen.findByRole("switch", { name: "上游共享并发分配" });
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
  expect(api.setUpstreamAllocationSetting).not.toHaveBeenCalled();
  expect(api.setAccountIgnoreCostWall).not.toHaveBeenCalled();
});

it("成本墙保存失败时保留选择，重试成功后才执行账号设置", async () => {
  vi.mocked(api.setAccountIgnoreCostWall).mockRejectedValueOnce(
    new Error("成本墙写入失败，请重试"),
  );
  const notify = vi.spyOn(toast, "error").mockReturnValue("error");
  const state = mount();
  await screen.findByRole("switch", { name: "上游共享并发分配" });
  await userEvent.click(screen.getByRole("switch", { name: "无视成本墙" }));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(notify).toHaveBeenCalled());
  expect(state.onSaved).not.toHaveBeenCalled();
  expect(api.saveAccountSettings).not.toHaveBeenCalled();
  expect(screen.getByRole("switch", { name: "无视成本墙" })).toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
});
