import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { api } from "@/api";
import { Switch } from "@/components/ui/switch";
import { notifyOperationError } from "@/lib/operation-feedback";

export function AccountCostWallSwitch(props: {
  accountId: string;
  enabled?: boolean;
  disabled?: boolean;
}) {
  const client = useQueryClient();
  const [enabled, setEnabled] = useState(props.enabled === true);
  useEffect(() => setEnabled(props.enabled === true), [props.accountId, props.enabled]);
  const save = useMutation({
    mutationFn: (value: boolean) => api.setAccountIgnoreCostWall(props.accountId, value),
    onSuccess: async (result) => {
      setEnabled(result.ignore_cost_wall);
      toast.success(result.ignore_cost_wall ? "已开启无视成本墙" : "已恢复成本墙控制");
      await Promise.all([
        client.invalidateQueries({ queryKey: ["accounts"] }),
        client.invalidateQueries({ queryKey: ["account-detail", props.accountId] }),
        client.invalidateQueries({ queryKey: ["policy"] }),
        client.invalidateQueries({ queryKey: ["logs"] }),
      ]);
    },
    onError: (error) => notifyOperationError(error, "成本墙开关保存失败"),
  });
  const id = `account-ignore-cost-wall-${props.accountId}`;
  return (
    <div
      data-slot="settings-switch-row"
      className="flex min-h-12 items-center justify-between gap-3 px-3 py-2"
    >
      <div className="grid gap-1">
        <label htmlFor={id} className="text-sm font-medium">
          无视成本墙
        </label>
        <p id={`${id}-description`} className="text-muted-foreground text-xs leading-4">
          默认关闭。开启后，成本墙不再限制此账号的调度、权重和自动探活；仍遵守其他调度规则。
          切换后立即保存，后续调度按新设置评估。
        </p>
      </div>
      <Switch
        id={id}
        checked={enabled}
        disabled={props.disabled || save.isPending}
        aria-describedby={`${id}-description`}
        aria-busy={save.isPending}
        className="shrink-0"
        onCheckedChange={(value) => save.mutate(value)}
      />
    </div>
  );
}
