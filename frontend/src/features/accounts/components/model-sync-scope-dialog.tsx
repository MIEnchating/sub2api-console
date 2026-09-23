import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api } from "@/api";
import { MultiSelect } from "@/components/multi-select";
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
import { orderByDictionary } from "@/lib/dictionary-order";
import { modelSyncScopeSchema, type ModelSyncScopeValues } from "../lib/model-sync-scope-schema";

type ScopeGroup = { value: string; name: string; accountIds: Set<string> };

export function ModelSyncScopeDialog(props: {
  accountIds: string[];
  accountGroups: ReadonlyMap<string, string[]>;
  onOpenChange: (open: boolean) => void;
  onStart: (accountIds: string[]) => void;
}) {
  const dictionary = useQuery({
    queryKey: ["dictionaries", "group"],
    queryFn: () => api.dictionaries("group"),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
  });
  const groups = useMemo(() => {
    const result = new Map<string, ScopeGroup>();
    for (const accountId of props.accountIds) {
      const memberships = (props.accountGroups.get(accountId) ?? [])
        .map((group) => group.trim())
        .filter(Boolean);
      const entries = memberships.length > 0 ? memberships : [""];
      for (const name of entries) {
        const value = name ? `group:${name.toLocaleLowerCase()}` : "ungrouped";
        const group = result.get(value) ?? {
          value,
          name: name || "未分组",
          accountIds: new Set<string>(),
        };
        group.accountIds.add(accountId);
        result.set(value, group);
      }
    }
    return orderByDictionary(
      [...result.values()],
      dictionary.data?.items,
      (group) => group.name,
      "name",
    );
  }, [props.accountIds, props.accountGroups, dictionary.data?.items]);
  const form = useForm<ModelSyncScopeValues>({
    resolver: zodResolver(modelSyncScopeSchema),
    defaultValues: { groups: [] },
  });
  const selected = useWatch({ control: form.control, name: "groups" });
  const selectedGroups = groups.filter((group) => selected.includes(group.value));
  const selectedIds = new Set(selectedGroups.flatMap((group) => [...group.accountIds]));
  const accountIds = [...new Set(props.accountIds)].filter((id) => selectedIds.has(id));
  return (
    <Dialog open onOpenChange={props.onOpenChange}>
      <DialogContent width="medium">
        <DialogHeader>
          <DialogTitle>选择同步分组</DialogTitle>
          <DialogDescription>
            可多选分组；确认后再获取所选账号的模型，重复账号只同步一次。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <form
            id="model-sync-scope"
            className="grid gap-3"
            onSubmit={form.handleSubmit(() => {
              if (accountIds.length > 0) props.onStart(accountIds);
            })}
          >
            <Controller
              control={form.control}
              name="groups"
              render={({ field }) => (
                <MultiSelect
                  options={groups.map((group) => ({
                    value: group.value,
                    label: `${group.name} · ${group.accountIds.size}`,
                  }))}
                  selected={field.value}
                  onChange={field.onChange}
                  ariaLabel="同步分组"
                  title="选择要同步的分组"
                  searchPlaceholder="搜索分组"
                  clearText="清空分组"
                  maxVisibleChips={3}
                  disabled={groups.length === 0}
                />
              )}
            />
            {groups.length === 0 ? (
              <p className="text-muted-foreground text-sm">当前没有可同步的账号</p>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-muted-foreground text-sm" role="status">
                  已选 {selectedGroups.length} 个分组，共 {accountIds.length} 个账号
                </p>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() =>
                    form.setValue(
                      "groups",
                      groups.map((group) => group.value),
                    )
                  }
                >
                  选择全部分组
                </Button>
              </div>
            )}
          </form>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button type="submit" form="model-sync-scope" disabled={accountIds.length === 0}>
            开始同步
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
