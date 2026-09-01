import { DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";

export type UpstreamGroupDialogView = "catalog" | "history";

export function UpstreamGroupDialogHeader(props: {
  view: UpstreamGroupDialogView;
  onViewChange: (view: UpstreamGroupDialogView) => void;
}) {
  return (
    <DialogHeader className="flex-row flex-wrap items-center justify-between gap-3 pr-10">
      <DialogTitle>上游分组</DialogTitle>
      <SegmentedControl className="mr-1 shrink-0">
        <SegmentedControlItem
          selected={props.view === "catalog"}
          onClick={() => props.onViewChange("catalog")}
        >
          分组目录
        </SegmentedControlItem>
        <SegmentedControlItem
          selected={props.view === "history"}
          onClick={() => props.onViewChange("history")}
        >
          变化历史
        </SegmentedControlItem>
      </SegmentedControl>
    </DialogHeader>
  );
}
