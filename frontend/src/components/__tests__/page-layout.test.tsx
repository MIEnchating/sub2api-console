import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PageHeading } from "../page-heading";
import { PageLayout } from "../page-layout";

describe("PageLayout", () => {
  it("长表单滚动时标题、操作和分类导航保留在滚动区之外", () => {
    render(
      <PageLayout navigation={<nav aria-label="分类">分类导航</nav>}>
        <PageHeading eyebrow="" title="调度策略" description="" action={<button>保存</button>} />
        <section aria-label="策略表单">设置</section>
      </PageLayout>,
    );
    const content = screen
      .getByRole("region", { name: "策略表单" })
      .closest('[data-slot="page-content"]');
    expect(content).toHaveClass("min-h-0", "min-w-0", "overflow-auto");
    for (const element of [
      screen.getByRole("heading"),
      screen.getByRole("button"),
      screen.getByRole("navigation"),
    ]) {
      expect(content).not.toContainElement(element);
    }
    expect(screen.getByRole("navigation").closest('[data-slot="page-navigation"]')).toHaveClass(
      "shrink-0",
    );
  });

  it("固定面板在低高度窗口保留列表可用高度，并允许外围滚动到达底部", () => {
    render(
      <PageLayout fixedContent>
        <PageHeading eyebrow="" title="系统信息" description="" />
        <section aria-label="任务列表">任务</section>
      </PageLayout>,
    );
    const list = screen.getByRole("region", { name: "任务列表" });
    expect(list.closest('[data-slot="page-workspace"]')).toHaveClass(
      "h-full",
      "min-h-[32rem]",
      "sm:min-h-[28rem]",
    );
    expect(list.closest('[data-slot="page-content"]')).toHaveClass("overflow-auto");
    expect(screen.getByRole("heading").closest('[data-slot="page-workspace"]')).toBeNull();
  });

  it("普通表单不强制最小工作区高度或添加空导航间距", () => {
    const result = render(
      <PageLayout>
        <p>短表单</p>
      </PageLayout>,
    );
    expect(result.container.querySelector('[data-slot="page-workspace"]')).toBeNull();
    expect(result.container.querySelector('[data-slot="page-navigation"]')).toBeNull();
  });
});
