import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { UpstreamGroupBindingAuditItem, UpstreamGroupChange } from "@/api";
import { UpstreamGroupHistory } from "../upstream-group-history";

const changes: UpstreamGroupChange[] = [
  {
    id: 1,
    upstream_id: "up-1",
    group_id: "7",
    group_name: "新组",
    change_type: "added",
    changed_at: "2026-09-01T00:00:00Z",
  },
  {
    id: 2,
    upstream_id: "up-1",
    group_id: "8",
    group_name: "旧组",
    change_type: "removed",
    changed_at: "2026-09-02T00:00:00Z",
  },
];
const missingBinding: UpstreamGroupBindingAuditItem = {
  upstream_id: "up-1",
  host: "api.example.test",
  group_id: "8",
  group_name: "旧组",
  account_count: 2,
  accounts: [
    { id: "11", name: "账号甲" },
    { id: "12", name: "账号乙" },
  ],
  status: "missing",
  reason: "当前上游目录中已确认不存在",
};

it("新增分组有当前上游时点击添加账号直达该稳定分组", async () => {
  const onAddAccount = vi.fn();
  render(
    <UpstreamGroupHistory
      rows={[changes[0]]}
      upstreams={[{ upstream_id: "up-1", name: "上游", host: "api.example.test" }]}
      onAddAccount={onAddAccount}
    />,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 上游 的变化明细" }));
  await userEvent.setup().click(screen.getByRole("button", { name: "向新组添加账号" }));
  expect(onAddAccount).toHaveBeenCalledWith(changes[0]);
});

it("删除分组有已确认缺失的绑定时仅传递该稳定分组的账号", async () => {
  const onDeleteAccounts = vi.fn();
  render(
    <UpstreamGroupHistory
      rows={changes}
      upstreams={[{ upstream_id: "up-1", name: "上游", host: "api.example.test" }]}
      bindingAuditItems={[missingBinding]}
      onDeleteAccounts={onDeleteAccounts}
    />,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 上游 的变化明细" }));
  const removed = screen.getAllByRole("listitem")[0];
  expect(within(removed).getByText("2 个绑定账号")).toBeVisible();
  await userEvent
    .setup()
    .click(within(removed).getByRole("button", { name: "删除旧组的绑定账号" }));
  expect(onDeleteAccounts).toHaveBeenCalledWith(changes[1], ["11", "12"]);
});

it("分组重新出现或记录已过期时不显示删除和添加操作", async () => {
  const rows: UpstreamGroupChange[] = [
    ...changes,
    {
      id: 3,
      upstream_id: "up-1",
      group_id: "8",
      group_name: "旧组",
      change_type: "added",
      changed_at: "2026-09-03T00:00:00Z",
    },
    {
      id: 4,
      upstream_id: "up-1",
      group_id: "7",
      group_name: "新组",
      change_type: "removed",
      changed_at: "2026-09-04T00:00:00Z",
    },
  ];
  render(
    <UpstreamGroupHistory
      rows={rows}
      upstreams={[{ upstream_id: "up-1", name: "上游", host: "api.example.test" }]}
      bindingAuditItems={[{ ...missingBinding, status: "present" }]}
      onAddAccount={vi.fn()}
      onDeleteAccounts={vi.fn()}
    />,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "展开 上游 的变化明细" }));
  expect(screen.queryByRole("button", { name: "向新组添加账号" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "删除旧组的绑定账号" })).not.toBeInTheDocument();
});

it("清除单个上游变化时只回传该上游稳定 ID 且不展开明细", async () => {
  const onClear = vi.fn();
  render(
    <UpstreamGroupHistory
      rows={changes}
      upstreams={[{ upstream_id: "up-1", name: "上游", host: "api.example.test" }]}
      onClearUpstream={onClear}
    />,
  );
  await userEvent.setup().click(screen.getByRole("button", { name: "清除 上游 的变化记录" }));
  expect(onClear).toHaveBeenCalledWith("up-1", "上游");
  expect(screen.getByRole("button", { name: "展开 上游 的变化明细" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
});

it("从开户返回统计变化时自动展开之前操作的上游", () => {
  render(
    <UpstreamGroupHistory
      rows={changes}
      upstreams={[{ upstream_id: "up-1", name: "上游", host: "api.example.test" }]}
      initialExpandedUpstreamID="up-1"
    />,
  );
  expect(screen.getByRole("button", { name: "收起 上游 的变化明细" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  expect(screen.getByText("新组")).toBeVisible();
});

it("返回的上游在后续分页时自动定位到该上游所在页", async () => {
  const rows: UpstreamGroupChange[] = Array.from({ length: 21 }, (_, index) => ({
    ...changes[0],
    id: index + 1,
    upstream_id: `up-${index + 1}`,
  }));
  render(<UpstreamGroupHistory rows={rows} upstreams={[]} initialExpandedUpstreamID="up-1" />);
  expect(await screen.findByRole("button", { name: "收起 up-1 的变化明细" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );
  expect(screen.queryByRole("button", { name: "展开 up-21 的变化明细" })).not.toBeInTheDocument();
});
