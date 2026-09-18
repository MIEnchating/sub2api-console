import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";
import { useQuery } from "@tanstack/react-query";
import { CircleAlert, CircleHelp, LoaderCircle, Radio, WifiOff } from "lucide-react";
import { api, type AccountTrafficSnapshot } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type TrafficState = {
  status: "loading" | "unavailable" | "disabled" | "ready";
  accounts: Map<string, AccountTrafficSnapshot["accounts"][number]>;
};
const TrafficContext = createContext<TrafficState | null>(null);

export function AccountTrafficProvider(props: { children: ReactNode }): ReactElement {
  const query = useQuery({
    queryKey: ["accounts", "live-traffic"],
    queryFn: api.accountTraffic,
    refetchInterval: 5_000,
    staleTime: 2_000,
    retry: false,
  });
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 5_000);
    return () => clearInterval(timer);
  }, []);
  const value = useMemo<TrafficState>(() => {
    let status: TrafficState["status"] = "ready";
    if (query.isPending) status = "loading";
    else if (query.isError || !query.data) status = "unavailable";
    else if (!query.data.enabled) status = "disabled";
    else {
      const observed = Date.parse(query.data.observed_at);
      if (!Number.isFinite(observed) || now - observed > 15_000 || observed - now > 5_000)
        status = "unavailable";
    }
    return {
      status,
      accounts: new Map((query.data?.accounts ?? []).map((item) => [item.account_id, item])),
    };
  }, [query.data, query.isError, query.isPending, now]);
  return <TrafficContext.Provider value={value}>{props.children}</TrafficContext.Provider>;
}

export function AccountTrafficBadge(props: {
  accountID: string;
  compact?: boolean;
}): ReactElement | null {
  const traffic = useContext(TrafficContext);
  if (!traffic) return null;
  const row = traffic.accounts.get(props.accountID);
  let label = "流量未知";
  let compactLabel = "未知";
  let Icon = CircleHelp;
  let detail = "未返回该账号的实时并发，不能判断当前是否有请求。";
  let active = false;
  if (traffic.status === "loading") {
    label = "流量读取中";
    compactLabel = "读取中";
    Icon = LoaderCircle;
    detail = "正在读取 Sub2API 实时并发。";
  } else if (traffic.status === "unavailable") {
    label = "流量读取失败";
    compactLabel = "不可用";
    Icon = CircleAlert;
    detail = "实时快照不可用或已过期，稍后自动重试；旧计数不参与选择。";
  } else if (traffic.status === "disabled") {
    label = "流量监控未开启";
    compactLabel = "未开启";
    Icon = WifiOff;
    detail = "请在 Sub2API 中开启运维监控和实时监控。";
  } else if (row?.current_requests && row.current_requests > 0) {
    active = true;
    label = `真实请求 · ${row.current_requests}`;
    compactLabel = `${row.current_requests > 999 ? "999+" : row.current_requests} 请求`;
    Icon = Radio;
    detail = `Sub2API 当前占用 ${row.current_requests} 个账号请求槽，排队 ${row.waiting_requests} 个；每 5 秒刷新，不含 Console 直连探活和模型检测。`;
  } else if (row?.tracked) {
    label = "未观测到请求";
    compactLabel = "未观测";
    Icon = WifiOff;
    detail = `当前没有观测到请求槽占用，排队 ${row.waiting_requests} 个。Sub2API 计数读取异常也可能返回零，不据此断定账号空闲。`;
  } else if (row) {
    detail = "该账号不限并发，Sub2API 可能不记录请求槽，无法以零计数判断流量。";
  }
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span
            tabIndex={0}
            aria-label={`账号 ${props.accountID}：${label}`}
            className={cn(
              "inline-flex shrink-0 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring",
              props.compact &&
                "h-5 w-20 items-center justify-end gap-1 whitespace-nowrap text-xs tabular-nums text-muted-foreground",
              props.compact && active && "text-success",
              props.compact && traffic.status === "unavailable" && "text-warning",
            )}
          />
        }
      >
        {props.compact ? (
          <>
            <Icon aria-hidden="true" className="size-3 shrink-0" />
            <span className="truncate">{compactLabel}</span>
          </>
        ) : (
          <Badge variant={active ? "secondary" : "outline"}>{label}</Badge>
        )}
      </TooltipTrigger>
      <TooltipContent role="tooltip" className="max-w-80">
        {props.compact ? `${label}。${detail}` : detail}
      </TooltipContent>
    </Tooltip>
  );
}

export function SelectTrafficAccounts(props: {
  accountIDs: readonly string[];
  disabled?: boolean;
  onSelect: (ids: string[]) => void;
}): ReactElement {
  const traffic = useContext(TrafficContext);
  const active =
    traffic?.status === "ready"
      ? props.accountIDs.filter((id) => (traffic.accounts.get(id)?.current_requests ?? 0) > 0)
      : [];
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        <Button
          type="button"
          variant="outline"
          disabled={props.disabled || active.length === 0}
          onClick={() => props.onSelect(active.slice(0, 20))}
        >
          <Radio aria-hidden="true" />
          选择实时流量（{active.length}）
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        选择当前筛选范围内正在处理真实请求的账号，最多 20 个；不会自动启动检测。
      </TooltipContent>
    </Tooltip>
  );
}
