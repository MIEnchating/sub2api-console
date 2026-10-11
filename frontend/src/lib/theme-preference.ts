import { browserPreferenceStorage } from "./browser-preferences";

export type ThemePreference = "light" | "dark";
export const themePreferenceKey = "sub2api-console-theme";

export function readThemePreference(): ThemePreference {
  return browserPreferenceStorage.getItem(themePreferenceKey) === "light" ? "light" : "dark";
}

export function applyThemePreference(theme: ThemePreference): void {
  const root = document.documentElement;
  root.dataset.theme = theme;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme;
  document
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute("content", theme === "light" ? "#ffffff" : "#1e1e1e");
}
