import { Pencil, Trash2 } from "lucide-react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TableActionButton } from "../table-action-button";

describe("TableActionButton", () => {
  it("uses the shared bordered icon style and accessible action label", () => {
    const markup = renderToStaticMarkup(
      <TableActionButton label="编辑分组">
        <Pencil />
      </TableActionButton>,
    );

    expect(markup).toContain('aria-label="编辑分组"');
    expect(markup).toContain("size-8");
    expect(markup).toContain("border-border");
    expect(markup).toContain("bg-background");
    expect(markup).toContain("hover:bg-muted");
    expect(markup).not.toContain("title=");
  });

  it("uses a consistent danger treatment for destructive row actions", () => {
    const markup = renderToStaticMarkup(
      <TableActionButton label="删除凭据" tone="danger">
        <Trash2 />
      </TableActionButton>,
    );

    expect(markup).toContain("text-destructive");
    expect(markup).toContain("hover:bg-destructive/10");
  });

  it("allows row context in the accessible name without replacing the action label", () => {
    const markup = renderToStaticMarkup(
      <TableActionButton label="编辑" ariaLabel="编辑分组 codex">
        <Pencil />
      </TableActionButton>,
    );

    expect(markup).toContain('aria-label="编辑分组 codex"');
    expect(markup).not.toContain('aria-label="编辑"');
  });
});
