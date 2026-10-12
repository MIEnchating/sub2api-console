import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { StatusBadge } from "../status-badge";
import { render, screen } from "@testing-library/react";
import { Activity } from "lucide-react";

describe("StatusBadge", () => {
  it("forwards live-region attributes and keeps its decorative icon hidden", () => {
    render(
      <StatusBadge
        label="正在同步"
        role="status"
        aria-live="polite"
        id="sync-status"
        icon={Activity}
      />,
    );
    expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");
    expect(screen.getByRole("status")).toHaveAttribute("id", "sync-status");
    expect(screen.getByRole("status").querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });
  it("uses the shared status slot and semantic tone with a shared subtle background", () => {
    const markup = renderToStaticMarkup(<StatusBadge label="开启" variant="success" />);

    expect(markup).toContain('data-slot="status-badge"');
    expect(markup).toContain("text-success");
    expect(markup).toContain("bg-success/10");
    expect(markup).toContain("border-success/30");
  });

  it("converts its compatibility title into the shared tooltip", () => {
    const markup = renderToStaticMarkup(
      <StatusBadge label="待执行" variant="warning" title="等待上一任务完成" />,
    );

    expect(markup).toContain("data-base-ui-tooltip-trigger");
    expect(markup).not.toContain('title="等待上一任务完成"');
  });
});

it("长状态标签保持统一高度、字号和截断策略", () => {
  const label = "需要确认的超长状态".repeat(20);
  render(<StatusBadge label={label} role="status" />);
  expect(screen.getByRole("status")).toHaveClass(
    "h-5",
    "text-xs",
    "max-w-full",
    "bg-muted/30",
    "text-muted-foreground",
  );
  expect(screen.getByText(label)).toHaveClass("truncate", "min-w-0");
});
