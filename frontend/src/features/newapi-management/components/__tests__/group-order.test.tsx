import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import { NewAPIGroupBindings } from "../group-bindings";

it("本地分组字典逆序时绑定选择遵循配置且不绑定固定首位", async () => {
  renderWithDictionaries(
    <NewAPIGroupBindings
      groups={[{ id: "remote", name: "远端", ratio: "1" }]}
      localGroups={[
        { id: "1", name: "A", ratio: "1" },
        { id: "2", name: "B", ratio: "2" },
      ]}
      bindings={[]}
      pending={false}
      onSave={vi.fn()}
    />,
    { group: [{ value: "2" }, { value: "1" }] },
  );
  fireEvent.click(screen.getByRole("combobox", { name: "远端 的 Sub2API 分组" }));
  expect((await screen.findAllByRole("option")).map((item) => item.textContent)).toEqual([
    "不绑定",
    "B · 2",
    "A · 1",
  ]);
});
