import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { ReactElement } from "react";

import {
  PolicyOperationsEditor,
  PolicyPage,
  PolicyRulesEditor,
  PolicyScopeLayout,
  policyDraft,
} from "@/App";
import { policy } from "../../../../e2e/__tests__/fixtures/settings";

let client: QueryClient;

afterEach(() => {
  cleanup();
  client?.clear();
});

function renderPolicy(element: ReactElement): void {
  client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
  client.setQueryData(["policy"], policy);
  client.setQueryData(["config"], { probes_enabled: true });
  for (const kind of ["scheduling_strategy", "platform", "group", "account_type"]) {
    client.setQueryData(["dictionaries", kind], { items: [] });
  }
  render(<QueryClientProvider client={client}>{element}</QueryClientProvider>);
}

it("默认分类加载后，策略卡片共用自适应字段布局并保留两个中文选择器", () => {
  renderPolicy(<PolicyPage />);
  expect(screen.getByTestId("policy-page-layout")).toHaveClass("min-w-0", "space-y-4");
  expect(screen.getByRole("tabpanel", { name: "调度与写入" })).toBeInTheDocument();
  expect(screen.getByRole("combobox", { name: "全局默认策略" })).toHaveTextContent("均衡");
  expect(screen.getByRole("combobox", { name: "倍率缺失回退" })).toHaveTextContent(
    "回退当前成本墙",
  );
  expect(screen.getByRole("region", { name: "全局默认策略" })).toHaveClass(
    "@container/policy-card",
  );
});

it.each(["routing", "health", "sampling"] as const)(
  "%s 分类在宽屏分栏时保持卡片等高并允许内容收缩",
  (section) => {
    render(
      <PolicyOperationsEditor
        section={section}
        value={policyDraft(policy)}
        onChange={() => undefined}
        probesPending={false}
        onProbesEnabledChange={() => undefined}
      />,
    );
    expect(screen.getByTestId(`policy-${section}-sections`)).toHaveClass(
      "grid",
      "min-w-0",
      "items-stretch",
      "gap-4",
      "xl:grid-cols-2",
    );
  },
);

it("自动执行范围在足够宽的卡片内将四种执行开关并排", () => {
  renderPolicy(<PolicyPage />);
  expect(screen.getByTestId("policy-auto-apply")).toHaveClass(
    "@min-[28rem]/policy-card:grid-cols-2",
    "@min-[60rem]/policy-card:grid-cols-4",
  );
  expect(screen.getByRole("switch", { name: "调度状态自动执行" })).toBeChecked();
});

it("错误分类使用独立文本区域和状态码字段，不以跨列元素挤占窄卡片", () => {
  render(<PolicyRulesEditor value={policyDraft(policy)} onChange={() => undefined} />);
  expect(screen.getByTestId("policy-rules-sections")).toHaveClass("items-stretch", "gap-4");
  const patterns = screen.getByRole("textbox", { name: "致命错误关键字（每行一个）" });
  expect(patterns.closest('[data-slot="policy-fields"]')).toHaveClass(
    "grid-cols-1",
    "@min-[28rem]/policy-card:grid-cols-2",
  );
  expect(screen.getByRole("region", { name: "错误分类" })).toHaveClass("xl:col-span-2");
});

it("守护范围为空时保留选择入口，账号操作占满整行", () => {
  renderPolicy(
    <PolicyScopeLayout
      value={policyDraft(policy)}
      onChange={() => undefined}
      groups={[]}
      accounts={[]}
      onRestoreControl={() => undefined}
      restorePending={false}
    />,
  );
  expect(screen.getByTestId("policy-scope-layout")).toHaveClass(
    "min-w-0",
    "items-stretch",
    "gap-4",
    "xl:grid-cols-2",
  );
  expect(screen.getByRole("region", { name: "暂停与排除的账号" })).toHaveClass("xl:col-span-2");
  expect(screen.getByRole("combobox", { name: "选择暂停调度的账号" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "恢复全部账号原始配置" })).toBeEnabled();
});
