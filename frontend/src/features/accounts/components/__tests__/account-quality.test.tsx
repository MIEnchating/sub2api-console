import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { account } from "../../__tests__/fixtures";
import { AccountQualityCell } from "../account-quality-cell";

afterEach(cleanup);
it("无检测和稳定性样本时显示暂无样本而非零分或满分", () => {
  render(<AccountQualityCell account={account} />);
  expect(screen.getByText("未检测")).toBeVisible();
  expect(screen.getAllByText("暂无样本")).toHaveLength(4);
});
it("置信度接口失败时明确标记读取失败并保留稳定性", () => {
  render(
    <AccountQualityCell
      account={{
        ...account,
        model_check_status: "unavailable",
        stability: {
          evaluated_at: "2026-09-18T00:00:00Z",
          short: { score: 0, samples: 1, passed: 0, failed: 1, inconclusive: 0 },
          long: { score: 50, samples: 2, passed: 1, failed: 1, inconclusive: 0 },
        },
      }}
    />,
  );
  expect(screen.getAllByText("读取失败")).toHaveLength(2);
  expect(screen.getByLabelText("稳定性短期：0.0%")).toBeVisible();
  expect(screen.getByLabelText("稳定性长期：50.0%")).toBeVisible();
});
it("键盘聚焦置信度可查看窗口、样本量与计算口径", async () => {
  const user = userEvent.setup();
  render(
    <TooltipProvider delay={0}>
      <AccountQualityCell
        account={{
          ...account,
          model_check_status: "consistent",
          model_check: {
            account_id: account.id,
            status: "consistent",
            checked_at: "2026-09-18T00:00:00Z",
            task_id: "check-1",
            confidence: {
              evaluated_at: "2026-09-18T00:00:00Z",
              short: { score: 20.7, samples: 2, passed: 1, failed: 0, inconclusive: 1 },
              long: { score: 50, samples: 8, passed: 6, failed: 2, inconclusive: 0 },
            },
          },
        }}
      />
    </TooltipProvider>,
  );
  await user.tab();
  expect(await screen.findByRole("tooltip")).toHaveTextContent("95% Wilson 下界");
  expect(screen.getByRole("tooltip")).toHaveTextContent(
    "短期 24 小时：2 个样本，1 个通过，0 个失败，1 个无结论",
  );
  expect(screen.getByLabelText("置信度短期：20.7%")).toBeVisible();
});
