import { Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";

type AlertListActionsProps = {
  loading: boolean;
  failed: boolean;
  clearableCount: number;
  onClear: () => void;
};

export function AlertListActions(props: AlertListActionsProps) {
  return (
    <div className="flex items-center">
      <Button
        variant="outline"
        size="sm"
        className="text-destructive hover:text-destructive"
        disabled={props.loading || props.failed || props.clearableCount === 0}
        onClick={props.onClear}
      >
        <Trash2 size={15} aria-hidden="true" />
        清理已结束
      </Button>
    </div>
  );
}
