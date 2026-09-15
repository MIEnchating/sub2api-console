import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { WorkbenchTaskResults } from "../components/workbench-task-results";
import { workbenchResultItems } from "../lib/task-results";

afterEach(cleanup);

it("账号检测完成后直接显示中文结论，展开可查看覆盖率和相似度", async () => {
  render(
    <WorkbenchTaskResults
      items={workbenchResultItems({
        items: [
          {
            index: 0,
            name: "参考账号",
            status: "succeeded",
            report: {
              verdict: "SOL_CONSISTENT",
              coverage: { percent: 75 },
              similarity_percent: { sol: 90, luna: 30 },
            },
          },
        ],
      })}
    />,
  );
  const summary = screen.getByRole("button", { name: "Sol 行为一致" });
  summary.focus();
  await userEvent.setup().keyboard("{Enter}");
  expect(screen.getByText("有效覆盖 75%")).toBeVisible();
  expect(screen.getByText("sol 90%")).toBeVisible();
  expect(screen.getByText("luna 30%")).toBeVisible();
});

it("检测报告没有结论时不伪造行为通过状态", () => {
  render(
    <WorkbenchTaskResults
      items={workbenchResultItems({
        items: [{ index: 0, name: "待检测账号", status: "review", report: { status: "failed" } }],
      })}
    />,
  );
  expect(screen.queryByText("Sol 行为一致")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "查看 待检测账号 的报告" })).toBeEnabled();
});
