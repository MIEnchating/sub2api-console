import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchOAuthCheckpoints } from "../components/workbench-oauth-checkpoints";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("检查点批量安全成功后发起新授权，关闭来源弹窗并交给上层授权页面", async () => {
  const expires = new Date(Date.now() + 600000).toISOString();
  const checkpoint = {
    id: "paused",
    source_task_id: "old-oauth",
    scope: "local-export",
    revision: 2,
    checkpoint_revision: 1,
    status: "ready",
    can_restore: true,
    expires_at: expires,
  };
  const row = {
    index: 0,
    source: { checkpoint_id: "paused", revision: 2, checkpoint_revision: 1 },
    security_id: "finished",
    email: "verified@example.test",
    user_id: "official-user",
    status: "succeeded",
    message: "安全设置完成",
  };
  const batch = {
    id: "safe-batch",
    task_id: "safe-batch",
    scope: "local-export",
    operation: "totp",
    status: "succeeded",
    message: "本批已结束",
    completed: 1,
    succeeded: 1,
    expires_at: expires,
    items: [row],
  };
  const oauth = {
    id: "new-oauth",
    task_id: "new-oauth",
    scope: "local-export",
    status: "waiting",
    message: "新授权等待登录",
    expires_at: expires,
    width: 1100,
    height: 760,
  };
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal(
    "fetch",
    vi.fn<typeof fetch>(async (input, init) => {
      const path = String(input);
      if (init?.method === "DELETE") return Response.json({ cancelled: true });
      if (path.endsWith("/security-batches/preview"))
        return Response.json({
          id: "preview",
          scope: "local-export",
          operation: "totp",
          target: "",
          expires_at: expires,
          items: [row],
          errors: [],
        });
      if (path.endsWith("/security-batches") || path.endsWith("/security-batches/safe-batch"))
        return Response.json(batch);
      if (path.endsWith("/security/finished/oauth")) return Response.json(oauth);
      if (path.startsWith("/api/account-workbench/oauth-checkpoints"))
        return Response.json([checkpoint]);
      throw new Error(`未预期请求 ${path}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const onOAuth = vi.fn();
  const view = render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuthCheckpoints
        scope="local-export"
        disabled={false}
        onRestore={() => undefined}
        onOAuth={onOAuth}
      />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "已暂停的授权" }));
  await user.click(await screen.findByRole("button", { name: "批量设置检查点账号安全" }));
  await user.click(await screen.findByRole("checkbox", { name: "来源任务：old-oauth" }));
  await user.click(screen.getByRole("button", { name: "预览批量安全操作" }));
  await user.click(await screen.findByRole("button", { name: "确认并执行批量安全操作" }));
  await user.click(await screen.findByRole("button", { name: "确认发起新的 OAuth 授权" }));
  await waitFor(() => expect(onOAuth).toHaveBeenCalledWith(oauth));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  view.unmount();
  client.clear();
});
