import { useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LogIn, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Task, type WorkbenchLoginProfile } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { ConfirmActionDialog } from "@/components/confirm-action-dialog";
import { FormField } from "@/components/form-field";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Skeleton } from "@/components/ui/skeleton";
import { notifyOperationError } from "@/lib/operation-feedback";
import { workbenchKeys } from "../constants";
import { smsProviderOptions } from "../lib/oauth-sms-schema";
import { WorkbenchLoginProfileEditor } from "./workbench-login-profile-editor";
import { WorkbenchProfileExport } from "./workbench-profile-export";
import { WorkbenchTask } from "./workbench-task";
import { WorkbenchCleanup } from "./workbench-cleanup";

export function WorkbenchLoginProfiles(props: {
  onAuthorize: (ids: string[]) => void;
}): ReactElement {
  const client = useQueryClient();
  const profiles = useQuery({
    queryKey: workbenchKeys.profiles,
    queryFn: api.workbenchLoginProfiles,
    gcTime: 0,
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [accountID, setAccountID] = useState("");
  const [editor, setEditor] = useState<{
    accountID: string;
    profile?: WorkbenchLoginProfile;
  } | null>(null);
  const [deleting, setDeleting] = useState<WorkbenchLoginProfile | null>(null);
  const [exportTask, setExportTask] = useState<Task | null>(null);
  const [cleanupProfiles, setCleanupProfiles] = useState<WorkbenchLoginProfile[] | null>(null);
  const remove = useMutation({
    mutationFn: (profile: WorkbenchLoginProfile) =>
      api.deleteWorkbenchLoginProfile(profile.id, profile.revision),
    onSuccess: (_value, profile) => {
      setDeleting(null);
      setSelected((ids) => ids.filter((id) => id !== profile.account_id));
      void client.invalidateQueries({ queryKey: workbenchKeys.profiles });
      toast.success("登录资料已删除");
    },
    onError: (error) => {
      notifyOperationError(error, "登录资料删除失败，请刷新资料版本后重试");
      if (error instanceof ApiError && error.status === 409) {
        setDeleting(null);
        void client.invalidateQueries({ queryKey: workbenchKeys.profiles });
      }
    },
  });
  if (profiles.isPending || accounts.isPending)
    return (
      <div
        role="status"
        aria-label="正在读取登录资料"
        aria-busy="true"
        className="grid min-w-0 gap-3"
      >
        <span className="sr-only">正在读取登录资料</span>
        <Skeleton className="h-8 w-full" />
        {[0, 1, 2].map((item) => (
          <Skeleton key={item} className="h-20 w-full" />
        ))}
      </div>
    );
  if (!profiles.data || !accounts.data)
    return (
      <ContentRetry
        pending={profiles.isFetching || accounts.isFetching}
        onRetry={() => {
          void profiles.refetch();
          void accounts.refetch();
        }}
      />
    );
  const values = profiles.data;
  const needle = search.trim().toLowerCase();
  const visible = values.filter((profile) =>
    `${profile.email} ${profile.account_id} ${profile.workspace_id}`.toLowerCase().includes(needle),
  );
  const eligible = accounts.data.filter(
    (account) =>
      account.platform === "openai" &&
      account.account_type === "oauth" &&
      !values.some((profile) => profile.account_id === account.id),
  );
  const chosen = selected.filter((id) => values.some((profile) => profile.account_id === id));
  const allSelected =
    visible.length > 0 && visible.every((profile) => chosen.includes(profile.account_id));
  const fresh = !profiles.isError && !accounts.isError;
  return (
    <section aria-label="登录资料" className="grid min-w-0 gap-4">
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <FormField label="绑定账号">
            <Select value={accountID} onValueChange={(value) => setAccountID(value ?? "")}>
              <SelectTrigger aria-label="绑定账号" className="min-w-0">
                <SelectValue className="min-w-0 truncate">
                  <span className="min-w-0 truncate">
                    {eligible.find((account) => account.id === accountID)?.name ??
                      "选择 OpenAI OAuth 账号"}
                  </span>
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {eligible.map((account) => (
                  <SelectItem key={account.id} value={account.id}>
                    {account.name}（ID {account.id}）
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <Button
          disabled={!fresh || !eligible.some((account) => account.id === accountID)}
          onClick={() => setEditor({ accountID })}
        >
          <Plus aria-hidden="true" />
          新增登录资料
        </Button>
      </div>
      <FormField label="搜索登录资料" htmlFor="workbench-profile-search">
        <Input
          id="workbench-profile-search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="邮箱、账号 ID 或工作区"
        />
      </FormField>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={allSelected}
            disabled={!fresh || visible.length === 0}
            onCheckedChange={(checked) =>
              setSelected(
                checked
                  ? [...new Set([...chosen, ...visible.map((profile) => profile.account_id)])]
                  : chosen.filter((id) => !visible.some((profile) => profile.account_id === id)),
              )
            }
          />
          选择当前资料
        </label>
        <Button
          disabled={!fresh || chosen.length === 0 || chosen.length > 500}
          onClick={() => props.onAuthorize(chosen)}
        >
          <LogIn aria-hidden="true" />
          预览重新授权（{chosen.length}）
        </Button>
        <WorkbenchProfileExport
          profiles={values.filter((profile) => chosen.includes(profile.account_id))}
          disabled={!fresh}
          onCreated={setExportTask}
        />
        <Button
          variant="outline"
          disabled={!fresh || !chosen.length || chosen.length > 500}
          onClick={() =>
            setCleanupProfiles(values.filter((profile) => chosen.includes(profile.account_id)))
          }
        >
          <Trash2 aria-hidden="true" />
          清理关联资料（{chosen.length}）
        </Button>
      </div>
      {!fresh && (
        <ContentRetry
          pending={profiles.isFetching || accounts.isFetching}
          onRetry={() => {
            void profiles.refetch();
            void accounts.refetch();
          }}
        />
      )}
      <ul aria-label="已保存登录资料" className="max-h-[32rem] min-w-0 divide-y overflow-y-auto">
        {visible.map((profile) => (
          <li key={profile.id} className="flex min-w-0 items-start gap-3 py-3">
            <Checkbox
              aria-label={`选择 ${profile.email}（ID ${profile.account_id}）`}
              checked={chosen.includes(profile.account_id)}
              disabled={!fresh}
              onCheckedChange={(checked) =>
                setSelected(
                  checked
                    ? [...chosen, profile.account_id]
                    : chosen.filter((id) => id !== profile.account_id),
                )
              }
            />
            <div className="min-w-0 flex-1 space-y-1 text-sm wrap-anywhere">
              <p>{profile.email}</p>
              <p className="text-xs text-muted-foreground">
                账号 ID：{profile.account_id}；工作区：{profile.workspace_id}；资料版本：
                {profile.revision}
              </p>
              <p className="text-xs text-muted-foreground">
                {[
                  profile.has_password ? "已保存密码" : "邮箱验证码",
                  profile.has_totp ? "已保存 TOTP" : "",
                  profile.has_proxy ? "已保存登录代理" : "",
                  profile.mail_kind === "microsoft" ? "Microsoft 邮箱" : "",
                  profile.mail_kind === "http" ? "HTTP 邮箱" : "",
                  smsProviderOptions.find((option) => option.value === profile.sms_provider)
                    ?.label ?? "",
                ]
                  .filter(Boolean)
                  .join("、")}
              </p>
            </div>
            <div className="flex shrink-0 flex-col gap-1 sm:flex-row">
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={`替换 ${profile.email} 的登录资料`}
                      disabled={!fresh}
                      onClick={() => setEditor({ accountID: profile.account_id, profile })}
                    >
                      <Pencil aria-hidden="true" />
                    </Button>
                  }
                />
                <TooltipContent>替换登录资料</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={`删除 ${profile.email} 的登录资料`}
                      disabled={!fresh || remove.isPending}
                      onClick={() => setDeleting(profile)}
                    >
                      <Trash2 aria-hidden="true" />
                    </Button>
                  }
                />
                <TooltipContent>删除登录资料</TooltipContent>
              </Tooltip>
            </div>
          </li>
        ))}
        {visible.length === 0 && (
          <li className="py-5 text-sm text-muted-foreground">
            {values.length ? "没有匹配的登录资料" : "暂无登录资料"}
          </li>
        )}
      </ul>
      {editor && (
        <WorkbenchLoginProfileEditor
          key={editor.profile?.id ?? editor.accountID}
          accountID={editor.accountID}
          profile={editor.profile}
          onClose={() => setEditor(null)}
        />
      )}
      {exportTask && <WorkbenchTask task={exportTask} />}
      {cleanupProfiles && (
        <WorkbenchCleanup
          profiles={cleanupProfiles}
          onClose={() => setCleanupProfiles(null)}
          onCreated={setExportTask}
        />
      )}
      <ConfirmActionDialog
        open={!!deleting}
        title="删除登录资料"
        description={`删除 ${deleting?.email ?? ""}（账号 ID ${deleting?.account_id ?? ""}）在服务器保存的登录资料。后续重新授权需要重新填写资料，线上账号保持原有状态。`}
        confirmLabel="确认删除登录资料"
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={() => {
          if (deleting) remove.mutate(deleting);
        }}
      />
    </section>
  );
}
