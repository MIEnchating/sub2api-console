import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { account } from "../../__tests__/fixtures";
import { AccountStateCell } from "../account-pool-cells";

describe("账号列表状态摘要", () => {
  it("错误、停止原因与恢复条件同时存在时优先显示错误，详情仍提供完整原因", () => {
    const failed = {
      ...account,
      schedulable: false,
      sub2api_status: "error",
      sub2api_error: "当前凭据已失效",
      upstream_block: "error",
      upstream_block_reason: "上游账号停止服务",
      recovery: {
        evaluated_at: "2026-09-08T00:00:00Z",
        ready: false,
        conditions: [{ code: "health_score", met: false, detail: "健康分尚未达标" }],
      },
    };
    const view = render(<AccountStateCell account={failed} compact />);
    expect(screen.getByText("最近错误：当前凭据已失效")).toBeVisible();
    expect(screen.getByText("恢复待满足：健康分尚未达标")).toBeVisible();
    expect(screen.queryByText("停止原因：上游账号停止服务")).not.toBeInTheDocument();
    view.rerender(<AccountStateCell account={failed} expanded />);
    expect(screen.getByText("停止原因：上游账号停止服务")).toBeVisible();
  });

  it("状态摘要展示调度开关，健康账号不额外制造警告", () => {
    render(<AccountStateCell account={account} compact />);
    expect(screen.getByText("健康")).toBeVisible();
    expect(screen.getByText("调度开关：已开启")).toBeVisible();
    expect(screen.queryByText(/最近错误|恢复待满足|停止原因/)).not.toBeInTheDocument();
  });
});
