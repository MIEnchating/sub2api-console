import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { UpstreamGroupBindingAuditItem, UpstreamGroupChange } from "@/api";
import { UpstreamGroupHistory } from "../upstream-group-history";

const change: UpstreamGroupChange = {
  id: 1,
  upstream_id: "up-1",
  group_id: "7",
  group_name: "新组",
  change_type: "added",
  changed_at: "2026-09-01T00:00:00Z",
};
const binding: UpstreamGroupBindingAuditItem = {
  upstream_id: "up-1",
  host: "api.example.test",
  group_id: "7",
  group_name: "新组",
  account_count: 1,
  accounts: [{ id: "11", name: "账号甲" }],
  status: "present",
  reason: null,
};

it.each(["overview", "details"] as const)(
  "%s 中新增分组已有绑定时显示已绑定并隐藏添加账号入口",
  (view) => {
    render(
      <UpstreamGroupHistory
        rows={[change]}
        upstreams={
          view === "overview"
            ? [{ upstream_id: "up-1", name: "上游", host: "api.example.test" }]
            : undefined
        }
        initialExpandedUpstreamID="up-1"
        upstreamAvailable
        bindingAuditItems={[binding]}
        onAddAccount={vi.fn()}
      />,
    );
    expect(screen.getByText("已绑定")).toBeVisible();
    expect(screen.queryByRole("button", { name: "向新组添加账号" })).not.toBeInTheDocument();
  },
);

it("目录状态未确认但存在多个绑定账号时仍显示已绑定", () => {
  render(
    <UpstreamGroupHistory
      rows={[change]}
      upstreamAvailable
      bindingAuditItems={[
        {
          ...binding,
          status: "unknown",
          account_count: 2,
          accounts: [...binding.accounts, { id: "12", name: "账号乙" }],
        },
      ]}
      onAddAccount={vi.fn()}
    />,
  );
  expect(screen.getByText("已绑定")).toBeVisible();
  expect(screen.queryByRole("button", { name: "向新组添加账号" })).not.toBeInTheDocument();
});

it.each([
  { label: "无绑定记录", items: [] },
  { label: "同名分组属于其他上游", items: [{ ...binding, upstream_id: "up-2" }] },
  { label: "同名分组的 ID 不同", items: [{ ...binding, group_id: "8" }] },
  { label: "绑定账号为空", items: [{ ...binding, account_count: 0, accounts: [] }] },
])("$label 时保留添加账号入口且不显示已绑定", (scenario) => {
  render(
    <UpstreamGroupHistory
      rows={[change]}
      upstreamAvailable
      bindingAuditItems={scenario.items}
      onAddAccount={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "向新组添加账号" })).toBeEnabled();
  expect(screen.queryByText("已绑定")).not.toBeInTheDocument();
});
