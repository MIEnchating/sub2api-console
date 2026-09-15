import { useMemo, useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Task, type WorkbenchRegenerationInput } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { PageLoadingSkeleton } from "@/components/page-loading-skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { taskIsTerminal } from "@/lib/task-state";
import { taskStatusLabels, workbenchKeys, workbenchOperationLabels } from "../constants";
import { filterHistory } from "../lib/history-filter";
import { WorkbenchHistoryActions } from "./workbench-history-actions";
import { WorkbenchHistoryQuery } from "./workbench-history-query";
import { WorkbenchRegeneration } from "./workbench-regeneration";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchRetry } from "./workbench-retry";
import { WorkbenchOAuthBatch } from "./workbench-oauth-batch";

export function WorkbenchHistory(): ReactElement {
  const [selectedSnapshot, setSelected] = useState<Task | null>(null);
  const [retry, setRetry] = useState<{ task: Task; indexes: number[] } | null>(null);
  const [reauthorizing, setReauthorizing] = useState<string | null>(null);
  const [regenerating, setRegenerating] = useState<WorkbenchRegenerationInput | null>(null);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [operation, setOperation] = useState("");
  const query = useQuery({
    queryKey: workbenchKeys.history,
    queryFn: api.workbenchHistory,
    refetchInterval: (state) =>
      state.state.data?.some((task) => !taskIsTerminal(task)) ? 2000 : false,
  });
  const selectedQuery = useQuery({
    queryKey: workbenchKeys.task(selectedSnapshot?.id ?? ""),
    queryFn: () => api.task(selectedSnapshot!.id),
    enabled: selectedSnapshot !== null,
    initialData: selectedSnapshot ?? undefined,
  });
  const selected = selectedQuery.data ?? selectedSnapshot;
  const tasks = useMemo(
    () => filterHistory(query.data ?? [], search, status, operation),
    [query.data, search, status, operation],
  );
  const operations = useMemo(
    () => [...new Set(query.data?.map((task) => task.operation) ?? [])],
    [query.data],
  );
  if (query.isPending) return <PageLoadingSkeleton label="正在读取账号处理记录" variant="list" />;
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  return (
    <div className="grid min-w-0 gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          aria-label="搜索处理记录"
          placeholder="搜索任务 ID、邮箱或结果"
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
            {Object.entries(taskStatusLabels).map(([id, label]) => (
              <SelectItem key={id} value={id}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={operation} onValueChange={(value) => setOperation(value ?? "")}>
          <SelectTrigger aria-label="筛选处理类型">
            <SelectValue>{workbenchOperationLabels[operation] ?? "全部类型"}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">全部类型</SelectItem>
            {operations.map((id) => (
              <SelectItem key={id} value={id}>
                {workbenchOperationLabels[id] ?? id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>
          刷新记录
        </Button>
        <WorkbenchHistoryQuery
          onSelect={(task) => {
            setRetry(null);
            setReauthorizing(null);
            setSelected(task);
          }}
        />
      </div>
      <WorkbenchHistoryActions
        tasks={query.data}
        checked={checked}
        onDeleted={(ids) => {
          setChecked((previous) => new Set([...previous].filter((id) => !ids.includes(id))));
          if (selected && ids.includes(selected.id)) setSelected(null);
          if (retry && ids.includes(retry.task.id)) setRetry(null);
          if (reauthorizing && ids.includes(reauthorizing)) setReauthorizing(null);
        }}
      />
      {tasks.length > 0 && (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={tasks.every((task) => checked.has(task.id))}
            indeterminate={
              tasks.some((task) => checked.has(task.id)) &&
              !tasks.every((task) => checked.has(task.id))
            }
            onCheckedChange={(value) =>
              setChecked((previous) => {
                const next = new Set(previous);
                for (const task of tasks) {
                  if (value) next.add(task.id);
                  else next.delete(task.id);
                }
                return next;
              })
            }
          />
          选择筛选结果（{tasks.length} 条）
        </label>
      )}
      {!tasks.length && (
        <p className="rounded-lg border p-8 text-center text-sm text-muted-foreground">
          {query.data.length ? "没有匹配的处理记录，请调整筛选条件" : "暂无账号处理记录"}
        </p>
      )}
      <ul className="divide-y rounded-lg border bg-card" aria-label="账号处理记录">
        {tasks.map((task) => (
          <li key={task.id} className="flex min-w-0 flex-col gap-2 p-3 sm:flex-row sm:items-center">
            <Checkbox
              aria-label={`选择处理记录 ${task.id}`}
              checked={checked.has(task.id)}
              onCheckedChange={(value) =>
                setChecked((previous) => {
                  const next = new Set(previous);
                  if (value) next.add(task.id);
                  else next.delete(task.id);
                  return next;
                })
              }
            />
            <div className="min-w-0 flex-1 text-sm">
              <p className="text-xs text-muted-foreground">
                {workbenchOperationLabels[task.operation] ?? task.operation}
              </p>
              <p className="wrap-anywhere">{task.message || task.id}</p>
              <p className="text-xs text-muted-foreground">{task.created_at}</p>
            </div>
            <Badge variant="secondary">{taskStatusLabels[task.status]}</Badge>
            <Button
              variant="outline"
              onClick={() => {
                setRetry(null);
                setReauthorizing(null);
                setSelected(task);
              }}
              aria-label={`查看任务 ${task.id}`}
            >
              查看结果
            </Button>
          </li>
        ))}
      </ul>
      {selected && (
        <WorkbenchTask
          key={selected.id}
          task={selected}
          onRetry={(task, indexes) => setRetry({ task, indexes })}
        />
      )}
      {selected &&
        taskIsTerminal(selected) &&
        [
          "account-workbench-export",
          "account-workbench-convert",
          "account-workbench-regenerate",
          "account-workbench-import",
          "account-workbench-retry",
        ].includes(selected.operation) && (
          <Button
            variant="outline"
            onClick={() => {
              const items: unknown = selected.result.items;
              const local =
                Array.isArray(items) &&
                items.some(
                  (item: unknown) =>
                    typeof item === "object" &&
                    item !== null &&
                    "report" in item &&
                    typeof item.report === "object" &&
                    item.report !== null &&
                    "scope" in item.report &&
                    item.report.scope === "local-export",
                );
              setRegenerating({
                source_task_id: selected.id,
                scope: local ? "local-export" : undefined,
              });
            }}
          >
            从此记录重新生成授权文件
          </Button>
        )}
      {regenerating && (
        <WorkbenchRegeneration
          source={regenerating}
          onClose={() => setRegenerating(null)}
          onCreated={setSelected}
        />
      )}
      {selected &&
      taskIsTerminal(selected) &&
      selected.operation === "account-workbench-oauth-batch" &&
      Array.isArray(selected.result.items) &&
      selected.result.items.some(
        (row: unknown) =>
          typeof row === "object" && row !== null && "profile_id" in row && !!row.profile_id,
      ) ? (
        <Button variant="outline" onClick={() => setReauthorizing(selected.id)}>
          从此记录重新授权
        </Button>
      ) : null}
      {reauthorizing ? (
        <section aria-label="历史账号重新授权" className="min-w-0 space-y-3">
          <Button variant="outline" onClick={() => setReauthorizing(null)}>
            关闭历史重新授权
          </Button>
          <WorkbenchOAuthBatch key={reauthorizing} sourceTaskId={reauthorizing} />
        </section>
      ) : null}
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
    </div>
  );
}
