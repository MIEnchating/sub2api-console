import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import { ModelCheckSelection } from "../model-check-selection";

it("共同模型超过面板高度时允许滚动到最后一个模型", () => {
  const models = Array.from({ length: 20 }, (_, index) => `model-${index + 1}`);
  render(
    <ModelCheckSelection
      accounts={[]}
      accountsLoading={false}
      accountsError={null}
      accountQuery=""
      selectedAccountIDs={["16"]}
      models={models}
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
  );

  expect(screen.getByRole("list", { name: "可检测模型" })).toHaveClass("h-full", "overflow-auto");
  expect(screen.getByRole("checkbox", { name: /选择模型 model-20\b/ })).toBeEnabled();
});
