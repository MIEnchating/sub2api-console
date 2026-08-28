import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AlertListActions } from "../alert-list-actions";

describe("AlertListActions", () => {
  it("disables clearing when no alert records exist", () => {
    const markup = renderToStaticMarkup(
      <AlertListActions
        loading={false}
        failed={false}
        clearableCount={0}
        onClear={() => undefined}
      />,
    );

    expect(markup).not.toContain("0 条");
    expect(markup).toContain("disabled");
    expect(markup).toContain("清理已结束");
  });

  it("keeps clearing available when filtering hides existing records", () => {
    const markup = renderToStaticMarkup(
      <AlertListActions
        loading={false}
        failed={false}
        clearableCount={3}
        onClear={() => undefined}
      />,
    );

    expect(markup).not.toContain("0 条");
    expect(markup.match(/<button[^>]*>/)?.[0]).not.toMatch(/\sdisabled(?:=|\s|>)/);
  });

  it("disables clearing while records are loading", () => {
    const markup = renderToStaticMarkup(
      <AlertListActions loading failed={false} clearableCount={3} onClear={() => undefined} />,
    );

    expect(markup).not.toContain("读取中…");
    expect(markup).toContain("disabled");
  });
});
