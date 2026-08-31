import { ArrowLeft, ChevronLeft, ChevronRight } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

type NavigationTarget = {
  label: string;
  onSelect: () => void;
};

function UpstreamNavigationButton(props: {
  direction: "previous" | "next";
  target: NavigationTarget | null;
}) {
  const previous = props.direction === "previous";
  const label = previous ? "上一个上游" : "下一个上游";
  const unavailable = previous ? "已经是第一个上游" : "已经是最后一个上游";
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        <Button
          type="button"
          variant="outline"
          disabled={props.target === null}
          aria-label={props.target ? `${label}：${props.target.label}` : label}
          onClick={props.target?.onSelect}
        >
          {previous ? <ChevronLeft aria-hidden="true" /> : null}
          {label}
          {!previous ? <ChevronRight aria-hidden="true" /> : null}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {props.target ? `${label}：${props.target.label}` : unavailable}
      </TooltipContent>
    </Tooltip>
  );
}

export function OnboardingHeadingActions(props: {
  onBack: () => void;
  previousUpstream?: NavigationTarget | null;
  nextUpstream?: NavigationTarget | null;
}) {
  return (
    <>
      <Button type="button" variant="outline" onClick={props.onBack}>
        <ArrowLeft aria-hidden="true" />
        返回上游管理
      </Button>
      <UpstreamNavigationButton direction="previous" target={props.previousUpstream ?? null} />
      <UpstreamNavigationButton direction="next" target={props.nextUpstream ?? null} />
      <Badge variant="outline">新建 Key</Badge>
    </>
  );
}
