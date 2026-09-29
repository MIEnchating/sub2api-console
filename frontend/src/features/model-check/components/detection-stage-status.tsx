import type { ReactElement } from "react";
import { CircleCheck, CircleX, Clock3, TriangleAlert } from "lucide-react";
import { cn } from "@/lib/utils";
import { detectionResultToneClasses, type DetectionResultTone } from "../constants";

const statusIcons = {
  success: CircleCheck,
  danger: CircleX,
  warning: TriangleAlert,
  muted: Clock3,
};

export function DetectionStageStatus(props: {
  stage: string;
  label: string;
  tone: DetectionResultTone;
}): ReactElement {
  const Icon = statusIcons[props.tone];
  const label = `${props.stage} · ${props.label}`;
  return (
    <div
      role="group"
      aria-label={label}
      className={cn(
        "inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-md border px-2 py-1 text-sm font-medium",
        detectionResultToneClasses[props.tone],
      )}
    >
      <Icon aria-hidden="true" className="size-4 shrink-0" />
      <span className="min-w-0 wrap-anywhere">{label}</span>
    </div>
  );
}
