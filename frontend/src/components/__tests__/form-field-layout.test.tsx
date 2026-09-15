import { renderToStaticMarkup } from "react-dom/server";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { FormField } from "../../App";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";

describe("form field layout", () => {
  it("未配置校验的展示字段不额外添加提示占位", () => {
    const view = render(
      <FormField label="平台">
        <input />
      </FormField>,
    );
    expect(view.container.querySelector('[data-slot="field-error"]')).not.toBeInTheDocument();
  });
  it("校验错误出现和清除时始终保留字段提示行", () => {
    const view = render(
      <FormField label="账号" error={undefined}>
        <input />
      </FormField>,
    );
    const slot = view.container.querySelector('[data-slot="field-error"]');
    expect(slot).toHaveClass("min-h-8", "shrink-0");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    view.rerender(
      <FormField label="账号" error="请输入账号">
        <input />
      </FormField>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("请输入账号");
    expect(view.container.querySelector('[data-slot="field-error"]')).toBe(slot);
    view.rerender(
      <FormField label="账号" error={undefined}>
        <input />
      </FormField>,
    );
    expect(slot).toBeEmptyDOMElement();
  });

  it("does not make the whole field row an implicit select click target", () => {
    const markup = renderToStaticMarkup(
      <FormField label="平台">
        <Select value="sub2api">
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="sub2api">Sub2API</SelectItem>
          </SelectContent>
        </Select>
      </FormField>,
    );

    expect(markup).toMatch(/<label[^>]*>平台<\/label>/);
    expect(markup).not.toMatch(/<label[^>]*>.*data-slot="select-trigger".*<\/label>/);
    expect(markup).toContain('data-slot="select-trigger"');
  });

  it("shows field descriptions through an accessible help tooltip", () => {
    const markup = renderToStaticMarkup(
      <FormField label="人工优先位范围" description="保留优先级 1 至 N；自动调度从 N+1 开始">
        <input type="number" />
      </FormField>,
    );

    expect(markup).toContain('data-slot="field-label"');
    expect(markup).toContain('aria-label="人工优先位范围说明"');
    expect(markup).toContain('data-slot="tooltip-trigger"');
    expect(markup).not.toContain("保留优先级 1 至 N；自动调度从 N+1 开始");
  });
});
