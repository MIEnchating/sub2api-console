import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import {
  OnboardingConfirmDialog,
  type OnboardingBindingPreview,
} from "../onboarding-confirm-dialog";

const item: OnboardingBindingPreview = {
  id: "deepseek",
  host: "models.test",
  upstreamGroupId: "7",
  upstreamGroup: "DeepSeek",
  platform: "DeepSeek",
  multiplier: "0.06",
  localGroup: "国产-特价",
  concurrency: 100,
  priority: 1000,
  status: "待添加",
};
let client: QueryClient;
afterEach(() => {
  cleanup();
  client?.clear();
});

function renderDialog(items: OnboardingBindingPreview[]): void {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <OnboardingConfirmDialog
        open
        items={items}
        pending={false}
        onOpenChange={() => undefined}
        onConfirm={() => undefined}
      />
    </QueryClientProvider>,
  );
}

it("混合批量变更时在顶部汇总新增和更新数量，并保留每个账号的独立摘要", () => {
  renderDialog([item, { ...item, id: "glm", upstreamGroup: "GLM", status: "待更新" }]);
  const summary = screen.getByRole("group", { name: "变更概览" });
  expect(summary).toHaveTextContent("共 2 项变更");
  expect(summary).toHaveTextContent("新增 1");
  expect(summary).toHaveTextContent("更新 1");
  expect(
    within(screen.getByRole("region", { name: "DeepSeek → 国产-特价" }))
      .getAllByRole("definition")
      .map((node) => node.textContent),
  ).toEqual(["DeepSeek", "0.06", "100", "1000", "开启"]);
  expect(
    within(screen.getByRole("region", { name: "GLM → 国产-特价" })).queryByText("模型配置"),
  ).not.toBeInTheDocument();
});

it("空映射显示简短默认说明，添加后显示编辑帮助，删除后恢复默认说明", async () => {
  const user = userEvent.setup();
  renderDialog([item]);
  expect(screen.getByText("使用默认映射")).toBeVisible();
  expect(screen.queryByText(/选择上游模型后自动回填/)).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "添加模型映射" }));
  expect(screen.getByText(/选择上游模型后自动回填/)).toBeVisible();
  expect(screen.queryByText("使用默认映射")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "删除第 1 条模型映射" }));
  expect(screen.getByText("使用默认映射")).toBeVisible();
});
