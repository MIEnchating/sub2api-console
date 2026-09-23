import type { ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { animationPlatformLabels } from "../constants";

export function AnimationEndpointDetails(props: {
  source: Pick<AnimationResult, "account_id" | "endpoint" | "platform">;
}): ReactElement | null {
  if (!props.source.account_id.startsWith("custom-")) return null;
  return (
    <>
      <dt className="text-muted-foreground">接口地址</dt>
      <dd className="min-w-0 wrap-anywhere">{props.source.endpoint || "接口地址未记录"}</dd>
      <dt className="text-muted-foreground">接口类型</dt>
      <dd>{props.source.platform ? animationPlatformLabels[props.source.platform] : "未记录"}</dd>
    </>
  );
}
