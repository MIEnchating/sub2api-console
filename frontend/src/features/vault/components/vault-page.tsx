import { useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eraser, KeyRound, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, type VaultEntryIndex } from "@/api";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { FieldLabel } from "@/components/field-help-tooltip";
import { StatusBadge } from "@/components/status-badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
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
type VaultStatusFilter = "all" | "complete" | "incomplete";

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

function isComplete(entry: VaultEntryIndex): boolean {
  return entry.has_username && entry.has_password;
}

function hostConflictEntries(entries: VaultEntryIndex[]): Set<string> {
  const matches = new Map<string, string[]>();
  const wildcardEntries = entries.filter((entry) => entry.hosts.length === 0);
  const conflicts = new Set<string>();
  if (wildcardEntries.length > 1) {
    for (const entry of wildcardEntries) conflicts.add(entry.entry);
  }
  for (const entry of entries) {
    for (const host of entry.hosts) {
      const current = matches.get(host) ?? [];
      current.push(entry.entry);
      matches.set(host, current);
    }
  }
  for (const names of matches.values()) {
    if (names.length > 1 || wildcardEntries.length > 0) {
      for (const name of names) conflicts.add(name);
      for (const entry of wildcardEntries) conflicts.add(entry.entry);
    }
  }
  return conflicts;
}

function VaultValueList(props: { values: string[]; emptyLabel: string; warning?: boolean }) {
  if (props.values.length === 0) {
    return (
      <span className={props.warning ? "text-warning text-sm" : "text-muted-foreground text-sm"}>
        {props.emptyLabel}
      </span>
    );
  }
  const visibleValues = props.values.slice(0, 7);
  const hiddenCount = props.values.length - visibleValues.length;
  return (
    <div className="flex max-w-full flex-wrap gap-1">
      {visibleValues.map((value) => (
        <span
          className="border-border/70 bg-muted/35 max-w-52 truncate rounded-md border px-1.5 py-0.5 text-xs leading-5"
          key={value}
        >
          {value}
        </span>
      ))}
      {hiddenCount > 0 ? (
        <span className="border-border/70 bg-muted/20 text-muted-foreground rounded-md border px-1.5 py-0.5 text-xs leading-5">
          +{hiddenCount} 个
        </span>
      ) : null}
    </div>
  );
}

