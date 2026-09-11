import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { OnboardingKeyCleanupDialog } from "../onboarding-key-cleanup-dialog";

it("重新扫描 Key 时保留预览尺寸及表格，扫描失败后禁用删除但可以重试", () => {
  const props = {
    open: true,
    preview: {
      host: "upstream.example",
      keys: [{ key_id: "17", name: "unused-key", group_id: "6", status: "active" }],
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
  const view = render(<OnboardingKeyCleanupDialog {...props} />);
  const table = screen.getByRole("table");
  const dialog = screen.getByRole("dialog");
  const originalWidth = dialog.className;
  fireEvent.click(screen.getByRole("button", { name: "刷新扫描结果" }));
  expect(props.onRefresh).toHaveBeenCalledOnce();
  view.rerender(<OnboardingKeyCleanupDialog {...props} previewPending />);
  expect(screen.getByRole("table")).toBe(table);
  expect(dialog.className).toBe(originalWidth);
  expect(screen.queryByText("正在扫描上游 Key 与绑定关系")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认删除 1 个 Key" })).toBeDisabled();
  view.rerender(<OnboardingKeyCleanupDialog {...props} previewError={new Error("扫描连接失败")} />);
  expect(screen.getByRole("table")).toBe(table);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.queryByText("扫描连接失败")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "确认删除 1 个 Key" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "刷新扫描结果" })).toBeEnabled();
});
