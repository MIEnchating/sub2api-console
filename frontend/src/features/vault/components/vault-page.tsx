import { useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Pencil, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, type VaultEntryIndex } from "@/api";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { notifyOperationError, operationErrorMessage } from "@/lib/operation-feedback";
import { parseJsonStringMap } from "@/lib/json-string-map";
import { sensitiveFieldPlaceholder } from "@/lib/sensitive-field";

type VaultForm = {
  entry: string;
  username: string;
  password: string;
  hosts: string;
  headers: string;
};

type MutableVaultField = "username" | "password" | "hosts" | "headers";

const emptyVaultForm: VaultForm = {
  entry: "",
  username: "",
  password: "",
  hosts: "",
  headers: "",
};

function splitHosts(value: string): string[] {
  return Array.from(
    new Set(
      value
        .split(/[，,\n]/)
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  );
}

function parseHeaders(value: string): Record<string, string> {
  if (!value.trim()) return {};
  return parseJsonStringMap(value, "Headers");
}

function vaultStatus(entry: VaultEntryIndex): {
  label: string;
  variant: "success" | "warning";
} {
  if (entry.has_username && entry.has_password) {
    return { label: "凭据完整", variant: "success" };
  }
  const missing = [
    !entry.has_username ? "用户名" : null,
    !entry.has_password ? "密码" : null,
  ].filter(Boolean);
  return { label: `缺少${missing.join("、")}`, variant: "warning" };
}

export function VaultEntryTable(props: {
  entries: VaultEntryIndex[];
  onEdit: (entry: VaultEntryIndex) => void;
  onDelete: (entry: VaultEntryIndex) => void;
}) {
  return (
    <Table containerClassName="max-h-[calc(100svh-15rem)] overflow-y-auto">
      <TableHeader>
        <TableRow>
          <TableHead>凭据名称</TableHead>
          <TableHead>匹配 Host</TableHead>
          <TableHead>凭据状态</TableHead>
          <TableHead>Headers</TableHead>
          <TableHead className="w-24 text-right">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.entries.map((entry) => {
          const status = vaultStatus(entry);
          return (
            <TableRow key={entry.entry}>
              <TableCell className="font-medium">{entry.entry}</TableCell>
              <TableCell className="max-w-72">
                {entry.hosts.length ? entry.hosts.join("、") : "全部 Host"}
              </TableCell>
              <TableCell>
                <StatusBadge label={status.label} variant={status.variant} />
              </TableCell>
              <TableCell className="max-w-56 text-muted-foreground">
                {entry.header_names.length ? entry.header_names.join("、") : "未设置"}
              </TableCell>
              <TableCell className="text-right" overflowTooltip={false}>
                <div className="flex justify-end gap-1">
                  <TableActionButton label="编辑凭据" onClick={() => props.onEdit(entry)}>
                    <Pencil />
                  </TableActionButton>
                  <TableActionButton
                    label="删除凭据"
                    tone="danger"
                    onClick={() => props.onDelete(entry)}
                  >
                    <Trash2 />
                  </TableActionButton>
                </div>
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

function VaultField(props: { label: string; hint?: string; error?: string; children: ReactNode }) {
  return (
    <label className="grid min-w-0 gap-1.5 text-sm">
      <span className="font-medium">{props.label}</span>
      {props.children}
      {props.hint ? <span className="text-xs text-muted-foreground">{props.hint}</span> : null}
      {props.error ? (
        <span className="text-xs text-destructive" role="alert">
          {props.error}
        </span>
      ) : null}
    </label>
  );
}

export function VaultPage() {
  const queryClient = useQueryClient();
  const config = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: api.authRecoveryConfig,
  });
  const [search, setSearch] = useState("");
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<VaultEntryIndex | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VaultEntryIndex | null>(null);
  const [form, setForm] = useState<VaultForm>(emptyVaultForm);
  const [headersError, setHeadersError] = useState<string | null>(null);
  const [touched, setTouched] = useState<Set<MutableVaultField>>(() => new Set());

  const entries = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    const rows = config.data?.vault_entries ?? [];
    if (!keyword) return rows;
    return rows.filter((entry) =>
      [entry.entry, ...entry.hosts, ...entry.header_names].some((value) =>
        String(value ?? "")
          .toLowerCase()
          .includes(keyword),
      ),
    );
  }, [config.data?.vault_entries, search]);

  function markTouched(field: MutableVaultField) {
    if (field === "headers") setHeadersError(null);
    setTouched((current) => new Set(current).add(field));
  }

  function openCreate() {
    setEditing(null);
    setForm(emptyVaultForm);
    setHeadersError(null);
    setTouched(new Set());
    setEditorOpen(true);
  }

  function openEdit(entry: VaultEntryIndex) {
    setEditing(entry);
    setForm({
      entry: entry.entry,
      username: "",
      password: "",
      hosts: entry.hosts.join(", "),
      headers: "",
    });
    setHeadersError(null);
    setTouched(new Set());
    setEditorOpen(true);
  }

  const save = useMutation({
    mutationFn: api.configureVaultEntry,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] });
      setEditorOpen(false);
      setHeadersError(null);
      toast.success(editing === null ? "凭据已添加" : "凭据已更新");
    },
    onError: (error) => notifyOperationError(error, "凭据保存失败"),
  });

  function submitSave() {
    const identity = { entry: form.entry.trim() };
    if (!identity.entry) return;
    let headers: Record<string, string> | undefined;
    if (editing === null || touched.has("headers")) {
      try {
        headers = parseHeaders(form.headers);
      } catch (error) {
        setHeadersError(operationErrorMessage(error, "Headers 格式无效"));
        return;
      }
    }
    setHeadersError(null);
    if (editing === null) {
      save.mutate({
        ...identity,
        username: form.username,
        password: form.password,
        hosts: splitHosts(form.hosts),
        headers: headers ?? {},
      });
      return;
    }
    save.mutate({
      ...identity,
      ...(touched.has("username") ? { username: form.username } : {}),
      ...(touched.has("password") ? { password: form.password } : {}),
      ...(touched.has("hosts") ? { hosts: splitHosts(form.hosts) } : {}),
      ...(headers === undefined ? {} : { headers }),
    });
  }

  const remove = useMutation({
    mutationFn: (entry: VaultEntryIndex) => api.deleteVaultEntry(entry.entry),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] });
      setDeleteTarget(null);
      toast.success("凭据已删除");
    },
    onError: (error) => notifyOperationError(error, "凭据删除失败"),
  });

  return (
    <PageLayout>
      <PageHeading
        eyebrow="SECURITY / VAULT"
        title="密码箱"
        description="管理上游鉴权恢复使用的账号密码。"
        action={
          <PageActions>
            <Tooltip>
              <TooltipTrigger render={<span className="inline-flex" />}>
                <Button
                  variant="outline"
                  size="icon-sm"
                  aria-label="刷新密码箱"
                  onClick={() => void config.refetch()}
                  disabled={config.isFetching}
                >
                  <RefreshCw className={config.isFetching ? "animate-spin" : ""} />
                </Button>
              </TooltipTrigger>
              <TooltipContent>刷新</TooltipContent>
            </Tooltip>
            <Button onClick={openCreate}>
              <Plus />
              添加凭据
            </Button>
          </PageActions>
        }
      />

      <div className="grid gap-3">
        <TableFilterToolbar>
          <div className="relative min-w-56 flex-1 sm:max-w-sm">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-8"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索凭据名称或 Host"
              aria-label="搜索凭据"
            />
          </div>
          <span className="ml-auto text-sm text-muted-foreground">
            {config.isLoading
              ? "读取中"
              : `${entries.length} / ${config.data?.vault_entries.length ?? 0} 项`}
          </span>
        </TableFilterToolbar>

        <DataTablePanel>
          {config.error ? (
            <QueryErrorToast error={config.error} fallback="密码箱读取失败" />
          ) : !config.isLoading && entries.length === 0 ? (
            <div className="grid min-h-48 place-items-center p-6 text-center">
              <div>
                <KeyRound className="text-muted-foreground mx-auto mb-3 size-6" />
                <p className="text-sm font-medium">{search ? "没有匹配的凭据" : "暂无凭据"}</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {search ? "调整搜索内容后重试" : "添加后可用于上游鉴权恢复"}
                </p>
              </div>
            </div>
          ) : (
            <VaultEntryTable entries={entries} onEdit={openEdit} onDelete={setDeleteTarget} />
          )}
        </DataTablePanel>
      </div>

      <Dialog
        open={editorOpen}
        onOpenChange={(open) => {
          if (!save.isPending) setEditorOpen(open);
        }}
      >
        <DialogContent
          width="medium"
          height="large"
          className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{editing ? "编辑凭据" : "添加凭据"}</DialogTitle>
            <DialogDescription>敏感字段不会回显；已配置的字段留空则不修改。</DialogDescription>
          </DialogHeader>
          <DialogBody className="grid gap-4 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <VaultField label="凭据名称">
                <Input
                  value={form.entry}
                  disabled={editing !== null}
                  onChange={(event) => setForm({ ...form, entry: event.target.value })}
                  placeholder="例如 operator"
                />
              </VaultField>
            </div>
            <VaultField
              label="用户名"
              hint={editing?.has_username ? "已配置，留空则不修改" : undefined}
            >
              <Input
                autoComplete="username"
                value={form.username}
                placeholder={sensitiveFieldPlaceholder(
                  editing?.has_username === true,
                  "输入用户名",
                )}
                onChange={(event) => {
                  markTouched("username");
                  setForm({ ...form, username: event.target.value });
                }}
              />
            </VaultField>
            <VaultField
              label="密码"
              hint={editing?.has_password ? "已配置，留空则不修改" : undefined}
            >
              <Input
                type="password"
                autoComplete="new-password"
                value={form.password}
                placeholder={sensitiveFieldPlaceholder(editing?.has_password === true, "输入密码")}
                onChange={(event) => {
                  markTouched("password");
                  setForm({ ...form, password: event.target.value });
                }}
              />
            </VaultField>
            <div className="sm:col-span-2">
              <VaultField label="匹配 Host" hint="多个 Host 使用逗号或换行分隔">
                <Textarea
                  className="min-h-20"
                  value={form.hosts}
                  onChange={(event) => {
                    markTouched("hosts");
                    setForm({ ...form, hosts: event.target.value });
                  }}
                  placeholder="api.example.com"
                />
              </VaultField>
            </div>
            <div className="sm:col-span-2">
              <VaultField
                label="Headers JSON"
                error={headersError ?? undefined}
                hint={
                  editing?.header_names.length
                    ? `已配置：${editing.header_names.join("、")}；留空则不修改`
                    : "仅在登录接口需要额外请求头时填写"
                }
              >
                <Textarea
                  className="min-h-24 font-mono"
                  aria-invalid={headersError !== null}
                  value={form.headers}
                  onChange={(event) => {
                    markTouched("headers");
                    setForm({ ...form, headers: event.target.value });
                  }}
                  placeholder={sensitiveFieldPlaceholder(
                    Boolean(editing?.header_names.length),
                    '{"X-Custom-Header":"value"}',
                  )}
                />
              </VaultField>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditorOpen(false)}
              disabled={save.isPending}
            >
              取消
            </Button>
            <Button onClick={submitSave} disabled={save.isPending || !form.entry.trim()}>
              {save.isPending ? "保存中..." : editing ? "保存修改" : "添加凭据"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setDeleteTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除凭据</DialogTitle>
            <DialogDescription>删除后，上游鉴权恢复将无法再使用该账号密码。</DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border bg-muted/30 px-3 py-2 text-sm font-medium">
            {deleteTarget?.entry ?? ""}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteTarget(null)}
              disabled={remove.isPending}
            >
              取消
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                if (deleteTarget) remove.mutate(deleteTarget);
              }}
              disabled={remove.isPending || deleteTarget === null}
            >
              <Trash2 />
              {remove.isPending ? "删除中..." : "确认删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageLayout>
  );
}
