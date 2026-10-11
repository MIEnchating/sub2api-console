import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TableOverflowTooltip } from "../table-overflow-tooltip";

const longText = "很长的账号名称，应该允许键盘查看完整内容";
let availableWidth = 100;
const resizeObservers = new Set<TestResizeObserver>();

class TestResizeObserver {
  readonly elements = new Set<Element>();
  constructor(readonly callback: ResizeObserverCallback) {
    resizeObservers.add(this);
  }
  observe(element: Element): void {
    this.elements.add(element);
  }
  unobserve(element: Element): void {
    this.elements.delete(element);
  }
  disconnect(): void {
    this.elements.clear();
    resizeObservers.delete(this);
  }
}

function resize(element: Element): void {
  act(() => {
    for (const observer of resizeObservers) {
      if (observer.elements.has(element)) {
        observer.callback(
          [{ target: element } as ResizeObserverEntry],
          observer as unknown as ResizeObserver,
        );
      }
    }
  });
}

beforeEach(() => {
  availableWidth = 100;
  vi.stubGlobal("ResizeObserver", TestResizeObserver);
  vi.spyOn(HTMLElement.prototype, "getClientRects").mockReturnValue([
    new DOMRect(0, 0, 100, 20),
  ] as unknown as DOMRectList);
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(() => availableWidth);
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockImplementation(
    function (this: HTMLElement) {
      return this.textContent?.includes("很长") ? 300 : 40;
    },
  );
});

afterEach(() => {
  cleanup();
  resizeObservers.clear();
  vi.restoreAllMocks();
});

describe("表格截断内容的键盘查看", () => {
  it("Tab 跳过完整文本并聚焦截断文本，Escape 关闭说明且保留焦点", async () => {
    const user = userEvent.setup();
    render(
      <>
        <TableOverflowTooltip content="短名称">短名称</TableOverflowTooltip>
        <TableOverflowTooltip content={longText}>{longText}</TableOverflowTooltip>
        <button>行操作</button>
      </>,
    );
    const trigger = screen.getByText(longText);
    await user.tab();
    expect(trigger).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(longText);
    expect(trigger).toHaveAccessibleDescription(longText);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("tooltip")).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "行操作" })).toHaveFocus();
  });

  it("宽度扩大后关闭已打开的说明并移除额外 Tab 停靠点", async () => {
    const user = userEvent.setup();
    render(<TableOverflowTooltip content={longText}>{longText}</TableOverflowTooltip>);
    const trigger = screen.getByText(longText);
    await user.tab();
    expect(await screen.findByRole("tooltip")).toBeVisible();
    availableWidth = 400;
    resize(trigger);
    await waitFor(() => expect(screen.queryByRole("tooltip")).not.toBeInTheDocument());
    expect(trigger).toHaveAttribute("tabindex", "-1");
    expect(trigger).toHaveFocus();
  });

  it("内容从短文本更新为长文本后无需窗口缩放即可键盘查看", async () => {
    const user = userEvent.setup();
    const view = render(<TableOverflowTooltip content="短名称">短名称</TableOverflowTooltip>);
    expect(screen.getByText("短名称").tabIndex).not.toBe(0);
    view.rerender(<TableOverflowTooltip content={longText}>{longText}</TableOverflowTooltip>);
    await user.tab();
    expect(screen.getByText(longText, { selector: '[data-slot="tooltip-trigger"]' })).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(longText);
  });

  it("截断内容内已有按钮时不重复增加停靠点且保留按钮操作", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <TableOverflowTooltip content={longText}>
        <button onClick={onClick}>{longText}</button>
      </TableOverflowTooltip>,
    );
    await user.tab();
    expect(screen.getByRole("button", { name: longText })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("内部按钮退出 Tab 顺序时仍能从外层查看截断文本", async () => {
    const user = userEvent.setup();
    render(
      <TableOverflowTooltip content={longText}>
        <button tabIndex={-1}>{longText}</button>
      </TableOverflowTooltip>,
    );
    await user.tab();
    expect(screen.getByRole("button", { name: longText })).not.toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(longText);
  });

  it("移除其他行后保留行仍随宽度缩小加入键盘顺序", async () => {
    availableWidth = 400;
    const user = userEvent.setup();
    const first = (
      <TableOverflowTooltip key="first" content="第一行">
        第一行
      </TableOverflowTooltip>
    );
    const second = (
      <TableOverflowTooltip key="second" content={longText}>
        {longText}
      </TableOverflowTooltip>
    );
    const view = render(
      <>
        {first}
        {second}
      </>,
    );
    view.rerender(<>{second}</>);
    const trigger = screen.getByText(longText);
    availableWidth = 100;
    resize(trigger);
    await user.tab();
    expect(trigger).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(longText);
  });

  it("浏览器没有 ResizeObserver 时仍通过窗口缩放更新键盘入口", async () => {
    vi.stubGlobal("ResizeObserver", undefined);
    availableWidth = 400;
    render(<TableOverflowTooltip content={longText}>{longText}</TableOverflowTooltip>);
    const trigger = screen.getByText(longText);
    expect(trigger.tabIndex).toBe(-1);
    availableWidth = 100;
    act(() => window.dispatchEvent(new Event("resize")));
    expect(trigger.tabIndex).toBe(0);
  });
});
