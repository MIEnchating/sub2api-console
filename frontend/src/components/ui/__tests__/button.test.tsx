import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Button } from "../button";

describe("Button", () => {
  it("gives the primary command a hover state without moving surrounding controls", () => {
    render(<Button>保存</Button>);
    expect(screen.getByRole("button", { name: "保存" })).toHaveClass(
      "hover:bg-primary/90",
      "transition-colors",
    );
    expect(screen.getByRole("button", { name: "保存" }).className).not.toContain("translate-y");
  });

  it("does not run a disabled command from pointer or keyboard input", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <Button disabled onClick={onClick}>
        保存
      </Button>,
    );
    await user.click(screen.getByRole("button", { name: "保存" }));
    await user.keyboard("{Enter}");
    expect(onClick).not.toHaveBeenCalled();
  });
});

it.each([
  ["outline", "border-border", "bg-background"],
  ["secondary", "border-transparent", "bg-secondary"],
] as const)("%s 辅助操作保持中性色，不冒充业务状态", (variant, border, background) => {
  render(<Button variant={variant}>提示词设置</Button>);
  expect(screen.getByRole("button", { name: "提示词设置" })).toHaveClass(
    border,
    background,
    "text-foreground",
  );
  expect(screen.getByRole("button", { name: "提示词设置" })).not.toHaveClass(
    "text-info",
    "text-success",
    "bg-primary",
  );
});
