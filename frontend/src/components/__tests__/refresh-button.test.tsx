import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { RefreshButton } from "../refresh-button";

describe("RefreshButton", () => {
  it("renders one icon-only button with the shared refresh tooltip", () => {
    const markup = renderToStaticMarkup(<RefreshButton ariaLabel="刷新账号池" onClick={vi.fn()} />);

    expect(markup).toContain('aria-label="刷新账号池"');
    expect(markup).toContain("刷新");
    expect(markup).toContain("size-7");
    expect(markup).not.toContain("重新加载");
    expect(markup).not.toContain("重试读取");
  });
});
