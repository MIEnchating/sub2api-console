import { useEffect, useRef } from "react";
import type { WorkbenchTemplate } from "@/api";

export function usePreferredTemplate(
  templates: WorkbenchTemplate[] | undefined,
  select: (id: string) => void,
): void {
  const initialized = useRef(false);
  useEffect(() => {
    if (initialized.current || !templates) return;
    initialized.current = true;
    const preferred = templates.find((item) => item.preferred);
    if (preferred) select(preferred.id);
  }, [templates, select]);
}
