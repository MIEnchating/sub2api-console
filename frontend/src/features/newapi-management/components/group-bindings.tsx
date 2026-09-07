import { useCallback, useEffect, useMemo, useState } from "react";
import { Link2, Save } from "lucide-react";

import type {
  NewAPIGroupBinding,
  NewAPIGroupBindingUpdate,
  NewAPILocalGroup,
  NewAPIRemoteGroup,
} from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useClientPagination } from "@/hooks/use-client-pagination";

type Props = {
  groups: NewAPIRemoteGroup[];
  localGroups: NewAPILocalGroup[];
  bindings: NewAPIGroupBinding[];
  pending: boolean;
  onSave: (bindings: NewAPIGroupBindingUpdate[]) => void;
};

type DraftBinding = { localGroupId: string; sub2APIRatio: string; syncRatio: boolean };

const unboundGroupValue = "__unbound__";

function createDraftBindings(
  groups: NewAPIRemoteGroup[],
  localGroups: NewAPILocalGroup[],
  bindings: NewAPIGroupBinding[],
): Record<string, DraftBinding> {
  const bindingByGroupID = new Map(bindings.map((binding) => [binding.newapi_group_id, binding]));
  const localGroupByID = new Map(localGroups.map((group) => [group.id, group]));
  const next: Record<string, DraftBinding> = {};
  for (const group of groups) {
    const binding = bindingByGroupID.get(group.id);
    next[group.id] = {
      localGroupId: binding?.sub2api_group_id ?? "",
      sub2APIRatio: binding ? (localGroupByID.get(binding.sub2api_group_id)?.ratio ?? "") : "",
      syncRatio: binding?.sync_ratio ?? false,
    };
  }
  return next;
}

function validSub2APIRatio(value: string): boolean {
  if (!/^\+?(?:\d+(?:\.\d*)?|\.\d+)(?:e[+-]?\d+)?$/i.test(value.trim())) return false;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0;
}

function updateLocalGroupRatio(
  drafts: Record<string, DraftBinding>,
  localGroupID: string,
  ratio: string,
): Record<string, DraftBinding> {
  return Object.fromEntries(
    Object.entries(drafts).map(([groupID, draft]) => [
      groupID,
      draft.localGroupId === localGroupID ? { ...draft, sub2APIRatio: ratio } : draft,
    ]),
  );
}

export function updateBoundGroupRatioSync(
  drafts: Record<string, DraftBinding>,
  checked: boolean,
): Record<string, DraftBinding> {
  return Object.fromEntries(
    Object.entries(drafts).map(([groupID, draft]) => [
      groupID,
      {
        ...draft,
        syncRatio: draft.localGroupId ? checked : false,
      },
    ]),
  );
}

