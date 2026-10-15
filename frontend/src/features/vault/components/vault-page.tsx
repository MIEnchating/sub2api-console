import { useEffect, useId, useMemo, useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, useWatch } from "react-hook-form";
import {
  emptyVaultForm,
  vaultSchema,
  type VaultForm,
  type MutableVaultField,
} from "../lib/vault-schema";
import { VaultEditorFields } from "./vault-editor-fields";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api, ApiError, type VaultEntryIndex } from "@/api";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
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
import { notifyOperationError, operationErrorMessage } from "@/lib/operation-feedback";
import { parseJsonStringMap } from "@/lib/json-string-map";

type VaultStatusFilter = "all" | "complete" | "incomplete";

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
  const wildcardEntries: VaultEntryIndex[] = [];
  const conflicts = new Set<string>();
  for (const entry of entries) {
    if (entry.hosts.length === 0) {
      wildcardEntries.push(entry);
      continue;
    }
    for (const host of entry.hosts) {
      const current = matches.get(host) ?? [];
      current.push(entry.id);
      matches.set(host, current);
    }
  }
  if (wildcardEntries.length > 1) {
    for (const entry of wildcardEntries) conflicts.add(entry.id);
  }
  for (const names of matches.values()) {
    if (names.length > 1 || wildcardEntries.length > 0) {
      for (const name of names) conflicts.add(name);
      for (const entry of wildcardEntries) conflicts.add(entry.id);
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
  disabled?: boolean;
  onEdit: (entry: VaultEntryIndex) => void;
  onDelete: (entry: VaultEntryIndex) => void;
}) {
  return (
    <Table
      actionColumn
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
            <TableRow key={entry.id}>
              <TableCell className="font-medium">{entry.entry}</TableCell>
              <TableCell className="whitespace-normal">
                <div className="grid gap-1">
                  <VaultValueList
                    values={entry.hosts}
                    emptyLabel="全部 Host"
                    warning={entry.hosts.length === 0}
                  />
                  {props.conflictEntries?.has(entry.id) ? (
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
                  <TableActionButton
                    label="编辑凭据"
                    disabled={props.disabled}
                    onClick={() => props.onEdit(entry)}
                  >
                    <Pencil />
                  </TableActionButton>
                  <TableActionButton
                    label="删除凭据"
                    disabled={props.disabled}
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

export function VaultPage() {
  const fieldId = useId();
  const queryClient = useQueryClient();
  const config = useQuery({
    queryKey: ["auth-recovery-config"],
    queryFn: ({ signal }) => api.authRecoveryConfig(signal),
  });
  const configReady = config.data !== undefined && !config.isError && !config.isFetching;
  const mounted = useRef(false);
  const writing = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      pendingSave.current = null;
    };
  }, []);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<VaultStatusFilter>("all");
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<VaultEntryIndex | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<VaultEntryIndex | null>(null);
  const editorForm = useForm<VaultForm>({
    resolver: zodResolver(vaultSchema),
    defaultValues: emptyVaultForm,
  });
  const form = useWatch({ control: editorForm.control });
  const [touched, setTouched] = useState<Set<MutableVaultField>>(() => new Set());
  const [cleared, setCleared] = useState<Set<MutableVaultField>>(() => new Set());
  const pendingSave = useRef<Parameters<typeof api.configureVaultEntry>[0] | null>(null);

  const entries = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    const rows = config.data?.vault_entries ?? [];
    if (!keyword && statusFilter === "all") return rows;
    const filtered: VaultEntryIndex[] = [];
    for (const entry of rows) {
      if (keyword) {
        const matchesSearch = [entry.entry, ...entry.hosts, ...entry.header_names].some((value) =>
          String(value ?? "")
            .toLowerCase()
            .includes(keyword),
        );
        if (!matchesSearch) continue;
      }
      if (
        statusFilter !== "all" &&
        (statusFilter === "complete" ? !isComplete(entry) : isComplete(entry))
      ) {
        continue;
      }
      filtered.push(entry);
    }
    return filtered;
  }, [config.data?.vault_entries, search, statusFilter]);

  const allEntries = config.data?.vault_entries ?? [];
  const conflictEntries = useMemo(() => hostConflictEntries(allEntries), [allEntries]);

  function markTouched(field: MutableVaultField) {
    if (field === "headers") editorForm.clearErrors("headers");
    setTouched((current) => new Set(current).add(field));
  }

  function openCreate() {
    if (!configReady) return;
    setEditing(null);
    editorForm.reset(emptyVaultForm);
    editorForm.clearErrors("headers");
    setTouched(new Set());
    setCleared(new Set());
    setEditorOpen(true);
  }

  function openEdit(entry: VaultEntryIndex) {
    setEditing(entry);
    editorForm.reset({
      entry: entry.entry,
      username: "",
      password: "",
      hosts: entry.hosts.join(", "),
      headers: "",
    });
    editorForm.clearErrors("headers");
    setTouched(new Set());
    setCleared(new Set());
    setEditorOpen(true);
  }

  const save = useMutation({
    gcTime: 0,
    retry: false,
    networkMode: "always",
    mutationFn: () => {
      const payload = pendingSave.current;
      pendingSave.current = null;
      if (!payload) throw new Error("凭据提交内容已清除，请重新填写");
      return api.configureVaultEntry(payload);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] });
      if (!mounted.current) return;
      setEditorOpen(false);
      editorForm.reset(emptyVaultForm);
      setCleared(new Set());
      editorForm.clearErrors("headers");
      toast.success(editing === null ? "凭据已添加" : "凭据已更新");
    },
    onError: (error) => {
      if (!mounted.current) return;
      if (error instanceof ApiError && error.code === "vault_entry_exists") {
        editorForm.setError("entry", { message: error.message }, { shouldFocus: true });
        return;
      }
      notifyOperationError(error, "凭据保存失败");
    },
    onSettled: () => {
      writing.current = false;
    },
  });

  function savePayload(payload: Parameters<typeof api.configureVaultEntry>[0]): void {
    if (writing.current || !configReady) return;
    writing.current = true;
    pendingSave.current = payload;
    save.mutate();
  }

  function closeEditor(): void {
    if (save.isPending) return;
    setEditorOpen(false);
    editorForm.reset(emptyVaultForm);
    setCleared(new Set());
    setTouched(new Set());
    pendingSave.current = null;
    save.reset();
  }

  function submitSave(form: VaultForm) {
    if (!configReady) return;
    const identity = { entry: form.entry.trim() };
    if (allEntries.some((item) => item.id !== editing?.id && item.entry === identity.entry)) {
      editorForm.setError(
        "entry",
        { message: "凭据名称已存在，请使用其他名称" },
        { shouldFocus: true },
      );
      return;
    }
    if (!identity.entry) return;
    let headers: Record<string, string> | undefined;
    if (editing === null || form.headers.trim() !== "" || cleared.has("headers")) {
      try {
        headers = parseHeaders(form.headers);
      } catch (error) {
        editorForm.setError("headers", {
          message: operationErrorMessage(error, "Headers 格式无效"),
        });
        return;
      }
    }
    editorForm.clearErrors("headers");
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
            id: editing.id,
            ...(form.username !== "" || cleared.has("username")
              ? { username: cleared.has("username") ? null : form.username }
              : {}),
            ...(form.password !== "" || cleared.has("password")
              ? { password: cleared.has("password") ? null : form.password }
              : {}),
            ...(touched.has("hosts") ? { hosts: splitHosts(form.hosts) } : {}),
            ...(headers === undefined ? {} : { headers: cleared.has("headers") ? {} : headers }),
          };
    savePayload(payload);
  }

  const remove = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: (entry: VaultEntryIndex) => api.deleteVaultEntry(entry.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["auth-recovery-config"] });
      if (!mounted.current) return;
      setDeleteTarget(null);
      toast.success("凭据已删除");
    },
    onError: (error) => {
      if (mounted.current) notifyOperationError(error, "凭据删除失败");
    },
    onSettled: () => {
      writing.current = false;
    },
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
            <Button onClick={openCreate} disabled={!configReady}>
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
          {config.isLoading && (
            <Table actionColumn containerClassName="min-h-0 flex-1 overflow-auto">
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
          {!config.data && config.isError && (
            <ContentRetry pending={config.isFetching} onRetry={() => void config.refetch()} />
          )}
          {config.data && entries.length === 0 && (
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
          {config.data && entries.length > 0 && (
            <VaultEntryTable
              entries={entries}
              disabled={!configReady}
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
          if (!open) closeEditor();
        }}
      >
        <DialogContent
          width="wide"
          height="adaptive"
          className="w-[min(48rem,calc(100vw-2rem))] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>{editing ? "编辑凭据" : "添加凭据"}</DialogTitle>
            <DialogDescription>
              账号密码和请求头不会回显，留空保留原值；清除操作在保存后生效。
            </DialogDescription>
          </DialogHeader>
          <DialogBody>
            <form id={`${fieldId}-form`} onSubmit={editorForm.handleSubmit(submitSave)}>
              <VaultEditorFields
                form={editorForm}
                editing={editing}
                cleared={cleared}
                disabled={save.isPending}
                onChange={(field) => {
                  markTouched(field);
                  setCleared((current) => {
                    const next = new Set(current);
                    next.delete(field);
                    return next;
                  });
                }}
                onToggleClear={(field) => {
                  markTouched(field);
                  editorForm.setValue(field, "");
                  setCleared((current) => {
                    const next = new Set(current);
                    if (next.has(field)) next.delete(field);
                    else next.add(field);
                    return next;
                  });
                }}
              />
            </form>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={closeEditor} disabled={save.isPending}>
              取消
            </Button>
            <Button
              type="submit"
              form={`${fieldId}-form`}
              disabled={!configReady || save.isPending || !form.entry?.trim()}
            >
              {saveButtonLabel}
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
                if (!deleteTarget || writing.current || !configReady) return;
                writing.current = true;
                remove.mutate(deleteTarget);
              }}
              disabled={!configReady || remove.isPending || deleteTarget === null}
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
