import type { ComponentProps, ReactElement } from "react";

import { cn } from "@/lib/utils";

export function PricingConfigLayout(props: ComponentProps<"div">): ReactElement {
  return (
    <div
      {...props}
      data-slot="pricing-config-layout"
      className={cn(
        "grid min-w-0 items-start gap-4 xl:grid-cols-[18rem_minmax(0,1fr)]",
        props.className,
      )}
    />
  );
}
