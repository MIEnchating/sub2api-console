import { describe, expect, it } from "vitest";

import {
  compactOperationDialogLayout,
  dialogBodyLayout,
  dialogContentClass,
  dialogHeightLayouts,
  dialogContentLayout,
  dialogWidthLayouts,
  operationDialogHeight,
  operationDialogWidth,
} from "../dialog";

describe("DialogContent layout", () => {
  it("sizes from content while staying inside the viewport", () => {
    expect(dialogContentLayout).toContain("w-fit");
    expect(dialogContentLayout).toContain("min-w-[min(20rem,calc(100%-2rem))]");
    expect(dialogContentLayout).toContain("max-w-[calc(100%-2rem)]");
    expect(dialogContentLayout).not.toContain("w-full");
    expect(dialogContentLayout).not.toContain("sm:max-w-sm");
  });

  it("keeps content dialogs adaptive and reserves stable widths for result dialogs", () => {
    expect(dialogWidthLayouts.content).toBe("");
    expect(dialogWidthLayouts.medium).toContain("32rem");
    expect(dialogWidthLayouts.wide).toContain("64rem");
    expect(dialogWidthLayouts.table).toContain("90rem");
    expect(dialogWidthLayouts.wide).toContain("100vw-2rem");
    expect(dialogWidthLayouts.table).toContain("100vw-2rem");
    expect(dialogHeightLayouts.content).toBe("");
    expect(dialogHeightLayouts.adaptive).toContain("max-h-[calc(100svh-2rem)]");
    expect(dialogHeightLayouts.medium).toContain("32rem");
    expect(dialogHeightLayouts.large).toContain("38rem");
    expect(dialogHeightLayouts.tall).toContain("44rem");
    expect(dialogHeightLayouts.medium).toContain("max-h-");
    expect(dialogHeightLayouts.large).toContain("max-h-");
    expect(dialogHeightLayouts.tall).toContain("max-h-");
    expect(dialogHeightLayouts.medium).not.toMatch(/^h-\[/);
    expect(dialogHeightLayouts.large).not.toMatch(/^h-\[/);
    expect(dialogHeightLayouts.tall).not.toMatch(/^h-\[/);
    expect(dialogContentClass()).toContain("w-fit");
    expect(dialogContentClass("table", "tall")).toContain("w-[min(90rem,calc(100vw-2rem))]");
    expect(dialogContentClass("table", "tall")).toContain("max-h-[min(44rem,calc(100svh-2rem))]");
    expect(dialogContentClass("table", "tall")).not.toContain("w-fit");
    expect(operationDialogWidth(false, "table")).toBe("medium");
    expect(operationDialogWidth(true, "table")).toBe("table");
    expect(operationDialogWidth(true)).toBe("wide");
    expect(operationDialogHeight(false)).toBe("content");
    expect(operationDialogHeight(true)).toBe("adaptive");
    expect(compactOperationDialogLayout).toEqual({ width: "medium", height: "adaptive" });
    expect(dialogContentClass("medium", "adaptive")).toContain("max-h-[calc(100svh-2rem)]");
    expect(dialogContentClass("medium", "adaptive")).not.toContain("h-[min(");
  });

  it("keeps complex dialog scrolling inside the shared body region", () => {
    expect(dialogBodyLayout).toContain("min-h-0");
    expect(dialogBodyLayout).toContain("min-w-0");
    expect(dialogBodyLayout).toContain("overflow-x-hidden");
    expect(dialogBodyLayout).toContain("overflow-y-auto");
    expect(dialogBodyLayout).toContain("overscroll-contain");
  });
});
