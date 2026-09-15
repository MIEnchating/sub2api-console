import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import { NewAPIChannelForm } from "../channel-form";

it("创建渠道选择本地分组时使用字典顺序并保留新分组", async () => {
  renderWithDictionaries(
    <NewAPIChannelForm
      groups={[
        { id: "1", name: "A", ratio: "1" },
        { id: "2", name: "B", ratio: "2" },
        { id: "3", name: "新分组", ratio: "1" },
      ]}
      newAPIGroups={[]}
      sub2APIBaseURL="https://sub2api.example.test"
      vaultEntries={[]}
      pending={false}
      creatingKey={false}
      fetchingModels={false}
      onCreateKey={vi.fn()}
      onFetchModels={vi.fn()}
      onSubmit={vi.fn()}
    />,
    { group: [{ value: "2" }, { value: "1" }] },
  );
  fireEvent.click(screen.getByRole("combobox", { name: "Sub2API 分组" }));
  expect((await screen.findAllByRole("option")).map((item) => item.textContent)).toEqual([
    "B",
    "A",
    "新分组",
  ]);
});
