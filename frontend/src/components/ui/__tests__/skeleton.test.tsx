import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { Skeleton } from "../skeleton";

it("骨架装饰默认不进入可访问树，动效仅在用户未减少动画时启用", () => {
  render(<Skeleton data-testid="placeholder" className="h-8 w-40" />);
  const placeholder = screen.getByTestId("placeholder");
  expect(placeholder).toHaveAttribute("aria-hidden", "true");
  expect(placeholder).toHaveClass("motion-safe:animate-pulse", "max-w-full", "min-w-0");
  expect(placeholder).not.toHaveClass("animate-pulse");
});
