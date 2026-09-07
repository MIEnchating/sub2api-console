import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import type { AccountModelSyncPreview, Task } from "@/api";
import { AccountModelSyncDialog } from "../account-model-sync-dialog";

afterEach(() => vi.unstubAllGlobals());

function renderModelSync(result: {
  phase: "discovery" | "apply";
  status: "failed" | "cancelled";
  message: string;
}): void {
  const discovery: Task = {
    id: "discovery",
    skill: "account-model-sync",
    operation: "account-model-discovery",
    status: "succeeded",
    progress: 100,
    message: "模型发现完成",
    result: { items: [{ account_id: "41", account_name: "账号 A", status: "succeeded" }] },
    created_at: "2026-09-07T00:00:00Z",
    updated_at: "2026-09-07T00:00:01Z",
  };
  const failed: Task = {
    ...discovery,
    id: result.phase,
    operation: `account-model-${result.phase}`,
    status: result.status,
    message: result.message,
    result: { error: result.message, remote_write: false },
  };
  const preview: AccountModelSyncPreview = {
    account_count: 1,
    accounts_with_catalog: 1,
    blocked_patterns: [],
    blocked_models: [],
    models: [{ model: "model-a", account_count: 1 }],
    accounts: [
      { account_id: "41", account_name: "账号 A", models: ["model-a"], probe_model: "model-a" },
    ],
    fingerprint: "catalog",
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      let body: unknown;
      if (path.endsWith("/models/discover") || path.endsWith("/tasks/discovery"))
        body = result.phase === "discovery" ? failed : discovery;
      else if (path.endsWith("/models/preview")) body = preview;
      else if (path.endsWith("/models/apply") || path.endsWith("/tasks/apply")) body = failed;
      else throw new Error(`Unexpected test request: ${path}`);
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AccountModelSyncDialog
        open
        accountIds={["41"]}
        accountPlatforms={new Map()}
        accountGroups={new Map()}
        onOpenChange={() => undefined}
        onCompleted={() => undefined}
      />
    </QueryClientProvider>,
  );
}

it("模型发现任务在生成逐账号结果前失败时显示真实失败原因", async () => {
  renderModelSync({
    phase: "discovery",
    status: "failed",
    message: "模型发现失败：管理目标配置已变更",
  });

  expect(await screen.findByText("模型发现失败：管理目标配置已变更")).toBeVisible();
});

it("模型应用在目录复核阶段失败时显示失败原因而非完成提示", async () => {
  const user = userEvent.setup();
  renderModelSync({
    phase: "apply",
    status: "failed",
    message: "模型目录在排队期间发生变化，请重新读取",
  });

  await user.click(await screen.findByRole("button", { name: "同步 1 个账号" }));

  expect(await screen.findByText("模型目录在排队期间发生变化，请重新读取")).toBeVisible();
  expect(screen.queryByText("模型写入和探活验证均已完成")).not.toBeInTheDocument();
});

it("模型应用任务被取消且尚无逐账号结果时显示取消状态", async () => {
  const user = userEvent.setup();
  renderModelSync({ phase: "apply", status: "cancelled", message: "账号模型同步已取消" });

  await user.click(await screen.findByRole("button", { name: "同步 1 个账号" }));

  expect(await screen.findByText("账号模型同步已取消")).toBeVisible();
  expect(screen.queryByText("模型写入和探活验证均已完成")).not.toBeInTheDocument();
});
