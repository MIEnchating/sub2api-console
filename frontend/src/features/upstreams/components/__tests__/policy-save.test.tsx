import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { api, type UpstreamConfiguration } from "@/api";
import { UpstreamEditDialog } from "../upstream-edit-dialog";

let client: QueryClient;
const configuration: UpstreamConfiguration = {
  upstream_id: "up_example",
  host: "api.example.test",
  name: "测试上游",
  base_url: "https://api.example.test",
  account_base_url: "https://api.example.test/v1",
  upstream_type: "sub2api",
  auth_mode: "sub2api_user_token",
  recharge_rate: "1",
  raw_balance: "24",
  balance: "24",
  has_access_token: true,
  has_refresh_token: false,
  has_admin_key: false,
  has_user_id: false,
  headers: {},
  header_names: [],
  cookie_names: [],
  groups: [],
};
const setting = {
  revision: "v1",
  target_id: "up_example",
  upstream_id: "up_example",
  override: null,
  selected: true,
  effective: true,
  global_enabled: true,
  source: "policy",
} as const;
beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue(setting);
  vi.spyOn(api, "setUpstreamAllocationSetting").mockResolvedValue({
    ...setting,
    override: false,
    selected: false,
    effective: false,
  });
  vi.spyOn(api, "updateUpstreamConfiguration").mockResolvedValue(configuration);
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(): { onSaved: ReturnType<typeof vi.fn>; onOpenChange: ReturnType<typeof vi.fn> } {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(["upstream-configuration", configuration.host], configuration);
  client.setQueryData(["auth-recovery-config"], { vault_entries: [] });
  client.setQueryData(["dictionaries", "upstream_type"], { items: [] });
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <UpstreamEditDialog host={configuration.host} onSaved={onSaved} onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  );
  return { onSaved, onOpenChange };
}
it("策略已包含上游时显示开启，切换后取消不写入配置", async () => {
  const state = mount();
  const toggle = await screen.findByRole("switch", { name: "上游共享并发分配" });
  expect(toggle).toBeChecked();
  expect(screen.queryByText("跟随调度策略范围")).not.toBeInTheDocument();
  await userEvent.click(toggle);
  expect(toggle).not.toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "取消" }));
  expect(state.onOpenChange).toHaveBeenCalledWith(false);
  expect(api.setUpstreamAllocationSetting).not.toHaveBeenCalled();
  expect(api.updateUpstreamConfiguration).not.toHaveBeenCalled();
});
it("切换共享并发后点击保存并重算，才提交稳定上游 ID 的设置", async () => {
  const state = mount();
  await userEvent.click(await screen.findByRole("switch", { name: "上游共享并发分配" }));
  expect(api.setUpstreamAllocationSetting).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
  expect(api.setUpstreamAllocationSetting).toHaveBeenCalledWith("upstreams", "up_example", {
    override: false,
    expected_revision: "v1",
    expected_upstream_id: "up_example",
  });
  expect(api.updateUpstreamConfiguration).toHaveBeenCalled();
});
it("共享并发保存失败时保留上游编辑草稿，重试成功才提示完成", async () => {
  vi.mocked(api.setUpstreamAllocationSetting).mockRejectedValueOnce(new Error("版本冲突，请重试"));
  const notify = vi.spyOn(toast, "error").mockReturnValue("error");
  const state = mount();
  await userEvent.click(await screen.findByRole("switch", { name: "上游共享并发分配" }));
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(notify).toHaveBeenCalled());
  expect(state.onSaved).not.toHaveBeenCalled();
  expect(screen.getByRole("switch", { name: "上游共享并发分配" })).not.toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "保存并重算" }));
  await waitFor(() => expect(state.onSaved).toHaveBeenCalled());
});
