import { useState, type ReactElement } from "react";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api, type Task } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogBody,
  DialogFooter,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { workbenchKeys, taskStatusLabels } from "../constants";
import {
  historyQuerySchema,
  type HistoryQueryForm,
  historyEmails,
} from "../lib/history-query-schema";
import { WorkbenchHistoryActions } from "./workbench-history-actions";

export function WorkbenchHistoryQuery(props: { onSelect: (task: Task) => void }): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" onClick={() => setOpen(true)}>
        查询全部历史
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>查询全部历史</DialogTitle>
            <DialogDescription>
              查询所有已保存记录，可按完整邮箱批量筛选。每页最多选择 50 条进行操作。
            </DialogDescription>
          </DialogHeader>
          {open && (
            <HistoryQueryContent
              onSelect={(task) => {
                props.onSelect(task);
                setOpen(false);
              }}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function HistoryQueryContent(props: { onSelect: (task: Task) => void }): ReactElement {
  const [input, setInput] = useState<HistoryQueryForm>({ search: "", emails: "" });
  const [offset, setOffset] = useState(0);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const form = useForm<HistoryQueryForm>({
    resolver: zodResolver(historyQuerySchema),
    defaultValues: input,
  });
  const query = useQuery({
    queryKey: [...workbenchKeys.history, "query", input, offset],
    queryFn: () =>
      api.queryWorkbenchHistory({
        search: input.search,
        emails: historyEmails(input.emails),
        status: "",
        operation: "",
        offset,
        limit: 50,
      }),
  });
  const tasks = query.data?.items ?? [];
  return (
    <>
      <DialogBody className="grid gap-3">
        <form
          className="grid gap-3"
          onSubmit={form.handleSubmit((value) => {
            setInput(value);
            setOffset(0);
            setChecked(new Set());
          })}
        >
          <FormField
            label="历史关键词"
            htmlFor="history-query-search"
            error={form.formState.errors.search?.message}
          >
            <Input
              id="history-query-search"
              {...form.register("search")}
              aria-invalid={!!form.formState.errors.search}
            />
          </FormField>
          <FormField
            label="批量查询邮箱"
            htmlFor="history-query-emails"
            error={form.formState.errors.emails?.message}
          >
            <Textarea
              id="history-query-emails"
              {...form.register("emails")}
              placeholder="每行一个完整邮箱，留空查询全部"
              className="max-h-36 min-h-20 resize-y"
              aria-invalid={!!form.formState.errors.emails}
            />
          </FormField>
          <div>
            <Button type="submit" disabled={query.isFetching}>
              查询记录
            </Button>
          </div>
        </form>
        {query.isPending && <ContentLoading label="正在查询全部历史" />}
        {query.isError && (
          <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
        )}
        {query.data && (
          <>
            <WorkbenchHistoryActions
              tasks={tasks}
              checked={checked}
              hideGlobal
              onDeleted={(ids) =>
                setChecked((previous) => new Set([...previous].filter((id) => !ids.includes(id))))
              }
            />
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={tasks.length > 0 && tasks.every((task) => checked.has(task.id))}
                disabled={!tasks.length || query.isFetching}
                onCheckedChange={(value) =>
                  setChecked(value ? new Set(tasks.map((task) => task.id)) : new Set())
                }
              />
              选择本页记录
            </label>
            <ul className="max-h-64 min-w-0 divide-y overflow-y-auto" aria-label="全部历史查询结果">
              {tasks.map((task) => (
                <li key={task.id} className="flex min-w-0 items-start gap-2 py-2 text-sm">
                  <Checkbox
                    aria-label={`选择历史记录 ${task.id}`}
                    checked={checked.has(task.id)}
                    disabled={query.isFetching}
                    onCheckedChange={(value) =>
                      setChecked((previous) => {
                        const next = new Set(previous);
                        if (value) next.add(task.id);
                        else next.delete(task.id);
                        return next;
                      })
                    }
                  />
                  <div className="min-w-0 flex-1 wrap-anywhere">
                    <p>{task.message || task.id}</p>
                    <p className="text-xs text-muted-foreground">
                      ID：{task.id} · {taskStatusLabels[task.status]}
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    aria-label={`查看历史任务 ${task.id}`}
                    onClick={() => props.onSelect(task)}
                  >
                    查看结果
                  </Button>
                </li>
              ))}
            </ul>
            {!tasks.length && <p className="text-sm text-muted-foreground">没有匹配的历史记录</p>}
          </>
        )}
      </DialogBody>
      {query.data && (
        <DialogFooter>
          <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
            <span>
              共 {query.data.total} 条 · 第 {Math.floor(offset / 50) + 1} 页
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                disabled={offset === 0 || query.isFetching}
                onClick={() => {
                  setOffset((value) => Math.max(0, value - 50));
                  setChecked(new Set());
                }}
              >
                上一页
              </Button>
              <Button
                variant="outline"
                disabled={offset + 50 >= query.data.total || query.isFetching}
                onClick={() => {
                  setOffset((value) => value + 50);
                  setChecked(new Set());
                }}
              >
                下一页
              </Button>
            </div>
          </div>
        </DialogFooter>
      )}
    </>
  );
}
