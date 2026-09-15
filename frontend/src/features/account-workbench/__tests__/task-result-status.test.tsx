import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { WorkbenchTaskResults } from "../components/workbench-task-results";
import { workbenchResultItems } from "../lib/task-results";

afterEach(cleanup);

it.each(["__proto__", "constructor", "toString"])(
  "任务返回非约定状态 %s 时保留原文并正常显示结果",
  (status) => {
    render(
      <WorkbenchTaskResults
        items={workbenchResultItems({ items: [{ index: 0, name: "导入账号", status }] })}
      />,
    );

    expect(screen.getByRole("list", { name: "账号处理结果" })).toHaveTextContent(status);
  },
);

it("任务返回已知失败状态时显示约定中文状态", () => {
  render(
    <WorkbenchTaskResults
      items={workbenchResultItems({
        items: [{ index: 0, name: "导入账号", status: "failed" }],
      })}
    />,
  );

  expect(screen.getByRole("list", { name: "账号处理结果" })).toHaveTextContent("处理失败");
});
