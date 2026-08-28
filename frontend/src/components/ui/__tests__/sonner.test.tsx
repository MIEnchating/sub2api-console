import type { ReactElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import type { ToasterProps } from "sonner";
import { describe, expect, it } from "vitest";

import { Toaster } from "../sonner";

describe("global message toaster", () => {
  it("renders at the top center as an overlay container", () => {
    const toaster = Toaster({
      position: "top-center",
      theme: "light",
    }) as ReactElement<ToasterProps>;
    const markup = renderToStaticMarkup(<Toaster position="top-center" theme="light" />);

    expect(toaster.props.position).toBe("top-center");
    expect(markup).toContain('aria-live="polite"');
    expect(markup).toContain('data-react-aria-top-layer="true"');
  });
});
