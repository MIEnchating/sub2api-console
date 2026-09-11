import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "@/api";
import { TaskProgressState, TaskStartupState, taskStartupStateLayout } from "../task-startup-state";

describe("task startup state", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });
  it("任务创建尚未返回进度时只显示忙碌提示，不伪造 0% 进度", () => {
    render(<TaskStartupState message="正在创建余额同步任务" />);
    expect(screen.getByRole("status", { name: "正在创建余额同步任务" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.queryByText("0%")).not.toBeInTheDocument();
  });

  it("reserves stable content height before the first task response arrives", () => {
    expect(taskStartupStateLayout.root).toContain("min-h-12");
    expect(taskStartupStateLayout.root).toContain("gap-3");
    expect(taskStartupStateLayout.heading).toContain("min-w-0");
  });

  it("uses the same layout for running task progress", () => {
    const markup = renderToStaticMarkup(<TaskProgressState message="正在校验账号" progress={46} />);

    expect(markup).toContain("正在校验账号");
    expect(markup).toContain("46%");
    expect(markup).toContain(taskStartupStateLayout.root);
    expect(markup).toContain('aria-valuenow="46"');
  });

  it("cancels the exact running task and prevents duplicate requests", async () => {
    const cancel = vi.spyOn(api, "cancelTask").mockResolvedValue({ cancelled: true });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    client.setQueryData(["task", "task-41"], { id: "task-41", status: "waiting_input" });
    render(
      <QueryClientProvider client={client}>
        <TaskProgressState message="正在校验账号" progress={46} taskId="task-41" />
      </QueryClientProvider>,
    );

    const button = screen.getByRole("button", { name: "取消任务" });
    expect(button).toHaveClass("h-8");
    expect(button).not.toHaveClass("h-7");
    await userEvent.click(button);

    await waitFor(() => expect(cancel).toHaveBeenCalledWith("task-41"));
    expect(screen.getByRole("button", { name: "已请求取消" })).toBeDisabled();
    expect(client.getQueryState(["task", "task-41"])?.isInvalidated).toBe(true);
  });
});
