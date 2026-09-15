import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowRight, ShieldCheck } from "lucide-react";
import { useCallback, useId, useState, type ReactElement } from "react";
import { Controller, useForm } from "react-hook-form";

import { Badge } from "@/components/ui/badge";
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
import { OnboardingProbeModelField } from "./onboarding-probe-model-field";
import {
  onboardingProbeModelsSchema,
  parseOnboardingProbeModels,
  type OnboardingAccountProbeModels,
  type OnboardingProbeModelsForm,
} from "../lib/onboarding-probe-models";

export type OnboardingBindingPreview = {
  id: string;
  host: string;
  upstreamGroupId: string;
  upstreamGroup: string;
  platform: string;
  multiplier: string;
  localGroup: string;
  concurrency: number;
  priority: number;
  status: "待添加" | "待更新";
};

type ContentProps = {
  items: OnboardingBindingPreview[];
  pending: boolean;
  onCancel: () => void;
  onConfirm: (models: OnboardingAccountProbeModels) => void;
};

function OnboardingConfirmContent(props: ContentProps): ReactElement {
  const fieldId = useId();
  const form = useForm<OnboardingProbeModelsForm>({
    resolver: zodResolver(onboardingProbeModelsSchema),
    defaultValues: { accounts: props.items.map((item) => ({ id: item.id, models: "" })) },
  });
  const hasNewAccounts = props.items.some((item) => item.status === "待添加");
  const [loadingAccounts, setLoadingAccounts] = useState<Set<string>>(() => new Set());
  const onModelPendingChange = useCallback((accountId: string, pending: boolean) => {
    setLoadingAccounts((current) => {
      if (current.has(accountId) === pending) return current;
      const next = new Set(current);
      if (pending) next.add(accountId);
      else next.delete(accountId);
      return next;
    });
  }, []);
  return (
    <form
      className="contents"
      onSubmit={form.handleSubmit((values) => {
        if (props.pending || loadingAccounts.size > 0) return;
        const models: OnboardingAccountProbeModels = {};
        values.accounts.forEach((account, index) => {
          if (props.items[index]?.status === "待添加")
            models[account.id] = parseOnboardingProbeModels(account.models);
        });
        props.onConfirm(models);
      })}
    >
      <DialogHeader>
        <DialogTitle>确认账号绑定变更</DialogTitle>
        <DialogDescription>
          {hasNewAccounts
            ? "每个新增账号可获取并选择探活模型；留空使用默认探活模型。"
            : "核对现有账号的本地分组变更。"}
        </DialogDescription>
      </DialogHeader>
      <DialogBody className="pr-0 [scrollbar-gutter:stable]">
        <div className="divide-y rounded-lg border">
          {props.items.map((item, index) => {
            const identity = `${item.upstreamGroup} → ${item.localGroup}`;
            return (
              <section key={item.id} aria-label={identity} className="grid min-w-0 gap-3 p-3">
                <div className="flex min-w-0 items-start justify-between gap-3">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 font-medium">
                    <span className="min-w-0 break-all">{item.upstreamGroup}</span>
                    <ArrowRight
                      aria-hidden="true"
                      className="text-muted-foreground size-3.5 shrink-0"
                    />
                    <span className="min-w-0 break-all">{item.localGroup}</span>
                  </div>
                  <Badge variant="outline" className="shrink-0">
                    {item.status}
                  </Badge>
                </div>
                <dl className="text-muted-foreground flex flex-wrap gap-x-5 gap-y-1 text-xs tabular-nums">
                  <div className="flex gap-1.5">
                    <dt>账号协议</dt>
                    <dd className="text-foreground">{item.platform}</dd>
                  </div>
                  <div className="flex gap-1.5">
                    <dt>账号成本</dt>
                    <dd className="text-foreground">{item.multiplier}</dd>
                  </div>
                  <div className="flex gap-1.5">
                    <dt>并发</dt>
                    <dd className="text-foreground">{item.concurrency}</dd>
                  </div>
                  <div className="flex gap-1.5">
                    <dt>优先级</dt>
                    <dd className="text-foreground">{item.priority}</dd>
                  </div>
                </dl>
                {item.status === "待添加" ? (
                  <Controller
                    control={form.control}
                    name={`accounts.${index}.models`}
                    render={(controller) => (
                      <OnboardingProbeModelField
                        accountId={item.id}
                        host={item.host}
                        groupId={item.upstreamGroupId}
                        fieldId={`${fieldId}-${index}`}
                        identity={identity}
                        value={controller.field.value}
                        error={controller.fieldState.error?.message}
                        disabled={props.pending}
                        onChange={controller.field.onChange}
                        onBlur={controller.field.onBlur}
                        onPendingChange={onModelPendingChange}
                      />
                    )}
                  />
                ) : null}
              </section>
            );
          })}
        </div>
        {hasNewAccounts ? (
          <p className="text-muted-foreground mt-2 text-xs">所选模型仅用于对应账号的后续探活。</p>
        ) : null}
      </DialogBody>
      <DialogFooter className="flex-row items-center justify-end">
        <Button type="button" variant="outline" disabled={props.pending} onClick={props.onCancel}>
          取消
        </Button>
        <Button
          type="submit"
          disabled={props.pending || loadingAccounts.size > 0 || props.items.length === 0}
        >
          <ShieldCheck aria-hidden="true" />
          {props.pending ? "正在提交" : `确认提交 ${props.items.length} 项变更`}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function OnboardingConfirmDialog(
  props: Omit<ContentProps, "onCancel"> & {
    open: boolean;
    onOpenChange: (open: boolean) => void;
  },
): ReactElement {
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!props.pending) props.onOpenChange(open);
      }}
    >
      <DialogContent
        width="progress"
        height="adaptive"
        className="grid grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <OnboardingConfirmContent
          key={JSON.stringify(props.items.map((item) => item.id))}
          items={props.items}
          pending={props.pending}
          onCancel={() => props.onOpenChange(false)}
          onConfirm={props.onConfirm}
        />
      </DialogContent>
    </Dialog>
  );
}
