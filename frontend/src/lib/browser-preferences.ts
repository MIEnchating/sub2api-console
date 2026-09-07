// UI preferences are optional; storage restrictions must not prevent login.
export const browserPreferenceStorage: Pick<Storage, "getItem" | "setItem"> = {
  getItem(key: string): string | null {
    try {
      return window.localStorage.getItem(key);
    } catch {
      return null;
    }
  },
  setItem(key: string, value: string): void {
    try {
      window.localStorage.setItem(key, value);
    } catch {
      // Keep the active preference in React state when persistence is unavailable.
    }
  },
};
