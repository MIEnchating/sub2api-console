import { useMutation, useQuery } from "@tanstack/react-query";
import { zodResolver } from "@hookform/resolvers/zod";
import { Controller, useForm } from "react-hook-form";
import { useMemo, type ReactElement } from "react";

import { api, type AccountGroupsPreview, type AccountStatus, type Task } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
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
import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { notifyOperationError } from "@/lib/operation-feedback";
import { accountGroupsSchema, type AccountGroupsForm } from "../lib/account-groups-schema";

type Props = {
  account: Pick<AccountStatus, "id" | "name" | "manual_priority" | "groups_locked">;
  onOpenChange: (open: boolean) => void;
  onStarted: (task: Task) => void;
};

export function AccountGroupsDialog(props: Props): ReactElement {
  const preview = useQuery({
    queryKey: ["account-groups", props.account.id],
    queryFn: () => api.accountGroups(props.account.id),
    retry: false,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog open onOpenChange={props.onOpenChange}>
      <DialogContent className="grid max-h-[calc(100svh-2rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden">
        <DialogHeader>
          <DialogTitle>切换分组</DialogTitle>
          <DialogDescription className="break-all">
            为“{props.account.name}”重新选择完整分组范围，确认后替换当前分组。
          </DialogDescription>
        </DialogHeader>
        {!preview.data && preview.isPending && <ContentLoading label="正在读取账号分组…" />}
        {!preview.data && preview.isError && (
          <ContentRetry onRetry={() => void preview.refetch()} pending={preview.isFetching} />
        )}
        {preview.data && (
          <AccountGroupsSelection
            key={`${props.account.id}:${preview.data.target_version}:${preview.data.current_group_ids.join(",")}`}
            {...props}
            preview={preview.data}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function AccountGroupsSelection(props: Props & { preview: AccountGroupsPreview }): ReactElement {
  const groups = useDictionaryOrder("group", props.preview.groups, (group) => group.id);
  const options = useMemo(
    () => groups.map((group) => ({ value: group.id, label: `${group.name}（#${group.id}）` })),
    [groups],
  );
  const form = useForm<AccountGroupsForm>({
    resolver: zodResolver(accountGroupsSchema),
    defaultValues: { groupIds: props.preview.current_group_ids },
  });
  const selected = form.watch("groupIds");
  const manual = props.account.manual_priority != null;
  const locked = props.account.groups_locked === true;
  const unchanged =
    selected.length === props.preview.current_group_ids.length &&
    selected.every((id) => props.preview.current_group_ids.includes(id));
  const mutation = useMutation({
    mutationFn: (values: AccountGroupsForm) =>
      api.updateAccountGroups(props.account.id, {
        group_ids: values.groupIds,
        expected_group_ids: props.preview.current_group_ids,
        target_version: props.preview.target_version,
      }),
    onSuccess: (task) => {
      props.onStarted(task);
      props.onOpenChange(false);
    },
    onError: (error) => notifyOperationError(error, "切换分组失败"),
  });
  const groupNames = (ids: string[]): string =>
    ids
      .map((id) => props.preview.groups.find((group) => group.id === id)?.name ?? `分组 #${id}`)
      .join("、") || "未分组";
  return (
    <form
      className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-4"
      onSubmit={form.handleSubmit((values) => {
        if (!locked && !manual && !unchanged && !mutation.isPending) mutation.mutate(values);
      })}
    >
      <DialogBody className="space-y-4">
        <p className="break-words text-sm">
          当前分组：{groupNames(props.preview.current_group_ids)}
        </p>
        <div className="space-y-2">
          <label htmlFor="account-target-groups" className="text-sm font-medium">
            目标分组
          </label>
          <Controller
            name="groupIds"
            control={form.control}
            render={({ field, fieldState }) => (
              <MultiSelect
                id="account-target-groups"
                ariaLabel="目标分组"
                options={options}
                selected={field.value}
                onChange={field.onChange}
                disabled={locked || manual || mutation.isPending || options.length === 0}
                ariaInvalid={fieldState.invalid}
                ariaDescribedBy={fieldState.error ? "account-target-groups-error" : undefined}
              />
            )}
          />
          {form.formState.errors.groupIds && (
            <p id="account-target-groups-error" role="alert" className="text-destructive text-sm">
              {form.formState.errors.groupIds.message}
            </p>
          )}
          {options.length === 0 && (
            <p className="text-muted-foreground text-sm">
              没有与账号平台兼容的分组，请先同步管理平台目录。
            </p>
          )}
        </div>
        <p className="break-words text-sm">切换后分组：{groupNames(selected)}</p>
        <p className="text-muted-foreground text-sm">
          未锁定的普通账号仍受自动分组策略约束；锁定分组或手动控制的账号禁止切换分组。
        </p>
        {locked && (
          <p className="text-muted-foreground text-sm">请先关闭锁定分组开关，再切换分组。</p>
        )}
        {manual && <p className="text-muted-foreground text-sm">请先取消手动控制，再切换分组。</p>}
      </DialogBody>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => props.onOpenChange(false)}>
          取消
        </Button>
        <Button
          type="submit"
          disabled={
            locked ||
            manual ||
            mutation.isPending ||
            unchanged ||
            selected.length === 0 ||
            options.length === 0
          }
        >
          {mutation.isPending ? "正在提交…" : "确认切换"}
        </Button>
      </DialogFooter>
    </form>
  );
}
