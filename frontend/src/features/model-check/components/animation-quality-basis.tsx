import { useId, useState, type ReactElement } from "react";
import { ChevronDown, Code2 } from "lucide-react";
import type { AnimationQualityPrediction } from "@/api";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function AnimationQualityBasis(props: {
  basis: AnimationQualityPrediction["basis"];
}): ReactElement | null {
  const [expanded, setExpanded] = useState(false);
  const contentID = useId();
  if (!props.basis?.length) return null;
  return (
    <section aria-label="自动判断依据" className="min-w-0 rounded-lg border">
      <div className="p-2">
        <Button
          type="button"
          variant="ghost"
          className="w-full justify-between gap-2"
          aria-expanded={expanded}
          aria-controls={contentID}
          onClick={() => setExpanded(!expanded)}
        >
          <span className="flex items-center gap-2">
            <Code2 aria-hidden="true" />
            原文依据
            <span className="text-xs font-normal text-muted-foreground">
              {props.basis.length} 项
            </span>
          </span>
          <ChevronDown
            aria-hidden="true"
            className={cn("transition-transform", expanded && "rotate-180")}
          />
        </Button>
      </div>
      {expanded ? (
        <dl id={contentID} className="min-w-0 divide-y border-t">
          {props.basis.map((item, index) => (
            <div key={index} className="min-w-0 space-y-2 p-3 sm:p-4">
              <dt className="flex items-start gap-2 font-medium wrap-anywhere">
                <span
                  className="grid size-5 shrink-0 place-items-center rounded bg-muted text-xs text-muted-foreground"
                  aria-hidden="true"
                >
                  {index + 1}
                </span>
                <span>{item.requirement}</span>
              </dt>
              <dd className="whitespace-pre-wrap leading-6 text-muted-foreground wrap-anywhere">
                {item.assessment}
              </dd>
              <dd>
                <pre
                  tabIndex={0}
                  aria-label={`${item.requirement}的原文证据`}
                  className="max-h-40 overflow-auto rounded-md bg-muted/50 p-3 font-mono text-xs leading-5 whitespace-pre-wrap break-all outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <code>{item.evidence}</code>
                </pre>
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
    </section>
  );
}
