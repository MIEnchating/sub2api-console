import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import { AccountSelectionToolbar } from "@/App";

it("账号批量操作进行中保留选择，清空按钮和 Escape 均不可清空", async () => {
  const onClear = vi.fn();
  render(
    <AccountSelectionToolbar
      selectedCount={2}
      pending
      onClear={onClear}
      onProbe={vi.fn()}
      onSyncModels={vi.fn()}
      onDelete={vi.fn()}
    />,
  );
  expect(screen.getByRole("button", { name: "清空选择" })).toBeDisabled();
  screen.getByRole("toolbar").focus();
  await userEvent.setup().keyboard("{Escape}");
  expect(onClear).not.toHaveBeenCalled();
});
