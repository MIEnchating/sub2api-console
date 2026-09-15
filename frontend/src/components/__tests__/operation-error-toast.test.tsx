import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { toast } from "sonner";

import { notifyOperationError } from "@/lib/operation-feedback";
import { Toaster } from "../ui/sonner";

afterEach(() => toast.dismiss());

describe("操作错误详情", () => {
  it("多上游长错误使用简短标题并保留可聚焦的完整详情及关闭入口", async () => {
    const user = userEvent.setup();
    const message = Array.from(
      { length: 20 },
      (_, index) => `upstream-${index}.test：上游鉴权失败（HTTP 401），请先完成鉴权恢复后刷新`,
    ).join("\n");
    render(<Toaster />);

    act(() => notifyOperationError(message, "上游价格读取失败"));

    expect(await screen.findByText("上游价格读取失败")).toBeInTheDocument();
    const details = screen.getByRole("region", { name: "错误详情" });
    expect(details.textContent).toBe(message);
    expect(details).toHaveAttribute("tabindex", "0");
    details.focus();
    expect(details).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByRole("button", { name: "关闭通知" })).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(screen.queryByRole("region", { name: "错误详情" })).toBeNull());
  });

  it("短错误仍直接展示原因且不增加详情区", async () => {
    render(<Toaster />);

    act(() => notifyOperationError("该上游分组已删除", "同步失败"));

    expect(await screen.findByText("该上游分组已删除")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "错误详情" })).toBeNull();
  });
});
