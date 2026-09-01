import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  Select,
  SelectItem,
  SelectTrigger,
  SelectValue,
  selectContentAppearanceLayouts,
  selectContentSearchableByDefault,
  selectOptionMatches,
  selectValueLabel,
} from "../select";

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

  it("searches global select options by visible label or internal value", () => {
    expect(selectOptionMatches("newapi", "New API", "new")).toBe(true);
    expect(selectOptionMatches("group-8", "A-kiro逆向", "KIRO")).toBe(true);
    expect(selectOptionMatches("sub2api", "Sub2API", "codex")).toBe(false);
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
    expect(markup).toContain('data-appearance="classic"');
    expect(markup).not.toContain("border-dashed");
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

  it("renders the ordinary single-select trigger by default", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "all" },
        createElement(SelectTrigger, null, createElement(SelectValue, null, "全部")),
      ),
    );

    expect(markup.match(/<svg/g)).toHaveLength(1);
    expect(markup).toContain("lucide-chevron-down");
    expect(markup).not.toContain("lucide-circle-plus");
    expect(markup).not.toContain("border-dashed");
    expect(markup).toContain("data-popup-open:border-ring");
  });

  it("uses the faceted search style only when explicitly requested", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "all" },
        createElement(
          SelectTrigger,
          { appearance: "faceted" },
          createElement(SelectValue, null, "全部"),
        ),
      ),
    );

    expect(markup).toContain('data-appearance="faceted"');
    expect(markup).toContain("lucide-circle-plus");
    expect(markup).toContain("border");
    expect(markup).not.toContain("border-dashed");
    expect(markup).not.toContain("lucide-chevron-down");
  });

  it("keeps faceted search controls aligned with search inputs at every requested size", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "all" },
        createElement(
          SelectTrigger,
          { appearance: "faceted", size: "sm" },
          createElement(SelectValue, null, "全部"),
        ),
      ),
    );

    expect(markup).toContain("!h-8");
    expect(markup).toContain('data-size="sm"');
  });

  it("makes every ordinary single-select searchable with a fixed search field", () => {
    expect(selectContentSearchableByDefault).toBe(true);
    expect(selectContentAppearanceLayouts.classic).toContain("flex");
    expect(selectContentAppearanceLayouts.classic).toContain("overflow-hidden");
  });

  it("renders the selected ordinary option with a selection indicator", () => {
    const markup = renderToStaticMarkup(
      createElement(
        Select,
        { value: "selected" },
        createElement(SelectItem, { value: "selected" }, "已选择"),
      ),
    );

    expect(markup).toContain('data-selected=""');
    expect(markup).toContain("lucide-check");
  });
});
