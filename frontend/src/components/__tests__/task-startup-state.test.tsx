import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  TaskProgressState,
  TaskStartupState,
  taskStartupStateLayout,
} from "../task-startup-state";

describe("task startup state", () => {
  it("shows immediate task creation feedback with a progress track", () => {
    const markup = renderToStaticMarkup(
      <TaskStartupState message="正在创建余额同步任务" />,
    );

    expect(markup).toContain("正在创建余额同步任务");
    expect(markup).toContain("0%");
    expect(markup).toContain('role="status"');
    expect(markup).toContain('role="progressbar"');
  });

  it("reserves stable content height before the first task response arrives", () => {
    expect(taskStartupStateLayout.root).toContain("min-h-12");
    expect(taskStartupStateLayout.root).toContain("gap-3");
    expect(taskStartupStateLayout.heading).toContain("min-w-0");
  });

  it("uses the same layout for running task progress", () => {
    const markup = renderToStaticMarkup(
      <TaskProgressState message="正在校验账号" progress={46} />,
    );

    expect(markup).toContain("正在校验账号");
    expect(markup).toContain("46%");
    expect(markup).toContain(taskStartupStateLayout.root);
    expect(markup).toContain('aria-valuenow="46"');
  });
});
