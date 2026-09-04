import { createContext, useContext } from "react";

import type { View } from "./App";

export type AppShellContextValue = {
  hiddenNavigationItemIDs: Set<View>;
  setNavigationItemVisibility: (itemID: View, visible: boolean) => void;
  resetNavigation: () => void;
};

export const AppShellContext = createContext<AppShellContextValue | null>(null);

export function useAppShellContext(): AppShellContextValue {
  const value = useContext(AppShellContext);
  if (!value) throw new Error("页面必须在控制台外壳内渲染");
  return value;
}
