import { ChevronLeft, ChevronRight, Layers2 } from "lucide-react";
import type { ReactElement } from "react";
import { Button } from "@/components/ui/button";

export function ResultModelSelector(props: {
  models?: string[];
  value?: string;
  onChange: (model: string) => void;
}): ReactElement | null {
  const models = props.models;
  if (!models || models.length < 2) return null;
  const index = Math.max(0, models.indexOf(props.value ?? ""));
  const select = (next: number): void => {
    if (next >= 0 && next < models.length && next !== index) props.onChange(models[next]);
  };
  return (
    <div
      role="group"
      aria-label="结果模型"
      tabIndex={0}
      className="flex min-w-0 items-center gap-2 border-y border-border/50 bg-muted/20 px-3 py-1.5 outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      onKeyDown={(event) => {
        const destinations: Record<string, number> = {
          ArrowLeft: index - 1,
          ArrowRight: index + 1,
          Home: 0,
          End: models.length - 1,
        };
        const next = destinations[event.key];
        if (next === undefined) return;
        event.preventDefault();
        select(next);
      }}
    >
      <Layers2 aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
      <span
        aria-live="polite"
        aria-atomic="true"
        className="flex min-w-0 flex-1 items-center gap-2"
      >
        <span title={models[index]} className="min-w-0 flex-1 truncate text-sm font-medium">
          {models[index]}
        </span>
        <span
          className="shrink-0 text-xs text-muted-foreground tabular-nums"
          aria-label={`第 ${index + 1} 个，共 ${models.length} 个模型结果`}
        >
          {index + 1} / {models.length}
        </span>
      </span>
      <div className="flex shrink-0 items-center gap-0.5">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="上一个模型结果"
          title="上一个模型结果"
          disabled={index === 0}
          onClick={() => select(index - 1)}
        >
          <ChevronLeft aria-hidden="true" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="下一个模型结果"
          title="下一个模型结果"
          disabled={index === models.length - 1}
          onClick={() => select(index + 1)}
        >
          <ChevronRight aria-hidden="true" />
        </Button>
      </div>
    </div>
  );
}
