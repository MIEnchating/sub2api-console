import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { StaleBindingCleanup } from "../stale-binding-cleanup";

function renderCleanup(disabled = false, groupId = "codex") {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const onChanged = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <StaleBindingCleanup
        host="api.example.test"
        bindingId={7}
        upstreamId="up_example"
        accountId="142"
        upstreamKeyId="key-142"
        groupId={groupId}
        groupName="Codex 分组"
        disabled={disabled}
        onChanged={onChanged}
      />
    </QueryClientProvider>,
  );
  return onChanged;
}

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("键盘打开清理确认后取消时保留绑定且不发送请求", async () => {
  const user = userEvent.setup();
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  renderCleanup();
  await user.tab();
  expect(screen.getByRole("button", { name: "清理失效绑定" })).toHaveFocus();
  await user.keyboard("{Enter}");
  const dialog = await screen.findByRole("dialog", { name: "清理失效绑定" });
  expect(within(dialog).getByText(/账号 142.*Codex 分组.*绑定 ID 7/)).toBeVisible();
  expect(within(dialog).getByText(/不删除账号、上游 Key 或其他绑定/)).toBeVisible();
  await user.click(within(dialog).getByRole("button", { name: "取消" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(fetch).not.toHaveBeenCalled();
});

it("确认清理时提交稳定绑定范围并在请求期间禁用重复提交，成功后刷新列表", async () => {
  const user = userEvent.setup();
  let resolve!: (response: Response) => void;
  const response = new Promise<Response>((done) => {
    resolve = done;
  });
  const fetch = vi.fn().mockReturnValue(response);
  vi.stubGlobal("fetch", fetch);
  const onChanged = renderCleanup();
  await user.click(screen.getByRole("button", { name: "清理失效绑定" }));
  await user.click(await screen.findByRole("button", { name: "确认清理" }));
  expect(screen.getByRole("button", { name: "正在清理…" })).toBeDisabled();
  expect(fetch).toHaveBeenCalledWith(
    "/api/upstreams/api.example.test/bindings/7/cleanup",
    expect.objectContaining({
      method: "POST",
      credentials: "include",
      body: JSON.stringify({
        upstream_id: "up_example",
        account_id: "142",
        upstream_key_id: "key-142",
        upstream_group_id: "codex",
      }),
    }),
  );
  await act(async () => resolve(new Response(JSON.stringify({ binding_id: 7 }), { status: 200 })));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(onChanged).toHaveBeenCalledOnce();
  expect(fetch).toHaveBeenCalledOnce();
});

it("账号恢复导致清理失败时显示具体原因并保留取消入口", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ detail: "账号已恢复，请刷新列表后重新确认" }), {
        status: 409,
      }),
    ),
  );
  const errorToast = vi.spyOn(toast, "error");
  const onChanged = renderCleanup();
  await user.click(screen.getByRole("button", { name: "清理失效绑定" }));
  await user.click(await screen.findByRole("button", { name: "确认清理" }));
  await waitFor(() =>
    expect(errorToast).toHaveBeenCalledWith(
      expect.stringContaining("账号已恢复"),
      expect.any(Object),
    ),
  );
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "确认清理" })).toBeEnabled();
  expect(onChanged).not.toHaveBeenCalled();
});

it("账号操作进行中时不能打开清理确认", () => {
  renderCleanup(true);
  expect(screen.getByRole("button", { name: "清理失效绑定" })).toBeDisabled();
});

it("缺少稳定分组 ID 时禁止清理并提示先同步上游", () => {
  renderCleanup(false, "");
  expect(
    screen.getByRole("button", { name: "清理失效绑定：请先同步上游确认绑定身份" }),
  ).toBeDisabled();
});
