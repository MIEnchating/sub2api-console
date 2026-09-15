import type { ReactElement } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export function FormFieldsSkeleton(props: { fields?: number; className?: string }): ReactElement {
  return (
    <div aria-hidden="true" className={cn("grid min-w-0 content-start gap-3", props.className)}>
      {Array.from({ length: props.fields ?? 4 }, (_, field) => (
        <div key={field} className="grid min-w-0 gap-1.5">
          <Skeleton className="h-4 w-24" />
          <Skeleton data-slot="skeleton-control" className="h-8 w-full rounded-lg" />
        </div>
      ))}
    </div>
  );
}
