import { QueryClient, QueryClientProvider, useMutation } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { api, type UpstreamAllocationSetting as Setting } from "@/api";
import { useUpstreamAllocationDraft } from "../../hooks/use-upstream-allocation-draft";
import { UpstreamAllocationSetting } from "../upstream-allocation-setting";

const inherited: Setting = {
  revision: "v1",
  target_id: "147",
  upstream_id: "up_fixture",
  override: null,
  selected: false,
  effective: false,
  global_enabled: true,
  source: "policy",
};
let client: QueryClient;
function Editor(props: { open: boolean }) {
  const draft = useUpstreamAllocationDraft("accounts", "147", props.open);
  const save = useMutation({ mutationFn: draft.save });
  if (!props.open) return null;
  return (
    <>
      <UpstreamAllocationSetting kind="accounts" id="147" draft={draft} disabled={save.isPending} />
      <button disabled={!draft.ready || save.isPending} onClick={() => save.mutate()}>
        保存
      </button>
    </>
  );
}
function mount() {
  vi.stubGlobal("PointerEvent", MouseEvent);
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Editor open />
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it("读取期间禁止保存，读取完成后仅用关闭的开关表示未纳入策略", async () => {
  let finish!: (value: Setting) => void;
  vi.spyOn(api, "upstreamAllocationSetting").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  mount();
  expect(screen.getByText("正在读取共享并发设置")).toBeVisible();
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  finish(inherited);
  expect(await screen.findByRole("switch", { name: "上游共享并发分配" })).not.toBeChecked();
  expect(screen.queryByText("未纳入分配范围")).not.toBeInTheDocument();
  expect(screen.queryByText("跟随调度策略范围")).not.toBeInTheDocument();
});
it("策略已包含账号时直接显示开启，不出现来源说明和恢复跟随按钮", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue({
    ...inherited,
    selected: true,
    effective: true,
  });
  mount();
  expect(await screen.findByRole("switch")).toBeChecked();
  expect(screen.queryByText("跟随调度策略范围")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "恢复跟随" })).not.toBeInTheDocument();
});
it("键盘切换只改变草稿，点击保存才提交并失效策略缓存", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue(inherited);
  const save = vi
    .spyOn(api, "setUpstreamAllocationSetting")
    .mockResolvedValue({ ...inherited, selected: true, effective: true, override: true });
  mount();
  client.setQueryData(["policy"], { revision: "old" });
  const control = await screen.findByRole("switch");
  control.focus();
  await userEvent.keyboard(" ");
  expect(control).toBeChecked();
  expect(save).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(save).toHaveBeenCalledWith("accounts", "147", {
      override: true,
      expected_revision: "v1",
      expected_upstream_id: "up_fixture",
    }),
  );
  expect(client.getQueryState(["policy"])?.isInvalidated).toBe(true);
});
it("总开关关闭时显示关闭并禁用，不将已选范围误显示为生效", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue({
    ...inherited,
    override: true,
    selected: true,
    global_enabled: false,
  });
  mount();
  const control = await screen.findByRole("switch");
  expect(control).not.toBeChecked();
  expect(control).toHaveAttribute("aria-disabled", "true");
});
it("读取失败时禁止保存，重新读取成功后恢复编辑", async () => {
  vi.spyOn(api, "upstreamAllocationSetting")
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValue(inherited);
  mount();
  const retry = await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  await userEvent.click(retry);
  await screen.findByRole("switch");
  expect(screen.getByRole("button", { name: "保存" })).toBeEnabled();
});
it("后台刷新保留未保存选择，关闭再打开时丢弃草稿", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue(inherited);
  const view = mount();
  await userEvent.click(await screen.findByRole("switch"));
  await act(async () => {
    client.setQueryData(["upstream-allocation", "accounts", "147"], {
      ...inherited,
      revision: "v2",
    });
  });
  expect(screen.getByRole("switch")).toBeChecked();
  view.rerender(
    <QueryClientProvider client={client}>
      <Editor open={false} />
    </QueryClientProvider>,
  );
  view.rerender(
    <QueryClientProvider client={client}>
      <Editor open />
    </QueryClientProvider>,
  );
  expect(await screen.findByRole("switch")).not.toBeChecked();
});
it("切换后恢复原值再保存，不生成独立覆盖", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue(inherited);
  const save = vi.spyOn(api, "setUpstreamAllocationSetting");
  mount();
  const control = await screen.findByRole("switch");
  await userEvent.click(control);
  await userEvent.click(control);
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存" })).toBeEnabled());
  expect(save).not.toHaveBeenCalled();
});

it("保存请求未完成时禁用开关和保存按钮，成功后恢复编辑", async () => {
  vi.spyOn(api, "upstreamAllocationSetting").mockResolvedValue(inherited);
  let finish!: (value: Setting) => void;
  vi.spyOn(api, "setUpstreamAllocationSetting").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  mount();
  const control = await screen.findByRole("switch");
  await userEvent.click(control);
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  expect(control).toHaveAttribute("aria-disabled", "true");
  expect(screen.getByRole("button", { name: "保存" })).toBeDisabled();
  finish({ ...inherited, effective: true, selected: true, override: true });
  await waitFor(() => expect(control).not.toHaveAttribute("aria-disabled", "true"));
  expect(control).toBeChecked();
  expect(screen.getByRole("button", { name: "保存" })).toBeEnabled();
});
