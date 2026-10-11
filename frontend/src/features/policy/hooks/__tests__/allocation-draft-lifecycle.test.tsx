import { QueryClient, QueryClientProvider, useMutation } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, type UpstreamAllocationSetting } from "@/api";
import { useUpstreamAllocationDraft } from "../use-upstream-allocation-draft";
import { UpstreamAllocationSetting as Control } from "../../components/upstream-allocation-setting";

let client: QueryClient;
const setting: UpstreamAllocationSetting = {
  revision: "v1",
  target_id: "41",
  upstream_id: "up-1",
  override: null,
  selected: false,
  effective: false,
  global_enabled: true,
  source: "policy",
};
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.spyOn(api, "upstreamAllocationSetting").mockImplementation(async (_kind, id) => ({
    ...setting,
    target_id: id,
  }));
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function Editor(props: { id: string }) {
  const draft = useUpstreamAllocationDraft("accounts", props.id, true);
  const save = useMutation({ mutationFn: draft.save });
  return (
    <>
      <Control kind="accounts" id={props.id} draft={draft} />
      <button disabled={!draft.ready} onClick={() => save.mutate()}>
        保存
      </button>
    </>
  );
}
function panel(id: string) {
  return (
    <QueryClientProvider client={client}>
      <Editor id={id} />
    </QueryClientProvider>
  );
}
it("账号A迟到保存不清空账号B的未提交选择", async () => {
  let finish!: (value: UpstreamAllocationSetting) => void;
  vi.spyOn(api, "setUpstreamAllocationSetting").mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const view = render(panel("41"));
  await userEvent.click(await screen.findByRole("switch"));
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(finish).toBeDefined());
  view.rerender(panel("42"));
  await userEvent.click(await screen.findByRole("switch"));
  await act(async () => finish({ ...setting, effective: true, override: true, revision: "v2" }));
  expect(screen.getByRole("switch")).toBeChecked();
});
it("编辑后后台读取新版本，提交仍使用开始编辑时的版本", async () => {
  const save = vi
    .spyOn(api, "setUpstreamAllocationSetting")
    .mockResolvedValue({ ...setting, effective: true });
  render(panel("41"));
  await userEvent.click(await screen.findByRole("switch"));
  await act(async () => {
    client.setQueryData(["upstream-allocation", "accounts", "41"], {
      ...setting,
      revision: "external-v2",
    });
  });
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save).toHaveBeenCalledWith(
    "accounts",
    "41",
    expect.objectContaining({ expected_revision: "v1" }),
  );
});

it("保存前取消旧读取，迟到结果不覆盖服务器确认版本", async () => {
  let finishRead!: (value: UpstreamAllocationSetting) => void;
  let signal: AbortSignal | undefined;
  const saved = { ...setting, revision: "saved-v2", effective: true, override: true };
  vi.spyOn(api, "setUpstreamAllocationSetting").mockResolvedValue(saved);
  render(panel("41"));
  await userEvent.click(await screen.findByRole("switch"));
  vi.mocked(api.upstreamAllocationSetting).mockImplementation((_kind, _id, incoming) => {
    signal = incoming;
    return new Promise((resolve) => {
      finishRead = resolve;
    });
  });
  let refresh!: Promise<void>;
  await act(async () => {
    refresh = client.invalidateQueries({ queryKey: ["upstream-allocation", "accounts", "41"] });
  });
  await waitFor(() => expect(finishRead).toBeDefined());
  await userEvent.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(client.getQueryData(["upstream-allocation", "accounts", "41"])).toEqual(saved),
  );
  expect(signal?.aborted).toBe(true);
  await act(async () => {
    finishRead(setting);
    await refresh;
  });
  expect(screen.getByRole("switch")).toBeChecked();
  expect(client.getQueryData(["upstream-allocation", "accounts", "41"])).toEqual(saved);
});
