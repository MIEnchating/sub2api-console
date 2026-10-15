import type { AnimationQualityPrediction } from "@/api";
import { animationQualityFailureLabels } from "../constants";

// Execution failures are not labels for the animation or training samples.
export function animationQualityFailureLabel(
  prediction: AnimationQualityPrediction,
): string | undefined {
  if (prediction.status !== "unknown") return undefined;
  return animationQualityFailureLabels[prediction.reason ?? ""];
}
