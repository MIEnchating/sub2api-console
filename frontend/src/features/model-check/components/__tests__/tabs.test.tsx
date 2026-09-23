import { QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { createConsoleQueryClient } from "@/lib/query-client";
import { account } from "@/features/accounts/__tests__/fixtures";
import { ModelCheckPage } from "../model-check-page";
import { AnimationCheckPage } from "../animation-check-page";

beforeAll(async () => {
  await import("../animation-check-panel");
});

beforeEach(() => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup(accountID?: string, animation = true): () => void {
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
  client.setQueryData(["terminal-continuity", "history"], []);
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({ items: [] })),
  );
  const view = render(
    <QueryClientProvider client={client}>
      {animation ? (
        <AnimationCheckPage accountID={accountID} onBackToAccounts={() => undefined} />
      ) : (
        <ModelCheckPage accountID={accountID} onBackToAccounts={() => undefined} />
      )}
    </QueryClientProvider>,
  );
  return () => {
    view.unmount();
    client.clear();
  };
}

it("独立动画页保留账号链接、模型输入和查看全部账号入口", async () => {
  const user = userEvent.setup();
  const dispose = setup("41");
  expect(screen.getByRole("heading", { name: "动画检测" })).toBeVisible();
  const panel = await screen.findByRole("tabpanel", { name: "账号检测" });
  const model = await within(panel).findByRole("combobox", { name: "检测模型" });
  expect(screen.queryByRole("tab", { name: "常规检测" })).not.toBeInTheDocument();
  expect(screen.getByRole("tab", { name: "前置检测" })).toBeVisible();
  expect(screen.getByRole("tab", { name: "终端续接检测" })).toBeVisible();
  expect(within(panel).getByRole("checkbox", { name: /检测 人工账号/ })).toBeChecked();
  fireEvent.change(model, { target: { value: "shared-model" } });
  await user.click(screen.getByRole("button", { name: "查看全部账号" }));
  expect(screen.getByRole("article", { name: "账号 自动账号" })).toBeVisible();
  expect(screen.queryByRole("button", { name: /选择实时流量/ })).not.toBeInTheDocument();
  expect(model).toHaveValue("shared-model");
  const accountsTab = screen.getByRole("tab", { name: "账号检测" });
  accountsTab.focus();
  await user.keyboard("{ArrowRight}{Enter}");
  expect(screen.getByRole("tab", { name: "前置检测" })).toHaveAttribute("aria-selected", "true");
  await user.keyboard("{ArrowRight}{Enter}");
  expect(screen.getByRole("tab", { name: "自定义接口" })).toHaveAttribute("aria-selected", "true");
  dispose();
});

it("前置检测使用独立 Tab 并与账号检测共享模型和账号选择", async () => {
  const user = userEvent.setup();
  const dispose = setup("41");
  const animationPanel = await screen.findByRole("tabpanel", { name: "账号检测" });
  const animationModel = within(animationPanel).getByRole("combobox", { name: "检测模型" });
  fireEvent.change(animationModel, { target: { value: "shared-model" } });
  fireEvent.change(within(animationPanel).getByRole("spinbutton", { name: "请求超时（秒）" }), {
    target: { value: "60" },
  });

  expect(
    within(animationPanel).queryByRole("button", { name: /前置检测/ }),
  ).not.toBeInTheDocument();
  expect(
    within(animationPanel).queryByRole("region", { name: "前置检测结果" }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "前置检测" }));

  const precheckPanel = screen.getByRole("tabpanel", { name: "前置检测" });
  expect(within(precheckPanel).getByRole("combobox", { name: "检测模型" })).toHaveValue(
    "shared-model",
  );
  expect(within(precheckPanel).getByRole("checkbox", { name: /检测 人工账号/ })).toBeChecked();
  expect(within(precheckPanel).getByRole("spinbutton", { name: "请求超时（秒）" })).toHaveValue(60);
  expect(within(precheckPanel).getByRole("button", { name: "前置检测（1）" })).toBeVisible();
  expect(within(precheckPanel).queryByRole("button", { name: /开始检测/ })).not.toBeInTheDocument();
  expect(within(precheckPanel).getByRole("region", { name: "前置检测结果" })).toBeVisible();
  expect(
    within(precheckPanel).queryByRole("group", { name: "动画预览区域" }),
  ).not.toBeInTheDocument();
  await user.click(within(precheckPanel).getByRole("button", { name: "清空选择" }));
  await user.click(screen.getByRole("tab", { name: "账号检测" }));
  expect(screen.getByRole("checkbox", { name: /检测 人工账号/ })).not.toBeChecked();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("shared-model");
  dispose();
});

it("模型检测页保留规则入口且不再内嵌动画页", () => {
  const dispose = setup("41", false);
  expect(screen.getByRole("heading", { name: "模型检测" })).toBeVisible();
  expect(screen.getByRole("button", { name: "检测规则与题库" })).toBeVisible();
  expect(screen.queryByRole("tab", { name: "动画检测" })).not.toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /选择账号 人工账号/ })).toBeChecked();
  expect(screen.queryByRole("button", { name: /选择实时流量/ })).not.toBeInTheDocument();
  dispose();
});

it("链接账号不存在时动画面板保持空列表且禁止开始检测", async () => {
  const dispose = setup("99");
  expect(await screen.findByText("没有匹配的账号")).toBeVisible();
  expect(screen.queryByRole("article")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "开始检测（0 个账号）" })).toBeDisabled();
  dispose();
});
