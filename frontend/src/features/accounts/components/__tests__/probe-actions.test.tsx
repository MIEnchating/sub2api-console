import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";

import { ProbeDialogActions } from "../account-probe-dialog";

afterEach(cleanup);

it("探活运行中只提供取消并关闭入口，点击沿用关闭清理流程", async () => {
  const onClose = vi.fn();
  render(
    <ProbeDialogActions
      runDisabled
      probePending
      hasResult={false}
      onClose={onClose}
      onRun={vi.fn()}
    />,
  );
  expect(screen.getAllByRole("button")).toHaveLength(2);
  expect(screen.queryByRole("button", { name: "取消探活" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "关闭" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "取消并关闭" }));
  expect(onClose).toHaveBeenCalledOnce();
  expect(screen.getByRole("button", { name: "测试中" })).toBeDisabled();
});

it("清理完成前显示正在关闭并阻止重复点击", () => {
  render(
    <ProbeDialogActions
      runDisabled
      probePending
      hasResult={false}
      closing
      onClose={vi.fn()}
      onRun={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "正在关闭" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "正在关闭" })).toHaveAttribute("aria-busy", "true");
});

it("探活结束后显示关闭和重试两个操作", () => {
  render(
    <ProbeDialogActions
      runDisabled={false}
      probePending={false}
      hasResult
      onClose={vi.fn()}
      onRun={vi.fn()}
    />,
  );
  expect(screen.getAllByRole("button")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
});
