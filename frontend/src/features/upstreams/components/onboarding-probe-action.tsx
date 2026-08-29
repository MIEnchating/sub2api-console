import { Activity, LoaderCircle } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export type OnboardingProbeTarget = {
  host: string;
  groupId: string;
  name: string;
};

export function OnboardingProbeAction(props: {
  target: OnboardingProbeTarget | null;
  groupName: string;
  pending: boolean;
  onProbe: () => void;
}) {
  const available = props.target !== null;
  const label = available ? "探活测试" : "当前不可探活";
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-label={label}
          disabled={!available || props.pending}
          onClick={props.onProbe}
        >
          {props.pending ? <LoaderCircle className="animate-spin" /> : <Activity />}
          探活
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
