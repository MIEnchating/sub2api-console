import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Dialog, DialogContent, DialogHeader, DialogTitle } from "../dialog";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "../sheet";

describe("overlay headers", () => {
  it("wraps a long dialog title and reserves space for its close button", () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{"account-".repeat(40)}</DialogTitle>
          </DialogHeader>
        </DialogContent>
      </Dialog>,
    );

    expect(screen.getByRole("heading")).toHaveClass("min-w-0", "[overflow-wrap:anywhere]");
    expect(screen.getByRole("dialog")).toHaveAttribute("data-close-button", "true");
    expect(screen.getByRole("heading").parentElement).toHaveClass(
      "group-data-[close-button=true]/dialog:pr-8",
    );
    expect(screen.getByRole("button", { name: "关闭" })).toBeEnabled();
  });

  it("does not reserve close-button space when the dialog hides that button", () => {
    render(
      <Dialog open>
        <DialogContent showCloseButton={false}>
          <DialogTitle>任务执行中</DialogTitle>
        </DialogContent>
      </Dialog>,
    );

    expect(screen.getByRole("dialog")).toHaveAttribute("data-close-button", "false");
    expect(screen.queryByRole("button", { name: "关闭" })).not.toBeInTheDocument();
  });

  it("keeps a bottom sheet scrollable within the viewport and wraps its title", () => {
    render(
      <Sheet open>
        <SheetContent side="bottom">
          <SheetHeader>
            <SheetTitle>{"upstream-".repeat(40)}</SheetTitle>
          </SheetHeader>
        </SheetContent>
      </Sheet>,
    );

    expect(screen.getByRole("dialog")).toHaveClass("max-h-dvh", "overflow-y-auto");
    expect(screen.getByRole("heading")).toHaveClass("min-w-0", "[overflow-wrap:anywhere]");
    expect(screen.getByRole("heading").parentElement).toHaveClass(
      "group-data-[close-button=true]/sheet:pr-12",
    );
  });

  it("moves focus into a sheet when it opens for keyboard users", async () => {
    render(
      <Sheet open>
        <SheetContent>
          <SheetTitle>编辑上游</SheetTitle>
          <input aria-label="上游名称" />
        </SheetContent>
      </Sheet>,
    );

    await waitFor(() => expect(screen.getByRole("textbox", { name: "上游名称" })).toHaveFocus());
  });
});
