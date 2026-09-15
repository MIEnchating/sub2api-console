import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { useMemo, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Play } from "lucide-react";
import { api, type Task } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { taskStatusLabels, workbenchKeys, workbenchOperationLabels } from "../constants";
import { filterHistory } from "../lib/history-filter";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchRetry } from "./workbench-retry";
import { WorkbenchTaskRecovery } from "./workbench-task-recovery";
import { WorkbenchTaskOrders } from "./workbench-task-orders";
import { WorkbenchOAuthBatchBrowser } from "./workbench-oauth-batch-browser";
import { WorkbenchExports } from "./workbench-exports";
import { workbenchResultItems } from "../lib/task-results";
import { taskIsTerminal } from "@/lib/task-state";

export function WorkbenchHistory(
  props: {
    activeTaskId?: string | null;
    onContinue?: () => void;
  } = {},
): ReactElement {
  const statuses = useDictionaryOrder(
    "task_status",
    Object.entries(taskStatusLabels),
    (item) => item[0],
  );
  const [selection, setSelected] = useState<Task | null>(null);
  const [retry, setRetry] = useState<{ task: Task; indexes: number[] } | null>(null);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [authorization, setAuthorization] = useState<string | null>(null);
  const [orders, setOrders] = useState<string | null>(null);
  const query = useQuery({
    queryKey: workbenchKeys.history,
    queryFn: api.workbenchHistory,
    refetchInterval: 3000,
  });
  const selected = query.data?.find((task) => task.id === selection?.id) ?? selection;
  const tasks = useMemo(
    () => filterHistory(query.data ?? [], search, status, ""),
    [query.data, search, status],
  );
  function closeDetails(): void {
    setRetry(null);
    setSelected(null);
    setAuthorization(null);
    setOrders(null);
  }
  if (query.isPending) return <PageLoadingSkeleton label="正在读取账号处理记录" variant="list" />;
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label="搜索处理记录"
          placeholder="搜索账号或处理结果"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="min-w-0 flex-1 basis-48"
        />
        <Select value={status} onValueChange={(value) => setStatus(value ?? "")}>
          <SelectTrigger aria-label="筛选处理状态">
            <SelectValue>{taskStatusLabels[status as Task["status"]] ?? "全部状态"}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">全部状态</SelectItem>
            {statuses.map(([id, label]) => (
              <SelectItem key={id} value={id}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>
          <RefreshCw aria-hidden="true" />
          刷新记录
        </Button>
      </div>
      {props.activeTaskId && (
        <div>
          <Button variant="outline" onClick={props.onContinue}>
            <Play aria-hidden="true" />
            继续当前批次
          </Button>
        </div>
      )}
      {!tasks.length && (
        <p className="py-12 text-center text-sm text-muted-foreground">
          {query.data.length ? "没有匹配的处理记录" : "暂无处理记录"}
        </p>
      )}
      <ul className="min-w-0 divide-y" aria-label="账号处理记录">
        {tasks.map((task) => (
          <li
            key={task.id}
            className="flex min-w-0 flex-col gap-2 py-3 sm:flex-row sm:items-center"
          >
            <div className="min-w-0 flex-1 text-sm">
              <p className="font-medium">
                {workbenchOperationLabels[task.operation] ?? task.operation}
              </p>
              <p className="wrap-anywhere">{task.message || task.id}</p>
              <p className="text-xs text-muted-foreground">{task.created_at}</p>
            </div>
            <Badge variant="secondary">{taskStatusLabels[task.status]}</Badge>
            <Button
              variant="outline"
              aria-label={`查看任务 ${task.id}`}
              onClick={() => {
                setRetry(null);
                setSelected(task);
              }}
            >
              查看结果
            </Button>
          </li>
        ))}
      </ul>
      {selected && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) closeDetails();
          }}
        >
          <DialogContent width="wide">
            <DialogHeader>
              <DialogTitle>处理详情</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <section aria-label="本批处理详情" className="grid min-w-0 gap-3">
                <WorkbenchTask
                  key={selected.id}
                  task={selected}
                  onRetry={(task, indexes) => setRetry({ task, indexes })}
                />
                {taskIsTerminal(selected) &&
                  ["account-workbench-import", "account-workbench-retry"].includes(
                    selected.operation,
                  ) &&
                  workbenchResultItems(selected.result).some((item) => !!item.accountId) && (
                    <WorkbenchExports key={`export-${selected.id}`} sourceTaskId={selected.id} />
                  )}
                {selected.operation === "account-workbench-oauth" &&
                  !props.activeTaskId &&
                  (selected.status === "waiting_input" || selected.status === "running") && (
                    <div>
                      <Button variant="outline" onClick={() => setAuthorization(selected.id)}>
                        <Play aria-hidden="true" />
                        继续授权
                      </Button>
                    </div>
                  )}
                {authorization === selected.id && (
                  <WorkbenchOAuthBatchBrowser
                    key={`oauth-${selected.id}`}
                    id={selected.id}
                    disabled={false}
                  />
                )}
                {selected.operation === "account-workbench-oauth" && (
                  <div>
                    <Button
                      variant="outline"
                      onClick={() => setOrders(orders === selected.id ? null : selected.id)}
                      aria-expanded={orders === selected.id}
                    >
                      本次授权短信订单
                    </Button>
                  </div>
                )}
                {orders === selected.id && <WorkbenchTaskOrders taskId={selected.id} />}
                {selected.operation === "account-workbench-mixed" &&
                  selected.id !== props.activeTaskId &&
                  typeof selected.result.recovery_id === "string" &&
                  !!selected.result.recovery_id && (
                    <WorkbenchTaskRecovery key={`recovery-${selected.id}`} task={selected} />
                  )}
              </section>
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" onClick={closeDetails}>
                关闭详情
              </Button>
            </DialogFooter>
            {retry && (
              <WorkbenchRetry
                key={`${retry.task.id}:${retry.indexes.join(",")}`}
                task={retry.task}
                indexes={retry.indexes}
                onClose={() => setRetry(null)}
                onCreated={(task) => {
                  setRetry(null);
                  setSelected(task);
                }}
              />
            )}
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
