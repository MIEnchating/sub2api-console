import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { MonitorDialog } from "../monitor-dialog";
import { monitor } from "./fixtures";
it("模板列表读取中保留监控表单并显示局部反馈，手动配置仍可保存", () => {
  render(
    <MonitorDialog
      monitor={monitor}
      monitors={[monitor]}
      templates={[]}
      templatesPending
      pending={false}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );
  expect(screen.getByRole("status", { name: "正在读取可用模板" })).toHaveAttribute(
    "aria-busy",
    "true",
  );
  expect(screen.getByRole("combobox", { name: "功能模板" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "保存监控项" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeEnabled();
});

it("模板列表读取失败结束等待并提供重试，保持手动监控可编辑", () => {
  const retry = vi.fn();
  render(
    <MonitorDialog
      monitor={monitor}
      monitors={[monitor]}
      templates={[]}
      templatesError
      onTemplatesRetry={retry}
      pending={false}
      onClose={vi.fn()}
      onSubmit={vi.fn()}
    />,
  );
  expect(screen.queryByRole("status", { name: "正在读取可用模板" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "重新读取模板" }));
  expect(retry).toHaveBeenCalledOnce();
  expect(screen.getByRole("button", { name: "保存监控项" })).toBeEnabled();
});
