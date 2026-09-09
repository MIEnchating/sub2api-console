import { ExternalLink } from "lucide-react";

import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { upstreamHostFromBaseUrl } from "@/lib/onboarding-entry";

type Props = {
  name: string;
  host: string;
  hosts: string[];
  baseUrl: string;
};

export const upstreamIdentityLayout = {
  root: "grid min-w-0 gap-1",
  name: "truncate font-medium",
  link: "text-muted-foreground hover:text-primary inline-flex min-w-0 items-center gap-1 text-xs",
  host: "truncate",
} as const;

export function UpstreamIdentity(props: Props) {
  const addressHost = upstreamHostFromBaseUrl(props.baseUrl);
  const displayHost = addressHost || props.host;
  const href = addressHost ? props.baseUrl.trim() : `https://${props.host}`;

  return (
    <div className={upstreamIdentityLayout.root}>
      <Tooltip>
        <TooltipTrigger render={<span className={upstreamIdentityLayout.name} />}>
          {props.name}
        </TooltipTrigger>
        <TooltipContent className="max-w-sm">{props.name}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <a
              href={href}
              target="_blank"
              rel="noreferrer"
              className={upstreamIdentityLayout.link}
              aria-label={`访问 ${props.name}（${displayHost}）`}
            />
          }
        >
          <span className={upstreamIdentityLayout.host}>{displayHost}</span>
          <ExternalLink size={12} className="shrink-0" aria-hidden="true" />
        </TooltipTrigger>
        <TooltipContent className="max-w-sm whitespace-pre-line break-all">
          {props.hosts.length > 1 ? `${href}\n关联 Host：\n${props.hosts.join("\n")}` : href}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}
