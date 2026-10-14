import { lazy, Suspense } from "react";
import { Film, CircleAlert } from "lucide-react";
import type { ProbeResult } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { TaskStartupState } from "@/components/task-startup-state";

const AnimationPreview = lazy(() =>
  import("@/features/model-check/components/animation-preview").then((module) => ({
    default: module.AnimationPreview,
  })),
);

export function ProbeAnimationStage(props: { result: ProbeResult | null; pending: boolean }) {
  const animation = props.result?.animation;
  const ready =
    !props.pending && animation?.status === "succeeded" && (animation.html || animation.svg);
  const failed = !props.pending && props.result?.status === "failed";
  return (
    <div
      role="region"
      aria-label={ready ? "动画测试预览" : "动画展示区域"}
      className="mx-auto aspect-[4/3] w-full max-w-[min(38rem,calc(max(10rem,100svh-31rem)*4/3))] sm:max-w-[min(38rem,calc(max(12rem,100svh-28rem)*4/3))] min-w-0 overflow-hidden rounded-lg border bg-muted/30"
    >
      {ready ? (
        <Suspense fallback={<ContentLoading label="正在加载动画预览" />}>
          <AnimationPreview result={animation} />
        </Suspense>
      ) : (
        <div className="flex h-full flex-col items-center justify-center gap-2 p-4 text-center text-muted-foreground">
          {props.pending ? (
            <TaskStartupState message="正在生成动画，请稍候" />
          ) : (
            <>
              {failed ? (
                <CircleAlert aria-hidden="true" className="size-6" />
              ) : (
                <Film aria-hidden="true" className="size-6" />
              )}
              <p className="text-sm">{failed ? "本次未生成可展示的动画" : "动画将在这里展示"}</p>
              <p className="text-xs">
                {failed ? "请查看测试结果后重试" : "选择模型并开始测试，完成后可放大查看"}
              </p>
            </>
          )}
        </div>
      )}
    </div>
  );
}
