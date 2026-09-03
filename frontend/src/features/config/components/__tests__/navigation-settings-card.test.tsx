import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { NavigationSettingsCard } from "../navigation-settings-card";

const sections = [
  {
    label: "运营管理",
    items: [
      { id: "overview", label: "运营总览", path: "/" },
      { id: "accounts", label: "账号管理", path: "/accounts" },
    ],
  },
  {
    label: "系统管理",
    items: [{ id: "config", label: "系统设置", path: "/config" }],
  },
];

function renderCard(hiddenItemIDs: ReadonlySet<string>): string {
  return renderToStaticMarkup(
    <NavigationSettingsCard
      sections={sections}
      hiddenItemIDs={hiddenItemIDs}
      lockedItemIDs={new Set(["config"])}
      onItemVisibilityChange={() => undefined}
      onReset={() => undefined}
    />,
  );
}

describe("菜单设置卡片", () => {
  it("按路由分组展示菜单开关和当前显示数量", () => {
    const markup = renderCard(new Set(["accounts"]));

    expect(markup).toContain("菜单设置");
    expect(markup).toContain("当前显示 2 / 3 个菜单入口");
    expect(markup).toContain("运营管理");
    expect(markup).toContain("系统管理");
    expect(markup).toContain('aria-label="在菜单中显示运营总览"');
    expect(markup).toMatch(
      /<span(?=[^>]*role="switch")(?=[^>]*aria-label="在菜单中显示账号管理")(?=[^>]*aria-checked="false")[^>]*>/,
    );
    expect(markup).not.toMatch(/<button(?=[^>]*>恢复默认<\/button>)(?=[^>]*disabled)/);
  });

  it("系统设置固定显示且默认状态不能执行恢复", () => {
    const markup = renderCard(new Set());

    expect(markup).toContain("/config · 始终显示");
    expect(markup).toMatch(
      /<span(?=[^>]*role="switch")(?=[^>]*aria-label="在菜单中显示系统设置")(?=[^>]*aria-disabled="true")[^>]*>/,
    );
    expect(markup).toMatch(/<button(?=[^>]*disabled)[^>]*>[\s\S]*?恢复默认<\/button>/);
  });
});
