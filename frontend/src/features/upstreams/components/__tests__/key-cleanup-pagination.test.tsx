import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import type { Task } from "@/api";
import {
  OnboardingKeyCleanupDialog,
  type OnboardingKeyCleanupDialogProps,
} from "../onboarding-key-cleanup-dialog";

function previewProps(count: number): OnboardingKeyCleanupDialogProps {
  return {
    open: true,
    preview: {
      host: "cleanup.example.test",
      keys: Array.from({ length: count }, (_, index) => ({
        key_id: String(index + 1),
        name: `待清理 Key ${index + 1}`,
        group_id: "6",
        status: "active",
      })),
    },
    previewPending: false,
    previewError: null,
    task: null,
    taskPending: false,
    taskError: null,
    onOpenChange: vi.fn(),
    onRefresh: vi.fn(),
    onConfirm: vi.fn(),
    onComplete: vi.fn(),
  };
}

function cleanupTask(count: number, status: Task["status"] = "succeeded"): Task {
  return {
    id: "cleanup-1",
    skill: "sub2api-account-onboarding",
    operation: "upstream-key-cleanup",
    status,
    progress: status === "running" ? 50 : 100,
    message: status === "running" ? "正在清理 Key" : "Key 清理完成",
    result: {
      deleted: count - 2,
      skipped: 1,
      failed: 1,
      items: Array.from({ length: count }, (_, index) => ({
        key_id: String(index + 1),
        name: `已处理 Key ${index + 1}`,
        status: index < 2 ? ["skipped", "failed"][index] : "deleted",
        reason: index < 2 ? ["已建立绑定", "上游拒绝删除"][index] : undefined,
      })),
    },
    created_at: "2026-10-10T00:00:00Z",
    updated_at: "2026-10-10T00:01:00Z",
  };
}

it("500 个预览 Key 每页只渲染 20 行，键盘翻页后仍确认全部 500 个", async () => {
  const props = previewProps(500);
  render(<OnboardingKeyCleanupDialog {...props} />);
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(21);
  expect(screen.queryByText("待清理 Key 21", { exact: true })).not.toBeInTheDocument();
  const user = userEvent.setup();
  await user.tab();
  screen.getByRole("button", { name: "转到下一页" }).focus();
  expect(screen.getByRole("button", { name: "转到下一页" })).toHaveFocus();
  await user.keyboard("{Enter}");
  expect(screen.getByText("待清理 Key 21", { exact: true })).toBeVisible();
  expect(screen.queryByText("待清理 Key 1", { exact: true })).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "确认删除 500 个 Key" }));
  expect(props.onConfirm).toHaveBeenCalledOnce();
});

it("预览刷新保留当前页及表格，数量缩减时收敛到有效页，切换上游重置到首页", async () => {
  const props = previewProps(41);
  const view = render(<OnboardingKeyCleanupDialog {...props} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  const table = screen.getByRole("table");
  view.rerender(<OnboardingKeyCleanupDialog {...props} previewPending />);
  expect(screen.getByRole("table")).toBe(table);
  expect(screen.getByText("待清理 Key 21", { exact: true })).toBeVisible();
  expect(screen.getByRole("button", { name: "确认删除 41 个 Key" })).toBeDisabled();
  view.rerender(<OnboardingKeyCleanupDialog {...previewProps(1)} />);
  expect(screen.getByText("待清理 Key 1", { exact: true })).toBeVisible();
  expect(screen.getByRole("button", { name: "转到下一页" })).toBeDisabled();
  view.rerender(<OnboardingKeyCleanupDialog {...props} />);
  await user.click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(
    <OnboardingKeyCleanupDialog
      {...props}
      preview={{ ...props.preview!, host: "another.example.test" }}
    />,
  );
  expect(screen.getByText("待清理 Key 1", { exact: true })).toBeVisible();
  expect(screen.queryByText("待清理 Key 21", { exact: true })).not.toBeInTheDocument();
});

it("500 项终态结果分页展示，末页仍能查看最后一项", async () => {
  render(<OnboardingKeyCleanupDialog {...previewProps(0)} task={cleanupTask(500)} />);
  expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(21);
  await userEvent.setup().click(screen.getByRole("button", { name: "转到最后一页" }));
  expect(screen.getByText("已处理 Key 500", { exact: true })).toBeVisible();
  expect(screen.queryByText("已处理 Key 1", { exact: true })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "完成" })).toBeEnabled();
});

it("运行中显示已处理结果和真实进度，轮询新增结果及任务完成均保留所在页", async () => {
  const props = previewProps(0);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(<OnboardingKeyCleanupDialog {...props} task={cleanupTask(21, "running")} />, {
    wrapper: (wrapperProps) => (
      <QueryClientProvider client={client}>{wrapperProps.children}</QueryClientProvider>
    ),
  });
  expect(screen.getByText("已建立绑定", { exact: true })).toBeVisible();
  expect(screen.getByText("上游拒绝删除", { exact: true })).toBeVisible();
  expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "50");
  expect(within(screen.getByRole("group", { name: "Key 清理统计" })).getByText("19")).toBeVisible();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  expect(screen.queryByRole("button", { name: "完成" })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole("button", { name: "转到下一页" }));
  const table = screen.getByRole("table");
  view.rerender(<OnboardingKeyCleanupDialog {...props} task={cleanupTask(22, "running")} />);
  expect(screen.getByRole("table")).toBe(table);
  expect(screen.getByText("已处理 Key 22", { exact: true })).toBeVisible();
  expect(screen.queryByText("已处理 Key 1", { exact: true })).not.toBeInTheDocument();
  view.rerender(<OnboardingKeyCleanupDialog {...props} task={cleanupTask(22)} />);
  expect(screen.getByRole("table")).toBe(table);
  expect(screen.getByText("已处理 Key 22", { exact: true })).toBeVisible();
  expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "完成" })).toBeEnabled();
  client.clear();
});

it("运行任务尚无中途结果时只显示进度，不补造结果表格或统计", () => {
  const client = new QueryClient();
  const task = { ...cleanupTask(21, "running"), result: {}, progress: 0 };
  render(
    <QueryClientProvider client={client}>
      <OnboardingKeyCleanupDialog {...previewProps(0)} task={task} />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
  expect(screen.queryByRole("group", { name: "Key 清理统计" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "取消任务" })).toBeEnabled();
  client.clear();
});

it("切换清理任务时结果从首页展示", async () => {
  const props = previewProps(0);
  const task = cleanupTask(41);
  const view = render(<OnboardingKeyCleanupDialog {...props} task={task} />);
  await userEvent.setup().click(screen.getByRole("button", { name: "转到下一页" }));
  view.rerender(<OnboardingKeyCleanupDialog {...props} task={{ ...task, id: "cleanup-2" }} />);
  expect(screen.getByText("已处理 Key 1", { exact: true })).toBeVisible();
  expect(screen.queryByText("已处理 Key 21", { exact: true })).not.toBeInTheDocument();
});
