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

function setup(): () => void {
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
  const view = render(
    <QueryClientProvider client={client}>
      <ModelCheckPage />
    </QueryClientProvider>,
  );
  return () => {
    view.unmount();
    client.clear();
  };
}

it("键盘切换检测 Tab 时展示独立面板，并保留两种检测的选择和统一模型", async () => {
  const user = userEvent.setup();
  const dispose = setup();
  const regular = screen.getByRole("tab", { name: "常规检测" });
  expect(regular).toHaveAttribute("aria-selected", "true");
  fireEvent.click(screen.getByRole("checkbox", { name: /选择账号 人工账号/ }));
  expect(screen.getByRole("checkbox", { name: /选择账号 人工账号/ })).toBeChecked();
  regular.focus();
  await user.keyboard("{ArrowRight}{Enter}");
  const panel = await screen.findByRole("tabpanel", { name: "动画检测" });
  await within(panel).findByRole("combobox", { name: "检测模型" });
  expect(screen.getByRole("tab", { name: "动画检测" })).toHaveAttribute("aria-selected", "true");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("结果展示时长（秒）")).not.toBeInTheDocument();
  expect(within(panel).getAllByRole("combobox", { name: "检测模型" })).toHaveLength(1);
  expect(screen.queryByLabelText("账号检测模型")).not.toBeInTheDocument();
  fireEvent.click(within(panel).getByRole("checkbox", { name: /检测 人工账号/ }));
  fireEvent.change(within(panel).getByRole("combobox", { name: "检测模型" }), {
    target: { value: "shared-model" },
  });
  fireEvent.click(regular);
  expect(screen.getByRole("checkbox", { name: /选择账号 人工账号/ })).toBeChecked();
  fireEvent.click(screen.getByRole("tab", { name: "动画检测" }));
  expect(screen.getByRole("checkbox", { name: /检测 人工账号/ })).toBeChecked();
  expect(screen.getByRole("combobox", { name: "检测模型" })).toHaveValue("shared-model");
  dispose();
});
