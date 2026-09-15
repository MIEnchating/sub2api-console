import type { ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, ClipboardList, ShieldCheck } from "lucide-react";
import { api } from "@/api";
import { Button } from "@/components/ui/button";
import { taskStatusLabels, workbenchKeys } from "../constants";
import { workbenchResultItems } from "../lib/task-results";

export function WorkbenchCheckerStatus(): ReactElement {
  const query = useQuery({
    queryKey: ["model-check-capabilities"],
    queryFn: api.modelCheckCapabilities,
  });
  let label = "正在读取检测状态";
  if (query.isError) label = "检测状态读取失败";
  else if (query.data) label = query.data.sol_models?.length ? "Sol 快检就绪" : "Sol 快检未就绪";
  return (
    <span role="status" className="flex items-center gap-1.5 text-xs text-muted-foreground">
      <ShieldCheck className="size-4" aria-hidden="true" />
      {label}
    </span>
  );
}

export function WorkbenchRecentRun(props: { onOpen: () => void }): ReactElement | null {
  const query = useQuery({
    queryKey: workbenchKeys.history,
    queryFn: api.workbenchHistory,
    refetchInterval: 3000,
  });
  const latest = query.data?.find((task) =>
    ["account-workbench-mixed", "account-workbench-import", "account-workbench-convert"].includes(
      task.operation,
    ),
  );
  if (!latest) return null;
  const count = workbenchResultItems(latest.result).length;
  return (
    <Button
      variant="ghost"
      className="h-auto min-h-8 w-full justify-start border-t py-3 text-left whitespace-normal"
      onClick={props.onOpen}
    >
      <ClipboardList aria-hidden="true" />
      <span className="min-w-0 flex-1 wrap-anywhere">
        最近处理 · {count} 个账号 · {taskStatusLabels[latest.status]}
      </span>
      <ChevronRight aria-hidden="true" />
    </Button>
  );
}
