import { useMemo, type ReactElement } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { api, type AccountGroupsPreview, type Task } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  accountGroupsSchema,
  type AccountGroupsForm,
} from "@/features/accounts/lib/account-groups-schema";
import { notifyOperationError } from "@/lib/operation-feedback";
import type { WorkbenchAccount } from "../types";

type Props = {
  account: WorkbenchAccount;
  onClose: () => void;
  onStarted: (task: Task) => void;
};

export function MoveGroupsDialog(props: Props): ReactElement {
  const preview = useQuery({
    queryKey: ["account-workbench", "account-groups", props.account.id],
    queryFn: ({ signal }) => api.workbenchAccountGroups(props.account.id, signal),
    retry: false,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent className="grid max-h-[calc(100svh-2rem)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden">
        <DialogHeader>
          <DialogTitle>移动分组</DialogTitle>
          <DialogDescription className="break-all">
            为“{props.account.name || props.account.id}”选择目标分组，确认后替换当前全部分组。
          </DialogDescription>
        </DialogHeader>
        {!preview.data && preview.isPending && <ContentLoading label="正在读取账号分组…" />}
        {!preview.data && preview.isError && (
          <ContentRetry pending={preview.isFetching} onRetry={() => void preview.refetch()} />
        )}
        {preview.data && (
          <GroupSelection
            {...props}
            key={`${preview.data.target_version}:${preview.data.account_version}`}
            preview={preview.data}
            refreshing={preview.isFetching}
            unavailable={preview.isError}
            onRefresh={() => void preview.refetch()}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function GroupSelection(
  props: Props & {
    preview: AccountGroupsPreview & { account_version: string };
    refreshing: boolean;
    unavailable: boolean;
    onRefresh: () => void;
  },
): ReactElement {
  const form = useForm<AccountGroupsForm>({
    resolver: zodResolver(accountGroupsSchema),
    defaultValues: { groupIds: props.preview.current_group_ids },
  });
  const options = useMemo(
    () =>
      props.preview.groups.map((group) => ({
        value: group.id,
        label: `${group.name || "分组"}（#${group.id}）`,
      })),
    [props.preview.groups],
  );
  const names = (ids: string[]): string =>
    ids
      .map(
        (id) =>
          props.preview.groups.find((group) => group.id === id)?.name ||
          props.account.groups.find((group) => group.id === id)?.name ||
          `分组 #${id}`,
      )
      .join("、") || "未分组";
  const selected = form.watch("groupIds");
  const unavailable = selected.some((id) => !options.some((option) => option.value === id));
  const unchanged =
    selected.length === props.preview.current_group_ids.length &&
    selected.every((id) => props.preview.current_group_ids.includes(id));
  const mutation = useMutation({
    mutationFn: (values: AccountGroupsForm) =>
      api.moveWorkbenchAccountGroups(props.account.id, {
        group_ids: values.groupIds,
        expected_group_ids: props.preview.current_group_ids,
        target_version: props.preview.target_version,
        account_version: props.preview.account_version,
      }),
    onSuccess: (task) => {
      props.onStarted(task);
      props.onClose();
    },
    onError: (error) => notifyOperationError(error, "移动分组失败"),
  });
  const disabled =
    mutation.isPending ||
    props.refreshing ||
    props.unavailable ||
    unavailable ||
    unchanged ||
    selected.length === 0 ||
    selected.length > 50;
  const error = unavailable
    ? "部分目标分组已不可用，请移除后重新选择"
    : form.formState.errors.groupIds?.message;
  return (
    <form
      className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] gap-4"
      onSubmit={form.handleSubmit((values) => {
        if (!disabled) mutation.mutate(values);
      })}
    >
      <DialogBody className="space-y-4">
        {props.refreshing && <ContentLoading compact label="正在更新账号分组…" />}
        <p className="break-words text-sm">当前分组：{names(props.preview.current_group_ids)}</p>
        <div className="space-y-2">
          <label htmlFor="workbench-target-groups" className="text-sm font-medium">
            目标分组
          </label>
          <Controller
            name="groupIds"
            control={form.control}
            render={({ field }) => (
              <MultiSelect
                id="workbench-target-groups"
                ariaLabel="目标分组"
                options={options}
                selected={field.value}
                onChange={field.onChange}
                clearText="清空选择"
                disabled={mutation.isPending || options.length === 0}
                ariaInvalid={Boolean(error)}
                ariaDescribedBy={error ? "workbench-target-groups-error" : undefined}
              />
            )}
          />
          {error && (
            <p role="alert" id="workbench-target-groups-error" className="text-sm text-destructive">
              {error}
            </p>
          )}
          {options.length === 0 && (
            <p className="text-sm text-muted-foreground">
              没有可用的目标分组，请先在管理平台添加或启用兼容分组。
            </p>
          )}
        </div>
        <p className="break-words text-sm">移动后分组：{names(selected)}</p>
        <p className="text-sm text-muted-foreground">可选择多个分组；未选中的原分组将被移除。</p>
      </DialogBody>
      <DialogFooter className="flex-wrap">
        <Button
          type="button"
          variant="outline"
          onClick={props.onRefresh}
          disabled={props.refreshing || mutation.isPending}
        >
          重新读取
        </Button>
        <Button type="button" variant="outline" onClick={props.onClose}>
          取消
        </Button>
        <Button type="submit" disabled={disabled}>
          {mutation.isPending ? "正在提交…" : "确认移动"}
        </Button>
      </DialogFooter>
    </form>
  );
}
