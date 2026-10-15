import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { account } from "@/features/accounts/__tests__/fixtures";
import { seedDictionaries } from "@/test/dictionary-fixtures";
import { RegularCheckPanel } from "../regular-check-panel";
import { AnimationCheckPanel } from "../animation-check-panel";
import { AnimationAccountCard } from "../animation-account-card";
import { DetectionAccountControlProvider } from "../detection-account-control-provider";

const clients: QueryClient[] = [];
beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.unstubAllGlobals();
});
const excluded = { ...account, name: "排除账号", platform: "openai", upstream_excluded: true };
const available = { ...account, id: "42", name: "可用账号", platform: "openai" };
function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  clients.push(client);
  seedDictionaries(client);
  client.setQueryData(["accounts"], [excluded, available]);
  client.setQueryData(["policy"], {});
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["detection-timeout", "animation"], { timeout_seconds: 120, version: 0 });
  client.setQueryData(["detection-timeout", "precheck"], { timeout_seconds: 120, version: 0 });
  const fetcher = vi.fn<typeof fetch>(async () => Response.json({ models: ["fixture-model"] }));
  vi.stubGlobal("fetch", fetcher);
  return { client, fetcher, user: userEvent.setup() };
}

it("模型检测全选跳过排除账号，后台排除已选账号后清空选择并禁用检测", async () => {
  const view = setup();
  render(
    <QueryClientProvider client={view.client}>
      <RegularCheckPanel />
    </QueryClientProvider>,
  );
  for (const checkbox of screen.getAllByRole("checkbox", { name: /排除账号/ })) {
    expect(checkbox).toHaveAttribute("aria-disabled", "true");
  }
  await view.user.click(screen.getByRole("button", { name: "全选账号" }));
  await waitFor(() => expect(view.fetcher).toHaveBeenCalled());
  expect(
    view.fetcher.mock.calls
      .map(([input]) => String(input))
      .filter((path) => path.includes("/models")),
  ).toEqual(["/api/accounts/42/models"]);
  await act(async () =>
    view.client.setQueryData(["accounts"], [excluded, { ...available, upstream_excluded: true }]),
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "全选账号" })).toBeDisabled());
  expect(screen.getByRole("button", { name: "开始检测" })).toBeDisabled();
  expect(screen.getByText("共同模型 · 已选 0/20")).toBeVisible();
});

it("模型检测从排除账号链接进入时不读取该账号模型", async () => {
  const view = setup();
  render(
    <QueryClientProvider client={view.client}>
      <RegularCheckPanel accountID="41" />
    </QueryClientProvider>,
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "全选账号" })).toBeDisabled());
  await waitFor(() => expect(screen.getByText("共同模型 · 已选 0/20")).toBeVisible());
  expect(
    view.fetcher.mock.calls.some(([input]) => String(input).includes("/accounts/41/models")),
  ).toBe(false);
});

it("智商检测全选只选择可用账号，排除后清空选择，取消排除后恢复选择入口", async () => {
  const view = setup();
  render(
    <QueryClientProvider client={view.client}>
      <AnimationCheckPanel />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("checkbox", { name: /检测 排除账号/ })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  await view.user.click(screen.getByRole("button", { name: "全选账号" }));
  expect(screen.getByRole("checkbox", { name: /检测 可用账号/ })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: /检测 排除账号/ })).not.toBeChecked();
  await act(async () =>
    view.client.setQueryData(["accounts"], [excluded, { ...available, upstream_excluded: true }]),
  );
  await waitFor(() => expect(screen.getByRole("button", { name: "全选账号" })).toBeDisabled());
  expect(screen.getByRole("checkbox", { name: /检测 可用账号/ })).not.toBeChecked();
  expect(screen.getByRole("button", { name: "开始检测（0 个账号）" })).toBeDisabled();
  await act(async () => view.client.setQueryData(["accounts"], [excluded, available]));
  await waitFor(() =>
    expect(screen.getByRole("checkbox", { name: /检测 可用账号/ })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    ),
  );
  expect(screen.getByRole("checkbox", { name: /检测 可用账号/ })).not.toBeChecked();
});

it.each(["animation", "precheck"] as const)(
  "上游排除时 %s 卡片禁用账号处置和设置，历史结果仍展示",
  (mode) => {
    const view = setup();
    render(
      <QueryClientProvider client={view.client}>
        <DetectionAccountControlProvider>
          <AnimationAccountCard
            mode={mode}
            account={excluded}
            checked={false}
            disabled={false}
            retryDisabled={false}
            schedulesReady
            onRetry={vi.fn()}
            onToggle={vi.fn()}
            onSchedule={vi.fn()}
            onManualPriority={vi.fn()}
            result={{
              account_id: "41",
              account_name: "排除账号",
              model: "fixture-model",
              request_id: "request-1",
              status: "failed",
              error: "历史失败原因",
              duration_ms: 10,
              completed_at: "2026-10-15T00:00:00Z",
            }}
          />
        </DetectionAccountControlProvider>
      </QueryClientProvider>,
    );
    for (const button of within(screen.getByRole("group", { name: "账号操作" })).getAllByRole(
      "button",
    ))
      expect(button).toBeDisabled();
    expect(screen.getByText("上游已排除，请先在上游管理取消排除后操作")).toBeVisible();
    if (mode === "animation") {
      expect(screen.getByText("历史失败原因")).toBeVisible();
      expect(screen.getByRole("button", { name: "重测 排除账号" })).toBeDisabled();
    }
  },
);
