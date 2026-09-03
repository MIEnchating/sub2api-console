import { describe, expect, it } from "vitest";

import {
  navigationPreferencesStorageKey,
  readHiddenNavigationItemIDs,
  visibleNavigationSections,
  writeHiddenNavigationItemIDs,
} from "../navigation-preferences";

const itemIDs = ["overview", "accounts", "config"] as const;

describe("菜单显示偏好", () => {
  it("读取时忽略未知值、非字符串值和固定显示的系统设置", () => {
    const storage = {
      getItem: () => JSON.stringify(["accounts", "removed-route", 1, "config"]),
    };

    expect([...readHiddenNavigationItemIDs(storage, itemIDs, ["config"])]).toEqual(["accounts"]);
  });

  it("损坏的浏览器设置回退为全部显示", () => {
    const storage = { getItem: () => "not-json" };

    expect(readHiddenNavigationItemIDs(storage, itemIDs, ["config"]).size).toBe(0);
  });

  it("保存时按菜单固定顺序写入且不包含未知值", () => {
    let persistedKey = "";
    let persistedValue = "";
    const storage = {
      setItem: (key: string, value: string) => {
        persistedKey = key;
        persistedValue = value;
      },
    };

    writeHiddenNavigationItemIDs(storage, new Set(["accounts", "overview", "unknown"]), itemIDs);

    expect(persistedKey).toBe(navigationPreferencesStorageKey);
    expect(persistedValue).toBe('["overview","accounts"]');
  });

  it("隐藏菜单项后移除空分组并保持其他分组顺序", () => {
    const sections = [
      { label: "运营", itemIDs: ["overview", "accounts"] },
      { label: "系统", itemIDs: ["config"] },
    ];

    expect(visibleNavigationSections(sections, new Set(["overview", "accounts"]))).toEqual([
      { label: "系统", itemIDs: ["config"] },
    ]);
  });
});
