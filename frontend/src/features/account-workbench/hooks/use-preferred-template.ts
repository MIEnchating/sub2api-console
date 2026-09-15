import { useEffect, useRef } from "react";
import type { WorkbenchTemplate } from "@/api";

export function usePreferredTemplate(
  templates: WorkbenchTemplate[] | undefined,
  select: (id: string) => void,
  followPreference = false,
): void {
  const initialized = useRef(false);
  const lastPreference = useRef("");
  useEffect(() => {
    if (!templates) return;
    const preferred = templates.find((item) => item.preferred);
    const id = preferred?.id ?? "";
    if (initialized.current && (!followPreference || lastPreference.current === id)) return;
    initialized.current = true;
    lastPreference.current = id;
    if (preferred || followPreference) select(id);
  }, [templates, select, followPreference]);
}
