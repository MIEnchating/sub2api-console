import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  matchesMultiSelectOption,
  MultiSelect,
  MultiSelectClearAction,
  nextMultiSelectValues,
  shouldCloseMultiSelectFromTriggerPress,
  shouldShowMultiSelectSearch,
} from "../multi-select";

describe("MultiSelect", () => {
  it("renders the New API chips control with selected labels", () => {
    const markup = renderToStaticMarkup(
      <MultiSelect
        options={[
          { value: "group-1", label: "codex" },
          { value: "group-2", label: "pro" },
        ]}
        selected={["group-1", "group-2"]}
        onChange={() => undefined}
        title="本地分组"
        searchPlaceholder="搜索本地分组"
        ariaLabel="测试上游 本地分组"
      />,
    );

    expect(markup).toContain('data-slot="combobox-trigger"');
    expect(markup).toContain('role="combobox"');
    expect(markup).toContain('aria-label="测试上游 本地分组"');
    expect(markup).toContain("border");
    expect(markup).not.toContain("border-dashed");
    expect(markup).toContain("codex");
    expect(markup).toContain("pro");
    expect(markup).not.toContain(">group-1<");
    expect(markup).not.toContain(">group-2<");
  });

  it("shows the unselected trigger without a redundant count", () => {
    const markup = renderToStaticMarkup(
      <MultiSelect options={[]} selected={[]} onChange={() => undefined} title="本地分组" />,
    );

    expect(markup).toContain(">本地分组<");
    expect(markup).not.toContain("已选");
  });

  it("renders a footer action for clearing the current filter", () => {
    const markup = renderToStaticMarkup(
      <MultiSelectClearAction text="清空筛选" disabled={false} onClear={() => undefined} />,
    );

    expect(markup).toContain("清空筛选");
    expect(markup).toContain("border-t");
    expect(markup).not.toContain('disabled=""');
  });

  it("supports rich dropdown options without changing selected chip labels", () => {
    const markup = renderToStaticMarkup(
      <MultiSelect
        options={[{ value: "group-1", label: "codex" }]}
        selected={["group-1"]}
        onChange={() => undefined}
        renderOption={(option) => <span>选项：{option.label}</span>}
      />,
    );

    expect(markup).toContain(">codex<");
    expect(markup).not.toContain("选项：codex");
  });

  it("can hide the visible title while preserving the accessible name", () => {
    const markup = renderToStaticMarkup(
      <MultiSelect
        options={[{ value: "1", label: "一组" }]}
        selected={[]}
        onChange={() => undefined}
        title="本地分组"
        showTitle={false}
        ariaLabel="上游分组 本地分组"
      />,
    );

    expect(markup).toContain('aria-label="上游分组 本地分组"');
    expect(markup).not.toContain(">本地分组<");
  });

  it("exposes its disabled state to assistive technology and removes keyboard focus", () => {
    const markup = renderToStaticMarkup(
      <MultiSelect options={[]} selected={[]} onChange={() => undefined} disabled />,
    );

    expect(markup).toContain('aria-disabled="true"');
    expect(markup).toContain('tabindex="-1"');
    expect(markup).toContain('data-disabled=""');
  });

  it("can limit visible chips without discarding selected values", () => {
    const markup = renderToStaticMarkup(
      <MultiSelect
        options={[
          { value: "1", label: "一组" },
          { value: "2", label: "二组" },
          { value: "3", label: "三组" },
        ]}
        selected={["1", "2", "3"]}
        onChange={() => undefined}
        title="本地分组"
        maxVisibleChips={2}
      />,
    );

    expect(markup).toContain("另有 1 个");
    expect(markup).toContain(">一组<");
    expect(markup).toContain(">二组<");
    expect(markup).not.toContain(">三组<");
  });

  it("toggles values without closing or discarding other selections", () => {
    expect(nextMultiSelectValues(["1", "2"], "3")).toEqual(["1", "2", "3"]);
    expect(nextMultiSelectValues(["1", "2"], "1")).toEqual(["2"]);
  });

  it("closes an open dropdown on a repeated primary trigger press", () => {
    expect(shouldCloseMultiSelectFromTriggerPress(true, 0, false)).toBe(true);
    expect(shouldCloseMultiSelectFromTriggerPress(false, 0, false)).toBe(false);
    expect(shouldCloseMultiSelectFromTriggerPress(true, 1, false)).toBe(false);
    expect(shouldCloseMultiSelectFromTriggerPress(true, 0, true)).toBe(false);
  });

  it("replaces the value in single-select mode", () => {
    expect(nextMultiSelectValues(["1"], "2", true)).toEqual(["2"]);
    expect(nextMultiSelectValues(["1"], "1", true)).toEqual([]);
  });

  it("matches search text against both labels and values", () => {
    const option = { value: "group-8", label: "A-kiro逆向" };

    expect(matchesMultiSelectOption(option, "KIRO")).toBe(true);
    expect(matchesMultiSelectOption(option, "group-8")).toBe(true);
    expect(matchesMultiSelectOption(option, "codex")).toBe(false);
  });

  it("shows search only when a multi-select has more than five data items", () => {
    const fiveOptions = Array.from({ length: 5 }, (_, index) => ({
      value: String(index),
      label: `选项 ${index}`,
    }));

    expect(shouldShowMultiSelectSearch(fiveOptions)).toBe(false);
    expect(shouldShowMultiSelectSearch([...fiveOptions, { value: "5", label: "选项 5" }])).toBe(
      true,
    );
  });
});
