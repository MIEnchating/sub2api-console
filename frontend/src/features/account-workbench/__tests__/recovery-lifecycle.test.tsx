import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactElement } from "react";
import { WorkbenchOAuth } from "../components/workbench-oauth";
import { WorkbenchOAuthBatch } from "../components/workbench-oauth-batch";
import { WorkbenchQueueRecovery } from "../components/workbench-queue-recovery";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
function mount(element: ReactElement, fetcher: typeof fetch): ReturnType<typeof render> {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}
function waiting() {
  return {
    id: "saved-login",
    task_id: "saved-login",
    scope: "managed",
    recovery_enabled: true,
    checkpoint_id: "checkpoint",
    status: "waiting",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    host: "auth.openai.com",
    width: 1100,
    height: 760,
    message: "等待验证",
  };
}

it("明确开启自动检查点后离开页面保留授权，重新接回后主动结束仍删除", async () => {
  const session = waiting();
  const fetcher = vi.fn<typeof fetch>(async () => Response.json(session));
  const first = mount(<WorkbenchOAuth />, fetcher);
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("checkbox", { name: "自动保存私有登录检查点（按原授权到期时间清除）" }),
  );
  await user.click(screen.getByRole("button", { name: "开始授权登录" }));
  await screen.findByRole("button", { name: "结束授权" });
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth",
      expect.objectContaining({ body: JSON.stringify({ recovery_enabled: true }) }),
    ),
  );
  first.unmount();
  expect(fetcher.mock.calls.some(([, input]) => input?.method === "DELETE")).toBe(false);
  fetcher.mockImplementation(async (url) =>
    Response.json(
      String(url).endsWith("oauth-checkpoints")
        ? [
            {
              id: "checkpoint",
              source_task_id: "saved-login",
              status: "watching",
              revision: 3,
              checkpoint_revision: 4,
              automatic: true,
              active: true,
              expires_at: session.expires_at,
              can_restore: true,
            },
          ]
        : session,
    ),
  );
  mount(<WorkbenchOAuth />, fetcher);
  await user.click(screen.getByRole("button", { name: "已暂停的授权" }));
  await user.click(await screen.findByRole("button", { name: "恢复授权" }));
  await user.click(screen.getByRole("button", { name: "确认人工恢复" }));
  await screen.findByRole("button", { name: "结束授权" });
  await user.click(screen.getByRole("button", { name: "结束授权" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth/saved-login",
      expect.objectContaining({ method: "DELETE" }),
    ),
  );
});

it("已开启批量恢复时离开页面不删除批次，明确结束必须删除", async () => {
  const queue = {
    id: "queue",
    kind: "oauth-batch",
    task_id: "saved-batch",
    status: "running",
    active: true,
    revision: 2,
    expires_at: new Date(Date.now() + 600000).toISOString(),
    can_resume: true,
  };
  const batch = {
    id: "saved-batch",
    task_id: "saved-batch",
    status: "running",
    recovery_enabled: true,
    expires_at: queue.expires_at,
    items: [],
    available: 0,
    message: "等待账号授权",
  };
  const fetcher = vi.fn<typeof fetch>(async (url) =>
    Response.json(String(url).endsWith("queue-recoveries") ? [queue] : batch),
  );
  const first = mount(<WorkbenchOAuthBatch />, fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  await user.click(await screen.findByRole("button", { name: "恢复本批" }));
  await user.click(screen.getByRole("button", { name: "确认继续本批" }));
  await screen.findByRole("region", { name: "批量授权进度" });
  first.unmount();
  expect(fetcher.mock.calls.some(([, input]) => input?.method === "DELETE")).toBe(false);
  mount(<WorkbenchOAuthBatch />, fetcher);
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  await user.click(await screen.findByRole("button", { name: "恢复本批" }));
  await user.click(screen.getByRole("button", { name: "确认继续本批" }));
  await user.click(await screen.findByRole("button", { name: "结束批量授权" }));
  await user.click(screen.getByRole("button", { name: "结束并清除本批" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth-batches/saved-batch",
      expect.objectContaining({ method: "DELETE" }),
    ),
  );
});

it("恢复确认期间到期会禁用确认并保留取消入口", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const onResume = vi.fn();
  const queue = {
    id: "expiring",
    kind: "oauth-batch",
    task_id: "saved-batch",
    status: "interrupted",
    revision: 2,
    expires_at: new Date(Date.now() + 60000).toISOString(),
    can_resume: true,
  };
  mount(
    <WorkbenchQueueRecovery kind="oauth-batch" disabled={false} onResume={onResume} />,
    async () => Response.json([queue]),
  );
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  await user.click(await screen.findByRole("button", { name: "恢复本批" }));
  expect(screen.getByRole("button", { name: "确认继续本批" })).toBeEnabled();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(61000);
  });
  expect(screen.getByRole("button", { name: "确认继续本批" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
  expect(onResume).not.toHaveBeenCalled();
});

it("启动自动保存授权期间主动结束再离开页面仍取消迟到会话", async () => {
  let resolveStart: ((response: Response) => void) | undefined;
  const pending = new Promise<Response>((resolve) => {
    resolveStart = resolve;
  });
  const fetcher = vi.fn<typeof fetch>(async (_url, options) =>
    options?.method === "POST" ? pending : Response.json({ cancelled: true }),
  );
  const view = mount(<WorkbenchOAuth />, fetcher);
  const user = userEvent.setup();
  await user.click(
    screen.getByRole("checkbox", { name: "自动保存私有登录检查点（按原授权到期时间清除）" }),
  );
  await user.click(screen.getByRole("button", { name: "开始授权登录" }));
  await user.click(await screen.findByRole("button", { name: "结束授权" }));
  view.unmount();
  await act(async () => {
    resolveStart?.(Response.json(waiting()));
  });
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/oauth/saved-login",
      expect.objectContaining({ method: "DELETE" }),
    ),
  );
});
