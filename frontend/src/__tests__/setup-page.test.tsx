import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { SetupPage } from "../App";
import type { SetupStatus } from "../api";

function renderSetup(status?: SetupStatus): string {
  return renderToStaticMarkup(<SetupPage status={status} onComplete={() => undefined} />);
}

describe("SetupPage", () => {
  it("shows a required setup token only when the server requests it", () => {
    const required = renderSetup({
      initialized: false,
      target_configured: true,
      setup_token_required: true,
    });
    const notRequired = renderSetup({
      initialized: false,
      target_configured: true,
      setup_token_required: false,
    });

    expect(required).toContain("初始化令牌");
    expect(required).toMatch(
      /<input(?=[^>]*type="password")(?=[^>]*required)(?=[^>]*aria-label="初始化令牌")[^>]*>/,
    );
    expect(notRequired).not.toContain("初始化令牌");
    expect(renderSetup()).not.toContain("初始化令牌");
  });
});
