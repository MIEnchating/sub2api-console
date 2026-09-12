import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { ModelCheckSelection } from "../model-check-selection";

function SearchSelection(props: { onClear: () => void; loading?: boolean; error?: string }) {
  const [query, setQuery] = useState("");
  return (
    <ModelCheckSelection
      accounts={[]}
      accountsLoading={props.loading ?? false}
      accountsError={props.error ?? null}
      accountQuery={query}
      selectedAccountIDs={["16"]}
      models={["gpt-5.6-sol"]}
      selectedModels={["gpt-5.6-sol"]}
      modelsLoading={false}
      modelsError={null}
      rounds={1}
      timeoutSeconds={45}
      combinationCount={1}
      selectionError={null}
      disabled={false}
      canSubmit
      onAccountQueryChange={setQuery}
      onAccountToggle={() => undefined}
      onAccountsSelectAll={() => undefined}
      onClear={props.onClear}
      onModelToggle={() => undefined}
      onModelsSelectAll={() => undefined}
      onRefreshModels={() => undefined}
      onRoundsChange={() => undefined}
      onTimeoutChange={() => undefined}
      onSubmit={() => undefined}
    />
  );
}

describe("账号搜索工具栏", () => {
  it("初始搜索为空时提供完整搜索范围且隐藏清除搜索入口", () => {
    render(<SearchSelection onClear={vi.fn()} />);

    expect(screen.getByRole("textbox", { name: "搜索账号、ID、平台或 Host" })).toHaveValue("");
    expect(screen.queryByRole("button", { name: "清除搜索" })).not.toBeInTheDocument();
  });

  it("输入无匹配关键词时显示零个匹配和空结果提示", async () => {
    const user = userEvent.setup();
    render(<SearchSelection onClear={vi.fn()} />);

    await user.type(screen.getByRole("textbox", { name: /搜索账号/ }), "不存在的账号");

    expect(screen.getByText("匹配 0 个")).toHaveAttribute("role", "status");
    expect(screen.getByText("没有匹配的账号")).toBeInTheDocument();
  });

  it("键盘清除搜索后输入框恢复焦点并保留已有选择", async () => {
    const user = userEvent.setup();
    const onClear = vi.fn();
    render(<SearchSelection onClear={onClear} />);
    const input = screen.getByRole("textbox", { name: /搜索账号/ });
    await user.type(input, "openai");
    await user.tab();
    expect(screen.getByRole("button", { name: "清除搜索" })).toHaveFocus();

    await user.keyboard("{Enter}");

    expect(input).toHaveValue("");
    expect(input).toHaveFocus();
    expect(screen.queryByRole("button", { name: "清除搜索" })).not.toBeInTheDocument();
    expect(screen.getByText("共同模型 · 已选 1/20")).toBeInTheDocument();
    expect(onClear).not.toHaveBeenCalled();
  });

  it.each([
    { loading: true, error: undefined },
    { loading: false, error: "读取账号失败" },
  ])("账号尚不可用时不报告零匹配（%j）", async (state) => {
    const user = userEvent.setup();
    render(<SearchSelection onClear={vi.fn()} {...state} />);

    await user.type(screen.getByRole("textbox", { name: /搜索账号/ }), "openai");

    expect(screen.queryByText("匹配 0 个")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "清除搜索" })).toBeEnabled();
  });

  it("窄屏搜索与操作纵向排列且宽屏恢复横向排列", () => {
    render(<SearchSelection onClear={vi.fn()} />);

    expect(screen.getByRole("group", { name: "账号搜索与选择" })).toHaveClass(
      "flex-col",
      "sm:flex-row",
    );
  });
});
