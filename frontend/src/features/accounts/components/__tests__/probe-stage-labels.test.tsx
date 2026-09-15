import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { ProbeProgressSummary } from "../probe-task-timeline";

afterEach(cleanup);

it.each(["__proto__", "constructor", "toString"])(
  "探活阶段为 %s 时概要和展开时间线均显示原文",
  async (stage) => {
    render(
      <ProbeProgressSummary
        steps={[{ stage, status: "succeeded", started_at: "2026-09-14T00:00:00Z" }]}
      />,
    );

    const summary = screen.getByRole("button", { name: `${stage} 已完成` });
    await userEvent.click(summary);

    expect(screen.getByRole("list", { name: "探活过程" })).toHaveTextContent(stage);
    expect(summary).toHaveAttribute("aria-expanded", "true");
  },
);
