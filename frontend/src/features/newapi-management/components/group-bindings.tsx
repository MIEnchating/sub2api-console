import { useCallback, useEffect, useMemo, useState } from "react";
import { Link2, Save } from "lucide-react";

import type {
  NewAPIGroupBinding,
  NewAPIGroupBindingUpdate,
  NewAPILocalGroup,
  NewAPIRemoteGroup,
} from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { Button } from "@/components/ui/button";
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

type DraftBinding = { localGroupId: string; syncRatio: boolean };

const unboundGroupValue = "__unbound__";

function createDraftBindings(
  groups: NewAPIRemoteGroup[],
  bindings: NewAPIGroupBinding[],
): Record<string, DraftBinding> {
  const bindingByGroupID = new Map(bindings.map((binding) => [binding.newapi_group_id, binding]));
  const next: Record<string, DraftBinding> = {};
  for (const group of groups) {
    const binding = bindingByGroupID.get(group.id);
    next[group.id] = {
      localGroupId: binding?.sub2api_group_id ?? "",
      syncRatio: binding?.sync_ratio ?? false,
    };
  }
  return next;
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
    createDraftBindings(props.groups, props.bindings),
  );
  const pagination = useClientPagination(props.groups, 10);
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

  useEffect(() => {
    setDrafts(createDraftBindings(props.groups, props.bindings));
  }, [props.bindings, props.groups]);

  function save() {
    const bindings = props.groups.flatMap<NewAPIGroupBindingUpdate>((group) => {
      const draft = drafts[group.id];
      if (!draft?.localGroupId) return [];
      return [
        {
          newapi_group_id: group.id,
          newapi_group_name: group.name,
          sub2api_group_id: draft.localGroupId,
          sync_ratio: draft.syncRatio,
        },
      ];
    });
    props.onSave(bindings);
  }

  return (
    <DataTablePanel>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">分组绑定</h2>
          <p className="text-muted-foreground mt-0.5 text-xs">
            已绑定 {Object.values(drafts).filter((item) => item.localGroupId).length} /{" "}
            {props.groups.length}
          </p>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-4">
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
          <Button size="sm" onClick={save} disabled={props.pending || props.groups.length === 0}>
            <Save aria-hidden="true" />
            {props.pending ? "正在保存" : "保存绑定"}
          </Button>
        </div>
      </div>
      {props.groups.length === 0 ? (
        <div className="text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-2 px-6 text-sm">
          <Link2 className="size-8 opacity-45" aria-hidden="true" />
          <span>尚未读取到 New API 分组</span>
        </div>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>New API 分组</TableHead>
                <TableHead>当前倍率</TableHead>
                <TableHead className="min-w-56">Sub2API 分组</TableHead>
                <TableHead className="w-28 text-center">倍率同步</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pagination.visibleItems.map((group) => {
                const draft = drafts[group.id] ?? { localGroupId: "", syncRatio: false };
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
  );
}
