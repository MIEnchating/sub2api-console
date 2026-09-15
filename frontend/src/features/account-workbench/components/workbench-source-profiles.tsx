import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LogIn, Pencil, RefreshCw, Trash2 } from "lucide-react";
import { api, ApiError, type Task, type WorkbenchSourceProfile } from "@/api";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { notifyOperationError } from "@/lib/operation-feedback";
import { WorkbenchLoginProfileEditor } from "./workbench-login-profile-editor";
import { WorkbenchProfileExport } from "./workbench-profile-export";
import { WorkbenchTask } from "./workbench-task";

export function WorkbenchSourceProfiles(props: {
  onAuthorize: (items: Array<{ id: string; revision: number }>) => void;
}): ReactElement {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["account-workbench", "source-profiles"],
    queryFn: (context) => api.workbenchSourceProfiles(context.signal),
    gcTime: 0,
  });
  const [selected, setSelected] = useState<string[]>([]);
  const [search, setSearch] = useState("");
  const [editing, setEditing] = useState<WorkbenchSourceProfile | null>(null);
  const [deleting, setDeleting] = useState<WorkbenchSourceProfile | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const remove = useMutation({
    mutationFn: (profile: WorkbenchSourceProfile) =>
      api.deleteWorkbenchSourceProfile(profile.id, profile.revision),
    onSuccess: (_value, profile) => {
      setDeleting(null);
      setSelected((items) => items.filter((id) => id !== profile.id));
      void query.refetch();
    },
    onError: (error) => {
      notifyOperationError(error, "本地登录资料删除失败，请刷新版本后重试");
      if (error instanceof ApiError && error.status === 409) {
        setDeleting(null);
        void query.refetch();
      }
    },
  });
  if (query.isPending)
    return (
      <div role="status" aria-label="正在读取本地登录资料" aria-busy="true" className="grid gap-3">
        <Skeleton className="h-8 w-full" />
        {[0, 1, 2].map((index) => (
          <Skeleton key={index} className="h-20 w-full" />
        ))}
      </div>
    );
  if (!query.data)
    return <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />;
  const visible = query.data.filter((item) =>
    `${item.email} ${item.user_id} ${item.workspace_id}`
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  const chosen = query.data.filter((item) => selected.includes(item.id));
  const fresh = !query.isError && !query.isFetching && !remove.isPending;
  return (
    <section aria-label="本地登录资料" className="grid min-w-0 gap-3">
      <div className="flex flex-wrap gap-2">
        <Input
          aria-label="搜索本地登录资料"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="min-w-0 flex-1 basis-48"
        />
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>
          <RefreshCw aria-hidden="true" />
          刷新资料
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            disabled={!fresh || !visible.length}
            checked={visible.length > 0 && visible.every((item) => selected.includes(item.id))}
            onCheckedChange={(checked) =>
              setSelected(
                checked
                  ? [...new Set([...selected, ...visible.map((item) => item.id)])]
                  : selected.filter((id) => !visible.some((item) => item.id === id)),
              )
            }
          />
          选择当前资料
        </label>
        <Button
          disabled={!fresh || !chosen.length}
          onClick={() =>
            props.onAuthorize(chosen.map((item) => ({ id: item.id, revision: item.revision })))
          }
        >
          <LogIn aria-hidden="true" />
          预览重新登录（{chosen.length}）
        </Button>
        <WorkbenchProfileExport
          profiles={chosen}
          scope="local-export"
          disabled={!fresh}
          onCreated={setTask}
        />
      </div>
      {query.isError ? (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      ) : null}
      {!visible.length ? (
        <p className="text-sm text-muted-foreground">暂无匹配的本地登录资料</p>
      ) : null}
      <ul aria-label="已保存本地登录资料" className="max-h-[32rem] min-w-0 divide-y overflow-auto">
        {visible.map((item) => (
          <li key={item.id} className="flex min-w-0 flex-wrap items-start gap-3 py-3">
            <Checkbox
              aria-label={`选择本地资料 ${item.email}`}
              disabled={!fresh}
              checked={selected.includes(item.id)}
              onCheckedChange={(checked) =>
                setSelected(
                  checked ? [...selected, item.id] : selected.filter((id) => id !== item.id),
                )
              }
            />
            <div className="min-w-0 flex-1 basis-40 text-sm wrap-anywhere">
              <p>{item.email}</p>
              <p className="text-xs text-muted-foreground">
                官方用户 {item.user_id}；工作区 {item.workspace_id}；版本 {item.revision}
              </p>
              <p className="text-xs text-muted-foreground">
                密码：{item.has_password ? "已保存" : "未保存"}；TOTP：
                {item.has_totp ? "已保存" : "未保存"}
              </p>
            </div>
            <Button
              variant="outline"
              aria-label={`替换本地资料 ${item.email}`}
              disabled={!fresh}
              onClick={() => setEditing(item)}
            >
              <Pencil aria-hidden="true" />
              替换
            </Button>
            <Button
              variant="outline"
              aria-label={`删除本地资料 ${item.email}`}
              disabled={!fresh}
              onClick={() => setDeleting(item)}
            >
              <Trash2 aria-hidden="true" />
              删除
            </Button>
          </li>
        ))}
      </ul>
      {editing ? (
        <WorkbenchLoginProfileEditor
          profile={editing}
          onClose={() => {
            setEditing(null);
            void client.invalidateQueries({ queryKey: ["account-workbench", "source-profiles"] });
          }}
        />
      ) : null}
      <ConfirmActionDialog
        open={!!deleting}
        title="删除本地登录资料"
        description={`删除 ${deleting?.email ?? ""} 的本地登录资料（版本 ${deleting?.revision ?? ""}）？之后无法使用此资料重新登录。`}
        confirmLabel="确认删除资料"
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={() => {
          if (deleting) remove.mutate(deleting);
        }}
      />
      {task ? <WorkbenchTask task={task} /> : null}
    </section>
  );
}
