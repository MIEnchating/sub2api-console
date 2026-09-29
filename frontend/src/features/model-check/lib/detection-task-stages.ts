import type { DetectionTask } from "@/api";
import { detectionStageOptions } from "../constants";

export function detectionTaskStages(
  value: Pick<DetectionTask, "animation" | "precheck" | "terminal" | "terminal_rounds">,
): string[] {
  return (["precheck", "terminal", "animation"] as const).flatMap((stage) => {
    const enabled = stage === "animation" ? value.animation !== false : value[stage];
    if (!enabled) return [];
    const label = detectionStageOptions.find((option) => option.value === stage)!.label;
    return [stage === "terminal" ? `${label} ${value.terminal_rounds} 轮` : label];
  });
}
