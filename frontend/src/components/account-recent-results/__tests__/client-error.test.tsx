import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { AccountRecentResult } from "@/api";
import { AccountRecentResults } from "@/components/account-recent-results";

const clientError: AccountRecentResult = {
  id: "client-error",
  result: "失败",
  event_type: "client_error",
  score: 0,
  observed_at: "2026-09-17T01:00:00Z",
  latency_ms: null,
  failure_reason: "请求参数无效，请检查请求内容",
  source: "traffic",
};

describe("客户端请求错误展示", () => {
  it.each([0, null, undefined])(
    "客户端错误的评分字段为 %s 时显示中立色和不计分说明，不展示分数",
    (score) => {
      render(<AccountRecentResults results={[{ ...clientError, score }]} />);

      const result = screen.getByLabelText(/客户端请求错误（不计入健康评分）/);
      expect(result).toHaveClass("bg-muted-foreground/60");
      expect(result).toHaveAccessibleName(/真实流量.*请求参数无效/);
      expect(result).not.toHaveAccessibleName(/\d+ 分/);
    },
  );

  it("键盘聚焦客户端错误时提示不计入健康评分，旧接口的零分不会出现在提示中", async () => {
    const user = userEvent.setup();
    render(<AccountRecentResults results={[clientError]} />);

    await user.tab();

    expect(screen.getByLabelText(/客户端请求错误（不计入健康评分）/)).toHaveFocus();
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("客户端请求错误（不计入健康评分）");
    expect(tooltip).not.toHaveTextContent(/\d+ 分/);
    expect(within(tooltip).getByText("请求参数无效，请检查请求内容")).toHaveClass(
      "text-muted-foreground",
    );
  });

  it("真实网关失败仍显示橙色和25分，键盘提示保留失败原因", async () => {
    const user = userEvent.setup();
    render(
      <AccountRecentResults
        results={[
          {
            ...clientError,
            id: "gateway-error",
            event_type: "gateway_error",
            score: 25,
            failure_reason: "上游网关错误",
          },
        ]}
      />,
    );

    const result = screen.getByLabelText(/网关错误 · 25 分/);
    expect(result).toHaveClass("bg-orange-500");
    await user.tab();

    expect(result).toHaveFocus();
    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("网关错误 · 25 分");
    expect(within(tooltip).getByText("上游网关错误")).toHaveClass("text-destructive/90");
    expect(tooltip).not.toHaveTextContent("不计入健康评分");
  });
});
