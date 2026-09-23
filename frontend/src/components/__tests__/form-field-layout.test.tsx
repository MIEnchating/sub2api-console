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
  it("显式预留错误空间时，校验错误出现和清除均保留字段提示行", () => {
    const view = render(
      <FormField reserveErrorSpace label="账号" error={undefined}>
        <input />
      </FormField>,
    );
    const slot = view.container.querySelector('[data-slot="field-error"]');
    expect(slot).toHaveClass("min-h-8", "shrink-0");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    view.rerender(
      <FormField reserveErrorSpace label="账号" error="请输入账号">
        <input />
      </FormField>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("请输入账号");
    expect(view.container.querySelector('[data-slot="field-error"]')).toBe(slot);
    view.rerender(
      <FormField reserveErrorSpace label="账号" error={undefined}>
        <input />
      </FormField>,
    );
    expect(slot).toBeEmptyDOMElement();
  });

  it("默认校验字段仅在有错误时显示提示，修正后移除空白行", () => {
    const view = render(
      <FormField label="倍率" error={undefined}>
        <input />
      </FormField>,
    );
    expect(view.container.querySelector('[data-slot="field-error"]')).not.toBeInTheDocument();
    view.rerender(
      <FormField label="倍率" error="倍率必须大于 0">
        <input />
      </FormField>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("倍率必须大于 0");
    expect(screen.getByRole("alert")).not.toHaveClass("min-h-8");
    view.rerender(
      <FormField label="倍率" error={undefined}>
        <input />
      </FormField>,
    );
    expect(view.container.querySelector('[data-slot="field-error"]')).not.toBeInTheDocument();
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
      <FormField label="手动控制范围" description="保留优先级 1 至 N；自动调度从 N+1 开始">
        <input type="number" />
      </FormField>,
    );

    expect(markup).toContain('data-slot="field-label"');
    expect(markup).toContain('aria-label="手动控制范围说明"');
    expect(markup).toContain('data-slot="tooltip-trigger"');
    expect(markup).not.toContain("保留优先级 1 至 N；自动调度从 N+1 开始");
  });
});
