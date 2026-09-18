import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import { account } from "@/features/accounts/__tests__/fixtures";
import { ModelCheckPage } from "../model-check-page";

beforeEach(() => vi.stubGlobal("PointerEvent", MouseEvent));
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(accountID?: string): () => void {
  const client = createConsoleQueryClient();
  client.setDefaultOptions({ queries: { retry: false, staleTime: Infinity } });
  client.setQueryData(
    ["accounts"],
    [
      {
        ...account,
        id: "41",
        name: "人工账号",
        groups: ["主组"],
        platform: "openai",
        manual_priority: 0,
      },
      {
        ...account,
        id: "42",
        name: "自动账号",
        groups: ["主组"],
        platform: "anthropic",
        manual_priority: null,
      },
      {
        ...account,
        id: "43",
        name: "备用账号",
        groups: ["备用"],
        platform: "openai",
        manual_priority: null,
      },
    ],
  );
  client.setQueryData(["model-check-capabilities"], {
    claude_standards: [],
    sol_models: ["gpt-5.6-sol"],
  });
  client.setQueryData(["model-check-account-statuses"], []);
  client.setQueryData(["model-check-account-models", "41"], { models: ["gpt-5.6-sol"] });
  client.setQueryData(["model-animation", "schedules"], []);
  client.setQueryData(["model-animation", "history"], []);
  client.setQueryData(["accounts", "live-traffic"], {
    enabled: true,
    observed_at: new Date().toISOString(),
    accounts: [
      { account_id: "41", current_requests: 1, waiting_requests: 0, tracked: true },
      { account_id: "42", current_requests: 0, waiting_requests: 2, tracked: true },
    ],
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/api/accounts/traffic")) {
        return Response.json({
          ...client.getQueryData<Record<string, unknown>>(["accounts", "live-traffic"]),
          observed_at: new Date().toISOString(),
        });
      }
      return Response.json({ items: [] });
    }),
  );
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckPage accountID={accountID} onBackToAccounts={() => undefined} />
    </QueryClientProvider>,
  );
  return () => {
    view.unmount();
    client.clear();
  };
}

it("键盘切换检测 Tab 时展示独立面板，并保留两种检测的选择和统一模型", async () => {
  const user = userEvent.setup();
  const dispose = setup("41");
  const regular = screen.getByRole("tab", { name: "常规检测" });
  const heading = screen
    .getByRole("heading", { name: "模型检测" })
    .closest('[data-slot="page-heading"]');
  expect(heading).toContainElement(screen.getByRole("button", { name: "账号管理" }));
  expect(heading).toContainElement(screen.getByRole("button", { name: "检测规则与题库" }));
  expect(regular).toHaveAttribute("aria-selected", "true");
  expect(screen.getAllByRole("button", { name: "检测规则与题库" })).toHaveLength(1);
  expect(screen.getByRole("checkbox", { name: /选择账号 人工账号/ })).toBeChecked();
  expect(screen.queryByRole("checkbox", { name: /选择账号 自动账号/ })).not.toBeInTheDocument();
  expect(screen.queryByText("人工账号（#41）")).not.toBeInTheDocument();
  regular.focus();
  await user.keyboard("{ArrowRight}{Enter}");
  const panel = await screen.findByRole("tabpanel", { name: "动画检测" });
  await within(panel).findByRole("combobox", { name: "检测模型" }, { timeout: 10_000 });
  expect(screen.getByRole("tab", { name: "动画检测" })).toHaveAttribute("aria-selected", "true");
  expect(heading).toContainElement(screen.getByRole("button", { name: "账号管理" }));
  expect(heading).toContainElement(screen.getByRole("button", { name: "检测规则与题库" }));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("结果展示时长（秒）")).not.toBeInTheDocument();
  expect(within(panel).getAllByRole("combobox", { name: "检测模型" })).toHaveLength(1);
  expect(screen.queryByLabelText("账号检测模型")).not.toBeInTheDocument();
  expect(within(panel).getByRole("checkbox", { name: /检测 人工账号/ })).toBeChecked();
  fireEvent.click(within(panel).getByRole("button", { name: "清空选择" }));
  fireEvent.click(within(panel).getByRole("button", { name: "选择实时流量（1）" }));
  expect(within(panel).getByRole("checkbox", { name: /检测 人工账号/ })).toBeChecked();
  expect(within(panel).queryByRole("article", { name: "账号 自动账号" })).not.toBeInTheDocument();
  fireEvent.change(within(panel).getByRole("combobox", { name: "检测模型" }), {
    target: { value: "shared-model" },
  });

  fireEvent.click(regular);
  expect(screen.getByRole("checkbox", { name: /选择账号 人工账号/ })).toBeChecked();
  fireEvent.click(screen.getByRole("tab", { name: "动画检测" }));
  expect(screen.getByRole("checkbox", { name: /检测 人工账号/ })).toBeChecked();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("shared-model");
  await user.click(screen.getByRole("button", { name: "查看全部账号" }));
  expect(screen.getByRole("article", { name: "账号 自动账号" })).toBeVisible();
  fireEvent.click(regular);
  expect(screen.getByRole("checkbox", { name: /选择账号 自动账号/ })).not.toBeChecked();
  fireEvent.click(screen.getByRole("button", { name: "选择实时流量（1）" }));
  expect(screen.getByRole("checkbox", { name: /选择账号 人工账号/ })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: /选择账号 自动账号/ })).not.toBeChecked();
  dispose();
}, 15_000);

it("链接账号不存在时动画面板保持空列表且禁止开始检测", async () => {
  const dispose = setup("99");
  fireEvent.click(screen.getByRole("tab", { name: "动画检测" }));
  const panel = screen.getByRole("tabpanel", { name: "动画检测" });
  expect(await within(panel).findByText("没有匹配的账号", {}, { timeout: 10_000 })).toBeVisible();
  expect(within(panel).queryByRole("article")).not.toBeInTheDocument();
  expect(within(panel).getByRole("button", { name: "开始检测（0 个账号）" })).toBeDisabled();
  dispose();
}, 15_000);
