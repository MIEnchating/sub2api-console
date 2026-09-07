import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Textarea } from "../textarea";

describe("Textarea layout", () => {
  it("limits manual resizing to the vertical axis inside narrow forms", () => {
    render(<Textarea aria-label="自定义请求头" />);
    expect(screen.getByRole("textbox")).toHaveClass("min-w-0", "resize-y", "overflow-y-auto");
  });

  it("keeps automatic growth non-resizable when requested", () => {
    render(<Textarea aria-label="任务结果" autoGrow />);
    expect(screen.getByRole("textbox")).toHaveClass("resize-none", "overflow-hidden");
    expect(screen.getByRole("textbox")).not.toHaveClass("resize-y");
  });
});
