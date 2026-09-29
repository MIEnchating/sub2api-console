import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, type ReactElement } from "react";

import { api, type AccountStatus } from "@/api";
import { Switch } from "@/components/ui/switch";
import { notifyOperationError } from "@/lib/operation-feedback";

type Props = {
  account: Pick<AccountStatus, "id" | "name" | "groups" | "groups_locked">;
  disabled?: boolean;
};

export function AccountGroupLockSwitch(props: Props): ReactElement {
  const id = useId();
  const client = useQueryClient();
  const mutation = useMutation({
    mutationFn: (enabled: boolean) => api.setAccountGroupsLocked(props.account.id, enabled),
    onSuccess: async (result) => {
      await client.cancelQueries({ queryKey: ["accounts"] });
      await client.cancelQueries({ queryKey: ["account-detail", props.account.id] });
      client.setQueryData<AccountStatus[]>(["accounts"], (accounts) =>
        accounts?.map((account) =>
          account.id === props.account.id ? { ...account, ...result } : account,
        ),
      );
      client.setQueryData<AccountStatus>(["account-detail", props.account.id], (account) =>
        account ? { ...account, ...result } : account,
      );
      await Promise.all(
        [
          ["accounts"],
          ["account-detail", props.account.id],
          ["account-groups", props.account.id],
          ["pricing"],
          ["policy"],
          ["logs"],
          ["overview-events"],
        ].map((queryKey) => client.invalidateQueries({ queryKey })),
      );
    },
    onError: (error) => notifyOperationError(error, "修改分组锁定失败"),
  });
  const locked = props.account.groups_locked === true;
  return (
    <span className="inline-flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
      <span aria-hidden="true">锁定分组</span>
      <Switch
        aria-label={`锁定分组：${props.account.name}`}
        aria-describedby={`${id}-description`}
        checked={locked}
        disabled={
          props.disabled || mutation.isPending || (!locked && props.account.groups.length === 0)
        }
        onCheckedChange={(enabled) => mutation.mutate(enabled)}
      />
      <span id={`${id}-description`} className="sr-only">
        开启后锁定当前全部分组，禁止手动和自动切换；关闭后恢复原有规则。未分组账号需先选择分组。
      </span>
    </span>
  );
}
