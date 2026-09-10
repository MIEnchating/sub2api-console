import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { PageLoadingSkeleton } from "../page-loading-skeleton";

it.each(["table", "form", "list"] as const)(
  "%s 骨架提供加载状态且装饰内容不进入可访问树",
  (variant) => {
    render(<PageLoadingSkeleton label="正在读取页面" variant={variant} />);
    const loading = screen.getByRole("status", { name: "正在读取页面" });
    expect(loading).toHaveAttribute("aria-busy", "true");
    expect(loading).toHaveClass("min-w-0", "w-full");
    for (const placeholder of loading.querySelectorAll('[data-slot="skeleton"]')) {
      expect(placeholder.closest('[aria-hidden="true"]')).not.toBeNull();
    }
    expect(loading.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(1);
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  },
);
