import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { LoginPage } from "../../App";

describe("login page", () => {
  it("explains that the previous login expired", () => {
    const markup = renderToStaticMarkup(
      <LoginPage reason="登录已过期，请重新登录" onLogin={() => undefined} />,
    );

    expect(markup).toContain('role="alert"');
    expect(markup).toContain("登录已过期，请重新登录");
  });

  it("does not show an expiry warning for an ordinary logged-out visit", () => {
    const markup = renderToStaticMarkup(<LoginPage onLogin={() => undefined} />);

    expect(markup).not.toContain('role="alert"');
    expect(markup).not.toContain("登录已过期");
  });
});
