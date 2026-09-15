import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import type { GroupStatus } from "@/api";
import { LogsFilterToolbar } from "../logs-filter-toolbar";

it("事件分组筛选按稳定 ID 消费字典逆序", async () => {
  renderWithDictionaries(
    <LogsFilterToolbar
      search=""
      kind="event"
      state="all"
      eventLevel="all"
      eventGroup="all"
      groups={
        [
          { id: "1", name: "A" },
          { id: "2", name: "B" },
        ] as GroupStatus[]
      }
      truncated={false}
      onSearchChange={vi.fn()}
      onKindChange={vi.fn()}
      onStateChange={vi.fn()}
      onEventLevelChange={vi.fn()}
      onEventGroupChange={vi.fn()}
    />,
    { group: [{ value: "2" }, { value: "1" }] },
  );
  fireEvent.click(screen.getByRole("button", { name: "事件分组筛选" }));
  expect((await screen.findAllByRole("option")).map((item) => item.textContent)).toEqual([
    "B",
    "A",
  ]);
});