export function NewAPIGroupBindings(props: Props) {
  const [drafts, setDrafts] = useState<Record<string, DraftBinding>>(() =>
    createDraftBindings(props.groups, props.localGroups, props.bindings),
  );
  const pagination = useClientPagination(props.groups);
  const localGroupLabels = useMemo(
    () =>
      new Map(
        props.localGroups.map((group) => [group.id, `${group.name} · ${group.ratio ?? "无倍率"}`]),
      ),
    [props.localGroups],
  );
  const groupValueLabel = useCallback(
    (value: string) => {
      if (value === unboundGroupValue) return "不绑定";
      return localGroupLabels.get(value) ?? value;
    },
    [localGroupLabels],
  );
  const boundDrafts = Object.values(drafts).filter((draft) => draft.localGroupId);
  const allBoundGroupsSyncRatio =
    boundDrafts.length > 0 && boundDrafts.every((draft) => draft.syncRatio);
  const hasInvalidRatio = boundDrafts.some((draft) => !validSub2APIRatio(draft.sub2APIRatio));

  useEffect(() => {
    setDrafts(createDraftBindings(props.groups, props.localGroups, props.bindings));
  }, [props.bindings, props.groups, props.localGroups]);

  function save() {
    const bindings = props.groups.flatMap<NewAPIGroupBindingUpdate>((group) => {
      const draft = drafts[group.id];
      if (!draft?.localGroupId) return [];
      return [
        {
          newapi_group_id: group.id,
          newapi_group_name: group.name,
          sub2api_group_id: draft.localGroupId,
          sub2api_ratio: draft.sub2APIRatio.trim(),
          sync_ratio: draft.syncRatio,
        },
      ];
    });
    props.onSave(bindings);
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <TableFilterToolbar aria-label="分组绑定操作">
        <div className="flex items-center gap-2">
          <span className="text-sm">统一倍率同步</span>
          <Switch
            aria-label="统一倍率同步"
            checked={allBoundGroupsSyncRatio}
            disabled={boundDrafts.length === 0}
            onCheckedChange={(checked) =>
              setDrafts((current) => updateBoundGroupRatioSync(current, checked))
            }
          />
        </div>
        <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
          {hasInvalidRatio ? (
            <span className="text-destructive text-xs">Sub2API 管理平台倍率必须大于 0</span>
          ) : null}
          <Button
            size="sm"
            onClick={save}
            disabled={props.pending || props.groups.length === 0 || hasInvalidRatio}
          >
            <Save aria-hidden="true" />
            {props.pending ? "正在保存" : "保存绑定与倍率"}
          </Button>
        </div>
      </TableFilterToolbar>
      <DataTablePanel className="flex-1">
        {props.groups.length === 0 ? (
          <div className="text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-2 px-6 text-sm">
            <Link2 className="size-8 opacity-45" aria-hidden="true" />
            <span>尚未读取到 New API 分组</span>
          </div>
        ) : (
          <>
            <Table containerClassName="min-h-0 flex-1 overflow-auto">
              <TableHeader>
                <TableRow>
                  <TableHead>New API 分组</TableHead>
                  <TableHead>New API 当前倍率</TableHead>
                  <TableHead className="min-w-56">Sub2API 分组</TableHead>
                  <TableHead className="w-44">Sub2API 管理平台倍率</TableHead>
                  <TableHead className="w-28 text-center">同步至 New API</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {pagination.visibleItems.map((group) => {
                  const draft = drafts[group.id] ?? {
                    localGroupId: "",
                    sub2APIRatio: "",
                    syncRatio: false,
                  };
                  return (
                    <TableRow key={group.id}>
                      <TableCell className="font-medium">{group.name}</TableCell>
                      <TableCell className="font-mono text-xs">{group.ratio ?? "-"}</TableCell>
                      <TableCell>
                        <Select
                          value={draft.localGroupId || unboundGroupValue}
                          itemToStringLabel={groupValueLabel}
                          onValueChange={(value) =>
                            setDrafts((current) => ({
                              ...current,
                              [group.id]: {
                                localGroupId: value === unboundGroupValue ? "" : (value ?? ""),
                                sub2APIRatio:
                                  value === unboundGroupValue
                                    ? ""
                                    : (props.localGroups.find((item) => item.id === value)?.ratio ??
                                      ""),
                                syncRatio: value === unboundGroupValue ? false : draft.syncRatio,
                              },
                            }))
                          }
                        >
                          <SelectTrigger aria-label={`${group.name} 的 Sub2API 分组`}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value={unboundGroupValue}>不绑定</SelectItem>
                            {props.localGroups.map((localGroup) => (
                              <SelectItem key={localGroup.id} value={localGroup.id}>
                                {localGroup.name} · {localGroup.ratio ?? "无倍率"}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        <Input
                          value={draft.sub2APIRatio}
                          inputMode="decimal"
                          disabled={!draft.localGroupId}
                          aria-label={`${group.name} 的 Sub2API 管理平台倍率`}
                          aria-invalid={
                            draft.localGroupId ? !validSub2APIRatio(draft.sub2APIRatio) : undefined
                          }
                          onChange={(event) =>
                            setDrafts((current) =>
                              updateLocalGroupRatio(
                                current,
                                draft.localGroupId,
                                event.target.value,
                              ),
                            )
                          }
                        />
                      </TableCell>
                      <TableCell className="text-center">
                        <Switch
                          aria-label={`${group.name} 倍率同步`}
                          checked={draft.syncRatio}
                          disabled={!draft.localGroupId}
                          onCheckedChange={(checked) =>
                            setDrafts((current) => ({
                              ...current,
                              [group.id]: { ...draft, syncRatio: checked },
                            }))
                          }
                        />
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            <DataTablePagination
              currentPage={pagination.currentPage}
              totalPages={pagination.totalPages}
              totalItems={props.groups.length}
              pageSize={pagination.pageSize}
              pageSizes={[10, 20, 50, 100]}
              onPageChange={pagination.setCurrentPage}
              onPageSizeChange={pagination.setPageSize}
            />
          </>
        )}
      </DataTablePanel>
    </div>
  );
}
