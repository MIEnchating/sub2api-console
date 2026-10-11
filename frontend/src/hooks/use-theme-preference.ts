import { useCallback, useEffect, useLayoutEffect, useState } from "react";
import { browserPreferenceStorage } from "@/lib/browser-preferences";
import {
  applyThemePreference,
  readThemePreference,
  themePreferenceKey,
  type ThemePreference,
} from "@/lib/theme-preference";

export function useThemePreference(): { theme: ThemePreference; toggleTheme: () => void } {
  const [theme, setTheme] = useState(readThemePreference);
  useLayoutEffect(() => applyThemePreference(theme), [theme]);
  useEffect(() => {
    const onStorage = (event: StorageEvent): void => {
      if (event.key !== null && event.key !== themePreferenceKey) return;
      try {
        if (event.storageArea !== window.localStorage) return;
      } catch {
        return;
      }
      // Read the current value: an older queued event may predate a local change.
      setTheme(readThemePreference());
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);
  const toggleTheme = useCallback(() => {
    const next = theme === "dark" ? "light" : "dark";
    browserPreferenceStorage.setItem(themePreferenceKey, next);
    setTheme(next);
  }, [theme]);
  return { theme, toggleTheme };
}
