import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Dialog, DialogContent, DialogTitle } from "../dialog";

describe("Dialog focus", () => {
  it("moves focus into the dialog when it opens", async () => {
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>编辑账号</DialogTitle>
          <label>
            账号名称
            <input aria-label="账号名称" />
          </label>
        </DialogContent>
      </Dialog>,
    );

    await waitFor(() => expect(screen.getByRole("textbox", { name: "账号名称" })).toHaveFocus());
    expect(screen.getByRole("dialog", { name: "编辑账号" })).toContainElement(
      document.activeElement as HTMLElement,
    );
  });
});
