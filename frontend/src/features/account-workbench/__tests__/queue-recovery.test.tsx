import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { WorkbenchOAuthBatch } from "../components/workbench-oauth-batch";
import { oauthBatchDefaults, oauthBatchSchema } from "../lib/oauth-batch-schema";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
function mount(fetcher: typeof fetch): void {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("fetch", fetcher);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <WorkbenchOAuthBatch />
    </QueryClientProvider>,
  );
}
const recovery = {
  id: "queue-1",
  kind: "oauth-batch",
  task_id: "source-task",
  status: "interrupted",
  revision: 4,
  expires_at: new Date(Date.now() + 600000).toISOString(),
  can_resume: true,
};
const batch = {
  id: "restored-batch",
  task_id: "restored-task",
  status: "running",
  message: "继续未执行项目",
  expires_at: recovery.expires_at,
  available: 0,
  items: [],
  recovery_enabled: true,
  recovery_id: "queue-1",
};

it("恢复批次仅在确认后提交保存版本并显示新任务", async () => {
  const fetcher = vi.fn<typeof fetch>(async (url) =>
    Response.json(String(url).endsWith("queue-recoveries") ? [recovery] : batch),
  );
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  await user.click(await screen.findByRole("button", { name: "恢复本批" }));
  expect(fetcher.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false);
  await user.click(screen.getByRole("button", { name: "确认继续本批" }));
  expect(await screen.findByRole("region", { name: "批量授权进度" })).toHaveTextContent(
    "restored-task",
  );
  expect(fetcher).toHaveBeenCalledWith(
    "/api/account-workbench/queue-recoveries/queue-1/oauth",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ revision: 4, confirmed: true }),
    }),
  );
});

it("活动和过期批次不能恢复，删除资料先确认版本", async () => {
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json([
      { ...recovery, expires_at: "2020-01-01T00:00:00Z" },
      { ...recovery, id: "active", status: "running", can_resume: false },
    ]),
  );
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  (await screen.findAllByRole("button", { name: "恢复本批" })).forEach((button) =>
    expect(button).toBeDisabled(),
  );
  expect(screen.getAllByRole("button", { name: "删除恢复资料" })[1]).toBeDisabled();
  await user.click(screen.getAllByRole("button", { name: "删除恢复资料" })[0]);
  expect(fetcher.mock.calls.some(([, options]) => options?.method === "DELETE")).toBe(false);
  await user.click(screen.getByRole("button", { name: "确认删除恢复资料" }));
  await waitFor(() =>
    expect(fetcher).toHaveBeenCalledWith(
      "/api/account-workbench/queue-recoveries/queue-1",
      expect.objectContaining({
        method: "DELETE",
        body: JSON.stringify({ revision: 4, confirmed: true }),
      }),
    ),
  );
});

it("恢复批次读取失败保留重试和返回，重试后可显示空列表", async () => {
  let failed = true;
  mount(
    vi.fn<typeof fetch>(async () =>
      failed
        ? Response.json({ detail: "私有恢复资料暂不可用" }, { status: 503 })
        : Response.json([]),
    ),
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "恢复已保存批次" }));
  await screen.findByRole("button", { name: "重新读取" });
  expect(screen.getByRole("button", { name: "返回" })).toBeEnabled();
  failed = false;
  await user.click(screen.getByRole("button", { name: "重新读取" }));
  expect(await screen.findByText("暂无可恢复批次")).toBeInTheDocument();
});

it("批量恢复默认关闭，只有明确选择才写入恢复选项", async () => {
  const values = oauthBatchSchema.parse({ ...oauthBatchDefaults, content: "owner@example.com" });
  expect(values.recovery_enabled).toBe(false);
  const fetcher = vi.fn<typeof fetch>(async () =>
    Response.json({
      id: "preview",
      expires_at: recovery.expires_at,
      target: "https://target.test",
      items: [],
      errors: [],
    }),
  );
  mount(fetcher);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "填写批量账号" }));
  const saving = screen.getByRole("checkbox", {
    name: "保存本批私有输入及成功结果以便重启恢复（最长 2 小时）",
  });
  expect(saving).not.toBeChecked();
  await user.click(saving);
  await user.type(screen.getByRole("textbox", { name: "批量授权内容" }), "owner@example.com");
  await user.click(screen.getByRole("button", { name: "解析授权账号" }));
  await waitFor(() =>
    expect(
      fetcher.mock.calls.some(([, options]) =>
        String(options?.body).includes('"recovery_enabled":true'),
      ),
    ).toBe(true),
  );
});
