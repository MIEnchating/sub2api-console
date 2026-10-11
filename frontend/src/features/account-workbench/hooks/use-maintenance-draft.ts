import { useEffect, useMemo, useRef, useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import { toast } from "sonner";
import { maintenanceFormValues, type MaintenanceValues } from "../lib/maintenance-schema";
import type { WorkbenchMaintenance } from "../types";

export function useMaintenanceDraft(
  value: WorkbenchMaintenance,
  form: UseFormReturn<MaintenanceValues>,
  awaitingConfirmation: boolean,
): { changed: boolean; reset: (value: WorkbenchMaintenance) => void } {
  const latest = useMemo(() => maintenanceFormValues(value), [value]);
  const latestKey = JSON.stringify(latest);
  const [baseline, setBaseline] = useState({ key: latestKey, revision: value.revision });
  const notifiedKey = useRef("");
  const isDirty = form.formState.isDirty;
  // A mutation can render its confirmed form before the query observer receives
  // the new value. Never reset that form from an older observer snapshot.
  const changed = value.revision >= baseline.revision && latestKey !== baseline.key;

  useEffect(() => {
    if (!changed) return;
    if (isDirty || awaitingConfirmation) {
      if (notifiedKey.current !== latestKey) {
        toast.info("维护设置已更新，请载入最新设置后重新编辑");
        notifiedKey.current = latestKey;
      }
      return;
    }
    form.reset(latest);
    setBaseline({ key: latestKey, revision: value.revision });
    notifiedKey.current = "";
  }, [changed, isDirty, awaitingConfirmation, latestKey, latest, value.revision, form]);

  const reset = (next: WorkbenchMaintenance): void => {
    const settings = maintenanceFormValues(next);
    form.reset(settings);
    setBaseline({ key: JSON.stringify(settings), revision: next.revision });
    notifiedKey.current = "";
  };
  return { changed, reset };
}