export function VaultEntryTable(props: {
  entries: VaultEntryIndex[];
  conflictEntries?: Set<string>;
  onEdit: (entry: VaultEntryIndex) => void;
  onDelete: (entry: VaultEntryIndex) => void;
}) {
  return (
    <Table
      overflowTooltip={false}
      containerClassName="min-h-0 flex-1 overflow-auto"
      className="min-w-[980px]"
    >
      <TableHeader>
        <TableRow>
          <TableHead className="w-[18%] min-w-40">凭据名称</TableHead>
          <TableHead className="w-[38%] min-w-[24rem]">匹配 Host</TableHead>
          <TableHead className="w-40">凭据状态</TableHead>
          <TableHead className="w-[24%] min-w-52">Headers</TableHead>
          <TableHead className="w-24 text-right">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody className="[&>tr]:h-auto">
        {props.entries.map((entry) => {
          const status = vaultStatus(entry);
          return (
            <TableRow key={entry.entry}>
              <TableCell className="font-medium">{entry.entry}</TableCell>
              <TableCell className="whitespace-normal">
                <div className="grid gap-1">
                  <VaultValueList
                    values={entry.hosts}
                    emptyLabel="全部 Host"
                    warning={entry.hosts.length === 0}
                  />
                  {props.conflictEntries?.has(entry.entry) ? (
                    <span className="text-xs text-warning">存在 Host 匹配冲突</span>
                  ) : null}
                </div>
              </TableCell>
              <TableCell>
                <StatusBadge label={status.label} variant={status.variant} />
              </TableCell>
              <TableCell className="whitespace-normal">
                <VaultValueList values={entry.header_names} emptyLabel="未设置" />
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
    <div className="grid min-w-0 gap-1.5 text-sm">
      <FieldLabel label={props.label} description={props.hint} />
      {props.children}
      {props.error ? (
        <span className="text-xs text-destructive" role="alert">
          {props.error}
        </span>
      ) : null}
    </div>
  );
}

export function VaultPage() {
  const queryClient = useQueryClient();
  const config = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: api.authRecoveryConfig,
  });
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<VaultStatusFilter>("all");
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<VaultEntryIndex | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VaultEntryIndex | null>(null);
  const [form, setForm] = useState<VaultForm>(emptyVaultForm);
  const [headersError, setHeadersError] = useState<string | null>(null);
  const [touched, setTouched] = useState<Set<MutableVaultField>>(() => new Set());
  const [cleared, setCleared] = useState<Set<MutableVaultField>>(() => new Set());
  const [overwritePayload, setOverwritePayload] = useState<
    Parameters<typeof api.configureVaultEntry>[0] | null
  >(null);

  const entries = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    const rows = config.data?.vault_entries ?? [];
    const filtered = keyword
      ? rows.filter((entry) =>
          [entry.entry, ...entry.hosts, ...entry.header_names].some((value) =>
            String(value ?? "")
              .toLowerCase()
              .includes(keyword),
          ),
        )
      : rows;
    if (statusFilter === "all") return filtered;
    return filtered.filter((entry) =>
      statusFilter === "complete" ? isComplete(entry) : !isComplete(entry),
    );
  }, [config.data?.vault_entries, search, statusFilter]);

  const allEntries = config.data?.vault_entries ?? [];
  const conflictEntries = useMemo(() => hostConflictEntries(allEntries), [allEntries]);

  function markTouched(field: MutableVaultField) {
    if (field === "headers") setHeadersError(null);
    setTouched((current) => new Set(current).add(field));
  }

  function openCreate() {
    setEditing(null);
    setForm(emptyVaultForm);
    setHeadersError(null);
    setTouched(new Set());
    setCleared(new Set());
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
    setCleared(new Set());
    setEditorOpen(true);
  }

  const save = useMutation({
    mutationFn: api.configureVaultEntry,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] });
      setEditorOpen(false);
      setForm(emptyVaultForm);
      setCleared(new Set());
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
    const payload =
      editing === null
        ? {
            ...identity,
            username: form.username,
            password: form.password,
            hosts: splitHosts(form.hosts),
            headers: headers ?? {},
          }
        : {
            ...identity,
            ...(touched.has("username")
              ? { username: cleared.has("username") ? null : form.username }
              : {}),
            ...(touched.has("password")
              ? { password: cleared.has("password") ? null : form.password }
              : {}),
            ...(touched.has("hosts") ? { hosts: splitHosts(form.hosts) } : {}),
            ...(headers === undefined ? {} : { headers: cleared.has("headers") ? {} : headers }),
          };
    if (editing === null && allEntries.some((entry) => entry.entry === identity.entry)) {
      setOverwritePayload(payload);
      return;
    }
    save.mutate(payload);
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
  let saveButtonLabel = "添加凭据";
  if (save.isPending) saveButtonLabel = "保存中...";
  else if (editing) saveButtonLabel = "保存修改";

  return (
    <PageLayout fixedContent>
      <PageHeading
        eyebrow="SECURITY / VAULT"
        title="密码箱"
        description="管理上游鉴权恢复使用的账号密码。"
        action={
          <PageActions>
            <RefreshButton
              pending={config.isFetching}
              ariaLabel="刷新密码箱"
              onClick={() => void config.refetch()}
            />
            <Button onClick={openCreate}>
              <Plus />
              添加凭据
            </Button>
          </PageActions>
        }
      />

      <div className="flex h-full min-h-0 flex-col gap-3">
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
          <FilterMenu
            label="状态"
            options={["complete", "incomplete"]}
            value={statusFilter === "all" ? null : statusFilter}
            onValueChange={(value) => setStatusFilter(value ?? "all")}
            optionLabel={(value) => (value === "complete" ? "凭据完整" : "凭据不完整")}
          />
        </TableFilterToolbar>

        <DataTablePanel className="flex-1">
          {config.error && <QueryErrorToast error={config.error} fallback="密码箱读取失败" />}
          {!config.error && config.isLoading && (
            <Table containerClassName="min-h-0 flex-1 overflow-auto">
              <TableBody>
                {Array.from({ length: 4 }, (_, row) => (
                  <TableRow key={row}>
                    {Array.from({ length: 5 }, (_, column) => (
                      <TableCell key={column}>
                        <Skeleton className={column === 0 ? "h-4 w-32" : "h-4 w-24"} />
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {!config.error && !config.isLoading && entries.length === 0 && (
            <div className="grid min-h-48 place-items-center p-6 text-center">
              <div>
                <KeyRound className="text-muted-foreground mx-auto mb-3 size-6" />
                <p className="text-sm font-medium">{search ? "没有匹配的凭据" : "暂无凭据"}</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {search ? "调整搜索内容后重试" : "添加后可用于上游鉴权恢复"}
                </p>
              </div>
            </div>
          )}
          {!config.error && !config.isLoading && entries.length > 0 && (
            <VaultEntryTable
              entries={entries}
              conflictEntries={conflictEntries}
              onEdit={openEdit}
              onDelete={setDeleteTarget}
            />
          )}
        </DataTablePanel>
      </div>

      <Dialog
        open={editorOpen}
        onOpenChange={(open) => {
          if (save.isPending) return;
          setEditorOpen(open);
          if (!open) {
            setForm(emptyVaultForm);
            setCleared(new Set());
            setTouched(new Set());
          }
        }}
      >
        <DialogContent
          width="wide"
          height="large"
          className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{editing ? "编辑凭据" : "添加凭据"}</DialogTitle>
            <DialogDescription>
              敏感字段不会回显；已配置的字段留空则不修改，需要移除时使用对应的“清除”按钮。
            </DialogDescription>
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
              <div className="flex min-w-0 items-center gap-2">
                <Input
                  className="min-w-0 flex-1"
                  autoComplete="username"
                  value={form.username}
                  placeholder={sensitiveFieldPlaceholder(
                    editing?.has_username === true,
                    "输入用户名",
                  )}
                  onChange={(event) => {
                    markTouched("username");
                    setCleared((current) => {
                      const next = new Set(current);
                      next.delete("username");
                      return next;
                    });
                    setForm({ ...form, username: event.target.value });
                  }}
                />
                {editing?.has_username ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    onClick={() => {
                      markTouched("username");
                      setCleared((current) => new Set(current).add("username"));
                      setForm({ ...form, username: "" });
                    }}
                  >
                    <Eraser />
                    清除
                  </Button>
                ) : null}
              </div>
            </VaultField>
            <VaultField
              label="密码"
              hint={editing?.has_password ? "已配置，留空则不修改" : undefined}
            >
              <div className="flex min-w-0 items-center gap-2">
                <Input
                  className="min-w-0 flex-1"
                  type="password"
                  autoComplete="new-password"
                  value={form.password}
                  placeholder={sensitiveFieldPlaceholder(
                    editing?.has_password === true,
                    "输入密码",
                  )}
                  onChange={(event) => {
                    markTouched("password");
                    setCleared((current) => {
                      const next = new Set(current);
                      next.delete("password");
                      return next;
                    });
                    setForm({ ...form, password: event.target.value });
                  }}
                />
                {editing?.has_password ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="shrink-0"
                    onClick={() => {
                      markTouched("password");
                      setCleared((current) => new Set(current).add("password"));
                      setForm({ ...form, password: "" });
                    }}
                  >
                    <Eraser />
                    清除
                  </Button>
                ) : null}
              </div>
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
                    setCleared((current) => {
                      const next = new Set(current);
                      next.delete("headers");
                      return next;
                    });
                    setForm({ ...form, headers: event.target.value });
                  }}
                  placeholder={sensitiveFieldPlaceholder(
                    Boolean(editing?.header_names.length),
                    '{"X-Custom-Header":"value"}',
                  )}
                />
                {editing?.header_names.length ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="mt-1 w-fit"
                    onClick={() => {
                      markTouched("headers");
                      setCleared((current) => new Set(current).add("headers"));
                      setForm({ ...form, headers: "" });
                    }}
                  >
                    <Eraser />
                    清除 Headers
                  </Button>
                ) : null}
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
              {saveButtonLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={overwritePayload !== null}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setOverwritePayload(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>覆盖已有凭据？</DialogTitle>
            <DialogDescription>
              凭据名称“{overwritePayload?.entry ?? ""}”已存在。继续后会覆盖其账号密码、Host 和
              Headers 配置。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setOverwritePayload(null)}
              disabled={save.isPending}
            >
              取消
            </Button>
            <Button
              onClick={() => {
                if (!overwritePayload) return;
                save.mutate(overwritePayload);
                setOverwritePayload(null);
              }}
              disabled={save.isPending || overwritePayload === null}
            >
              {save.isPending ? "保存中..." : "确认覆盖"}
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
            <DialogDescription>
              删除后，上游鉴权恢复将无法再使用该账号密码；请确认影响范围。
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border bg-muted/30 px-3 py-2 text-sm font-medium">
            <div>{deleteTarget?.entry ?? ""}</div>
            {deleteTarget?.hosts.length ? (
              <div className="mt-1 text-xs font-normal text-muted-foreground">
                关联 Host（{deleteTarget.hosts.length}）：{deleteTarget.hosts.join("、")}
              </div>
            ) : (
              <div className="mt-1 text-xs font-normal text-warning">
                当前凭据匹配全部 Host，删除影响范围较大。
              </div>
            )}
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
