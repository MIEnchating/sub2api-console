import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { GroupStatus } from "@/api";
import { render as renderWithDictionaries } from "@/test/dictionary";
import { WorkbenchGroupPicker } from "../components/group-picker";

it("分组顺序变更后按稳定 ID 更新选项，保留同名分组选中状态和未登记分组", async () => {
  vi.stubGlobal("PointerEvent", MouseEvent);
  const onChange = vi.fn();
  const groups = [
    { id: "1", name: "同名" },
    { id: "2", name: "同名" },
    { id: "3", name: "新分组" },
  ] as GroupStatus[];
  const view = renderWithDictionaries(
    <WorkbenchGroupPicker groups={groups} value={["1"]} onChange={onChange} />,
    { group: [{ value: "2" }, { value: "1" }] },
  );
  expect(screen.getAllByRole("checkbox")).toEqual([
    screen.getByRole("checkbox", { name: "同名（ID 2）" }),
    screen.getByRole("checkbox", { name: "同名（ID 1）" }),
    screen.getByRole("checkbox", { name: "新分组（ID 3）" }),
  ]);
  expect(screen.getByRole("checkbox", { name: "同名（ID 1）" })).toBeChecked();
  fireEvent.click(screen.getByRole("checkbox", { name: "同名（ID 2）" }));
  expect(onChange).toHaveBeenCalledWith(["1", "2"]);
  act(() =>
    view.client.setQueryData(["dictionaries", "group"], {
      items: [
        { value: "1", enabled: true },
        { value: "2", enabled: true },
      ],
    }),
  );
  await waitFor(() =>
    expect(screen.getAllByRole("checkbox")[0]).toHaveAccessibleName("同名（ID 1）"),
  );
  view.client.clear();
});
