import { StrictMode } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useThemePreference } from "../use-theme-preference";
import { themePreferenceKey } from "@/lib/theme-preference";

let themeColor: HTMLMetaElement;
beforeEach(() => {
  themeColor = document.createElement("meta");
  themeColor.name = "theme-color";
  document.head.append(themeColor);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  themeColor.remove();
  document.documentElement.classList.remove("dark");
  delete document.documentElement.dataset.theme;
  document.documentElement.style.removeProperty("color-scheme");
});

function Harness() {
  const preference = useThemePreference();
  return (
    <button onClick={preference.toggleTheme}>
      {preference.theme === "dark" ? "切换亮色主题" : "切换暗色主题"}
    </button>
  );
}

it.each([null, "invalid", "dark", "light"])("保存值为%s时应用合法主题且挂载不改写存储", (saved) => {
  if (saved !== null) localStorage.setItem(themePreferenceKey, saved);
  render(
    <StrictMode>
      <Harness />
    </StrictMode>,
  );
  const expected = saved === "light" ? "light" : "dark";
  expect(document.documentElement).toHaveAttribute("data-theme", expected);
  expect(document.documentElement.style.colorScheme).toBe(expected);
  expect(themeColor.content).toBe(expected === "light" ? "#ffffff" : "#1e1e1e");
  expect(localStorage.getItem(themePreferenceKey)).toBe(saved);
});

it("本地配额不足时切换仍更新当前界面，持久化失败不覆盖旧设置", async () => {
  localStorage.setItem(themePreferenceKey, "dark");
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("配额不足", "QuotaExceededError");
  });
  render(<Harness />);
  await userEvent.setup().click(screen.getByRole("button", { name: "切换亮色主题" }));
  expect(screen.getByRole("button", { name: "切换暗色主题" })).toBeVisible();
  expect(document.documentElement).toHaveAttribute("data-theme", "light");
  expect(localStorage.getItem(themePreferenceKey)).toBe("dark");
});

it("其他偏好或会话存储变更不会修改当前主题", () => {
  render(<Harness />);
  act(() => {
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "other-preference",
        newValue: "light",
        storageArea: localStorage,
      }),
    );
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: themePreferenceKey,
        newValue: "light",
        storageArea: sessionStorage,
      }),
    );
  });
  expect(screen.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
});

it("迟到的跨标签事件按最新存储更新主题，不回放事件中的旧值", () => {
  render(<Harness />);
  localStorage.setItem(themePreferenceKey, "light");
  act(() =>
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: themePreferenceKey,
        newValue: "dark",
        storageArea: localStorage,
      }),
    ),
  );
  expect(screen.getByRole("button", { name: "切换暗色主题" })).toBeVisible();
  expect(localStorage.getItem(themePreferenceKey)).toBe("light");
});

it("其他标签清空全部偏好后回退默认深色，卸载后不再处理存储事件", () => {
  localStorage.setItem(themePreferenceKey, "light");
  const view = render(<Harness />);
  localStorage.clear();
  act(() =>
    window.dispatchEvent(new StorageEvent("storage", { key: null, storageArea: localStorage })),
  );
  expect(screen.getByRole("button", { name: "切换亮色主题" })).toBeVisible();
  expect(localStorage.getItem(themePreferenceKey)).toBeNull();
  view.unmount();
  const read = vi.spyOn(Storage.prototype, "getItem");
  act(() =>
    window.dispatchEvent(
      new StorageEvent("storage", { key: themePreferenceKey, storageArea: localStorage }),
    ),
  );
  expect(read).not.toHaveBeenCalled();
});
