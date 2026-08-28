import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Select, SelectTrigger, SelectValue, selectValueLabel } from "../select";

describe("select value labels", () => {
  it.each([
    ["sub2api_user_token", "Token + 刷新 Token"],
    ["newapi_admin_key", "Admin Key + 用户 ID"],
    ["newapi_user_token", "Token"],
    ["sub2api_user_login", "密码箱登录"],
    ["newapi_user_login", "密码箱登录"],
    ["custom_headers", "自定义 Header / Cookie"],
  ])("does not expose authentication enum %s", (value, label) => {
    expect(selectValueLabel(value)).toBe(label);
  });

  it("formats encoded vault selections as a readable label", () => {
    expect(selectValueLabel("credential-vault\u0000大写")).toBe("credential-vault / 大写");
  });

  it.each([
    ["c2c", "私聊"],
    ["group", "群聊"],
    ["channel", "频道"],
  ])("does not expose notification target enum %s", (value, label) => {
    expect(selectValueLabel(value)).toBe(label);
  });

  it("fills the available field width by default", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "sub2api" },
        createElement(SelectTrigger, null, createElement(SelectValue)),
      ),
    );

    expect(markup).toContain("w-full");
    expect(markup).toContain("max-w-full");
    expect(markup).not.toContain("w-fit");
    expect(markup).toContain('data-press-animation="none"');
  });

  it("allows an explicit width for compact composite controls", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "https" },
        createElement(
          SelectTrigger,
          { className: "w-[6.75rem] shrink-0" },
          createElement(SelectValue),
        ),
      ),
    );

    expect(markup).toContain("w-[6.75rem]");
    expect(markup).not.toContain(" w-full");
    expect(markup).not.toContain("w-fit");
  });

  it("renders one down arrow that rotates upward while the popup is open", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "all" },
        createElement(SelectTrigger, null, createElement(SelectValue, null, "全部")),
      ),
    );

    expect(markup.match(/<svg/g)).toHaveLength(1);
    expect(markup).toContain("data-popup-open:rotate-180");
    expect(markup).toContain("data-popup-open:border-ring");
  });
});
