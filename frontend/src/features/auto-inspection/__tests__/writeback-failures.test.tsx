import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AutoInspectionHeartbeatDetails } from "@/App";
import type { AutoInspectionStatus, Task } from "@/api";

const record: AutoInspectionStatus["heartbeat_history"][number] = {
  checked_at: "2026-09-14T10:28:40Z",
  completed_at: "2026-09-14T10:30:44Z",
  status: "partial",
  operations: ["routing_writeback"],
  operation_timings: [{ operation: "routing_writeback", duration_seconds: 6 }],
  task_id: "inspection-failures",
  error: "自动执行部分失败：10 项",
  skipped: false,
};

function task(writeback: unknown): Task {
  return {
    id: "inspection-failures",
    skill: "sub2api-auto-inspection",
    operation: "automatic-inspection",
    status: "partial",
    progress: 100,
    message: "巡检完成，但存在部分失败",
    result: { writeback },
    created_at: record.checked_at,
    updated_at: record.completed_at!,
  };
}

describe("自动执行失败明细", () => {
  it("10 个账号失败时直接列出全部 ID 和原因，排除成功和跳过的账号", () => {
    const failures = Array.from({ length: 10 }, (_, index) => ({
      account_id: String(41 + index),
      error: `账号 ${41 + index} 读回校验不一致`,
      changed: false,
    }));
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({
          changed: 171,
          succeeded: 371,
          failed: 10,
          results: [
            { account_id: "1", changed: true },
            ...failures,
            { account_id: "2", skipped: true, reason: "人工优先位保护" },
          ],
        })}
      />,
    );

    const list = screen.getByRole("list", { name: "自动执行失败账号" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(10);
    for (const failure of failures) {
      expect(within(list).getByText(`ID ${failure.account_id}`)).toBeVisible();
      expect(within(list).getByText(failure.error)).toBeVisible();
    }
    expect(within(list).queryByText("ID 1")).not.toBeInTheDocument();
    expect(within(list).queryByText("ID 2")).not.toBeInTheDocument();
  });

  it("单账号错误字段为空但已记录失败时仍保留账号和缺失原因提示", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, error: "自动执行部分失败：1 项" }}
        task={task({ failed: 1, results: [{ account_id: "41", error: "" }] })}
      />,
    );

    expect(screen.getByText("ID 41")).toBeVisible();
    expect(screen.getByText(/未返回具体原因/)).toBeVisible();
  });

  it("账号列表顺序不同且名称重复时按稳定 ID 补充名称并保留任务中的名称", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, error: "自动执行部分失败：2 项" }}
        accounts={[
          { id: "52", name: "同名账号" },
          { id: "41", name: "同名账号" },
        ]}
        task={task({
          failed: 2,
          results: [
            { account_id: "52", account_name: "任务记录名称", error: "远程写入被拒绝" },
            { account_id: "41", error: "读取账号超时" },
          ],
        })}
      />,
    );

    const rows = within(screen.getByRole("list", { name: "自动执行失败账号" })).getAllByRole(
      "listitem",
    );
    expect(rows[0]).toHaveTextContent("ID 41");
    expect(within(rows[0]).getByText("同名账号")).toBeVisible();
    expect(rows[1]).toHaveTextContent("ID 52");
    expect(within(rows[1]).getByText("任务记录名称")).toBeVisible();
  });

  it("任务未保存失败总数时仍根据实际错误记录展示失败账号", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, error: null }}
        task={task({ results: [{ account_id: "41", error: "读取账号超时" }] })}
      />,
    );

    expect(screen.getByText("失败账号（1）")).toBeVisible();
    expect(screen.getByText("读取账号超时")).toBeVisible();
  });

  it("失败总数大于已保留明细时展示现有记录并说明缺失数量", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={record}
        task={task({
          failed: 10,
          results: [
            { account_id: "41", error: "读取账号超时" },
            { account_id: "52", error: "远程写入被拒绝" },
          ],
        })}
      />,
    );

    expect(screen.getByText("失败账号（10）")).toBeVisible();
    expect(screen.getByText(/已显示 2 项，另 8 项未返回明细/)).toBeVisible();
    expect(screen.getByText("远程写入被拒绝")).toBeVisible();
  });

  it("任务只保留失败总数时明确显示明细缺失而不显示空白列表", () => {
    render(<AutoInspectionHeartbeatDetails record={record} task={task({ failed: 10 })} />);

    expect(screen.getByText(/任务结果未返回逐账号明细/)).toBeVisible();
    expect(screen.queryByRole("list", { name: "自动执行失败账号" })).not.toBeInTheDocument();
  });

  it("账号名称和错误超长时完整换行，失败列表可聚焦并单独纵向滚动", () => {
    const reason = `远程读回失败\n${"request-path/".repeat(60)}`;
    const name = "需要核对调度状态的账号".repeat(20);
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, error: "自动执行部分失败：1 项" }}
        task={task({
          failed: 1,
          results: [{ account_id: "41", account_name: name, error: reason }],
        })}
      />,
    );

    const list = screen.getByRole("list", { name: "自动执行失败账号" });
    expect(list).toHaveAttribute("tabindex", "0");
    expect(list).toHaveClass("max-h-60", "overflow-y-auto");
    expect(screen.getByText(name)).toHaveClass("[overflow-wrap:anywhere]");
    const detail = screen.getByText(/远程读回失败/);
    expect(detail.textContent).toBe(reason);
    expect(detail).toHaveClass("whitespace-pre-wrap", "[overflow-wrap:anywhere]");
  });

  it("返回空结果或非结果对象时不会把成功或跳过的账号判为失败", () => {
    render(
      <AutoInspectionHeartbeatDetails
        record={{ ...record, status: "succeeded", error: null }}
        task={task({
          failed: 0,
          results: [
            null,
            "invalid",
            [],
            { account_id: "41", error: null },
            { account_id: "52", reason: "人工优先位保护", skipped: true },
          ],
        })}
      />,
    );

    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "自动执行失败账号" })).not.toBeInTheDocument();
  });

  it("已完成任务尚未读取时显示忙碌提示，后台刷新保留现有失败记录", () => {
    const view = render(<AutoInspectionHeartbeatDetails record={record} taskLoading />);

    expect(screen.getByRole("status", { name: "正在读取失败明细" })).toBeVisible();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();

    view.rerender(
      <AutoInspectionHeartbeatDetails
        record={record}
        taskLoading
        task={task({ failed: 1, results: [{ account_id: "41", error: "读取账号超时" }] })}
      />,
    );

    expect(screen.getByText("读取账号超时")).toBeVisible();
    expect(screen.queryByRole("status", { name: "正在读取失败明细" })).not.toBeInTheDocument();
  });

  it("历史心跳未关联任务时明确说明无法读取逐项结果", () => {
    render(<AutoInspectionHeartbeatDetails record={{ ...record, task_id: null }} />);

    expect(screen.getByText("本轮未关联任务记录，无法读取逐项结果。")).toBeVisible();
    expect(screen.queryByRole("button", { name: "重新读取" })).not.toBeInTheDocument();
  });
});
