import { useEffect, useState } from "react";
import { Link2, Save } from "lucide-react";

import type {
  NewAPIGroupBinding,
  NewAPIGroupBindingUpdate,
  NewAPILocalGroup,
  NewAPIRemoteGroup,
} from "@/api";
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

type Props = {
  groups: NewAPIRemoteGroup[];
  localGroups: NewAPILocalGroup[];
  bindings: NewAPIGroupBinding[];
  pending: boolean;
  onSave: (bindings: NewAPIGroupBindingUpdate[]) => void;
};

type DraftBinding = { localGroupId: string; syncRatio: boolean };

export function NewAPIGroupBindings(props: Props) {
  const [drafts, setDrafts] = useState<Record<string, DraftBinding>>({});

  useEffect(() => {
    const next: Record<string, DraftBinding> = {};
    for (const binding of props.bindings) {
      next[binding.newapi_group_id] = {
        localGroupId: binding.sub2api_group_id,
        syncRatio: binding.sync_ratio,
      };
    }
    setDrafts(next);
  }, [props.bindings]);

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
    <section className="overflow-hidden rounded-md border bg-background">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">分组绑定</h2>
          <p className="text-muted-foreground mt-0.5 text-xs">
            已绑定 {Object.values(drafts).filter((item) => item.localGroupId).length} /{" "}
            {props.groups.length}
          </p>
        </div>
        <Button size="sm" onClick={save} disabled={props.pending || props.groups.length === 0}>
          <Save aria-hidden="true" />
          {props.pending ? "正在保存" : "保存绑定"}
        </Button>
      </div>
      {props.groups.length === 0 ? (
        <div className="text-muted-foreground flex min-h-52 flex-col items-center justify-center gap-2 px-6 text-sm">
          <Link2 className="size-8 opacity-45" aria-hidden="true" />
          <span>尚未读取到 New API 分组</span>
        </div>
      ) : (
        <div className="overflow-x-auto">
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
              {props.groups.map((group) => {
                const draft = drafts[group.id] ?? { localGroupId: "", syncRatio: false };
                return (
                  <TableRow key={group.id}>
                    <TableCell className="font-medium">{group.name}</TableCell>
                    <TableCell className="font-mono text-xs">{group.ratio ?? "-"}</TableCell>
                    <TableCell>
                      <Select
                        value={draft.localGroupId || "__unbound__"}
                        onValueChange={(value) =>
                          setDrafts((current) => ({
                            ...current,
                            [group.id]: {
                              localGroupId: value === "__unbound__" ? "" : (value ?? ""),
                              syncRatio: value === "__unbound__" ? false : draft.syncRatio,
                            },
                          }))
                        }
                      >
                        <SelectTrigger aria-label={`${group.name} 的 Sub2API 分组`}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="__unbound__">不绑定</SelectItem>
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
        </div>
      )}
    </section>
  );
}
