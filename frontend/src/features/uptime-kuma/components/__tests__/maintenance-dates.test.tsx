import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import type { KumaResource } from "@/api";
import { resourceDefaults } from "../../lib/resource-schemas";
import { ResourceDialog } from "../resource-dialog";
import { statusOptions } from "./status-page-fixtures";

function renderMaintenance(): ReturnType<typeof vi.fn> {
  const submit = vi.fn();
  const item: KumaResource = {
    id: 1,
    name: "每月维护",
    type: "recurring-day-of-month",
    active: true,
    revision: "1",
    association_revision: "1",
    maintenance: {
      ...resourceDefaults("maintenance").maintenance,
      title: "每月维护",
      strategy: "recurring-day-of-month",
    },
  };
  render(
    <ResourceDialog
      kind="maintenance"
      item={item}
      options={statusOptions}
      pending={false}
      error={null}
      onClose={vi.fn()}
      onSubmit={submit}
    />,
  );
  return submit;
}

it("连续输入多个每月日期时保留输入文本并提交准确日期", async () => {
  const user = userEvent.setup();
  const submit = renderMaintenance();
  const input = screen.getByRole("textbox", { name: "每月日期（逗号分隔）" });
  await user.type(input, "1,15,31");
  expect(input).toHaveValue("1,15,31");
  await user.click(screen.getByRole("button", { name: "保存" }));
  await waitFor(() =>
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        maintenance: expect.objectContaining({ days_of_month: [1, 15, 31] }),
      }),
      expect.anything(),
    ),
  );
});

it.each(["1,", "0", "32", "1.5", "abc"])(
  "每月日期为 %s 时显示字段错误并阻止提交",
  async (value) => {
    const submit = renderMaintenance();
    const input = screen.getByRole("textbox", { name: "每月日期（逗号分隔）" });
    fireEvent.change(input, {
      target: { value },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("日期");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(submit).not.toHaveBeenCalled();
  },
);
