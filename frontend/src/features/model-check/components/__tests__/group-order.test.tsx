import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { render as renderWithDictionaries } from "@/test/dictionary";
import { ModelCheckSelection } from "../model-check-selection";

it("常规模型检测的分组筛选按名称字典排序并保留未登记项", async () => {
  renderWithDictionaries(
    <ModelCheckSelection
      accounts={[]}
      accountsLoading={false}
      accountsError={null}
      accountQuery=""
      accountGroups={["A", "B", "新分组"]}
      accountGroup="A"
      selectedAccountIDs={[]}
      models={[]}
      selectedModels={[]}
      modelsLoading={false}
      modelsError={null}
      rounds={1}
      timeoutSeconds={45}
      combinationCount={0}
      selectionError={null}
      disabled={false}
      canSubmit={false}
      onAccountQueryChange={vi.fn()}
      onAccountGroupChange={vi.fn()}
      onAccountToggle={vi.fn()}
      onAccountsSelectAll={vi.fn()}
      onClear={vi.fn()}
      onModelToggle={vi.fn()}
      onModelsSelectAll={vi.fn()}
      onRefreshModels={vi.fn()}
      onRoundsChange={vi.fn()}
      onTimeoutChange={vi.fn()}
      onSubmit={vi.fn()}
    />,
    {
      group: [
        { value: "2", name: "B" },
        { value: "1", name: "A" },
      ],
    },
  );
  fireEvent.click(screen.getByRole("button", { name: "分组筛选" }));
  const options = await screen.findAllByRole("option");
  expect(options.map((item) => item.textContent)).toEqual(["B", "A", "新分组"]);
  expect(options[1]).toHaveAttribute("aria-selected", "true");
});
