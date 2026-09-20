import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowRight, ShieldCheck } from "lucide-react";
import { useCallback, useState, type ReactElement } from "react";
import { useForm } from "react-hook-form";

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
import { OnboardingAccountModelFields } from "./onboarding-account-model-fields";
import {
  parseOnboardingProbeModels,
  type OnboardingAccountProbeModels,
} from "../lib/onboarding-probe-models";

import {
  onboardingConfirmationSchema,
  type OnboardingConfirmationForm,
  type OnboardingAccountModelMappings,
} from "../lib/onboarding-model-mapping";

export type OnboardingBindingPreview = {
  id: string;
  host: string;
  upstreamGroupId: string;
  upstreamGroup: string;
  platform: string;
  multiplier: string;
  localGroup: string;
  localGroupIds?: string[];
  concurrency: number;
  waitingForCapacity?: boolean;
  allocationOverride?: boolean;
  priority: number;
  status: "待添加" | "待更新";
};

type ContentProps = {
  items: OnboardingBindingPreview[];
  pending: boolean;
  onCancel: () => void;
  onConfirm: (
    models: OnboardingAccountProbeModels,
    mappings: OnboardingAccountModelMappings,
  ) => void;
};

function OnboardingConfirmContent(props: ContentProps): ReactElement {
  const form = useForm<OnboardingConfirmationForm>({
    resolver: zodResolver(onboardingConfirmationSchema),
    defaultValues: {
      accounts: props.items.map((item) => ({ id: item.id, models: "", mapping: [] })),
    },
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
        const mappings: OnboardingAccountModelMappings = {};
        values.accounts.forEach((account, index) => {
          if (props.items[index]?.status === "待添加") {
            models[account.id] = parseOnboardingProbeModels(account.models);
            mappings[account.id] = Object.fromEntries(
              account.mapping.map((row) => [row.source, row.target]),
            );
          }
        });
        props.onConfirm(models, mappings);
      })}
    >
      <DialogHeader>
        <DialogTitle>确认账号绑定变更</DialogTitle>
        <DialogDescription>
          {hasNewAccounts
            ? "核对账号信息，按需配置模型映射和探活模型。"
            : "核对现有账号的本地分组变更。"}
        </DialogDescription>
      </DialogHeader>
      <DialogBody className="pr-0 [scrollbar-gutter:stable]">
        <div className="grid gap-4">
          {props.items.map((item, index) => {
            const identity = `${item.upstreamGroup} → ${item.localGroup}`;
            return (
              <section
                key={item.id}
                aria-label={identity}
                className="grid min-w-0 gap-4 rounded-lg border p-3 sm:p-4"
              >
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
                <dl className="text-muted-foreground bg-muted/40 grid grid-cols-2 gap-x-4 gap-y-2 rounded-md p-3 text-xs tabular-nums sm:grid-cols-4">
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
                    <dd className="text-foreground">
                      {item.status === "待更新" ? "保持原值" : item.concurrency}
                    </dd>
                  </div>
                  <div className="flex gap-1.5">
                    <dt>优先级</dt>
                    <dd className="text-foreground">
                      {item.status === "待更新" ? "保持原值" : item.priority}
                    </dd>
                  </div>
                </dl>
                {item.status === "待添加" && item.allocationOverride !== undefined ? (
                  <p className="text-muted-foreground text-xs">
                    共享并发分配：{item.allocationOverride ? "单独开启" : "单独关闭"}
                    （总开关与容量保护继续生效）
                  </p>
                ) : null}
                {item.waitingForCapacity ? (
                  <p className="text-muted-foreground text-xs">
                    <span className="font-medium">等待并发额度</span>：账号将以并发 1
                    创建并保持停用，启用前须由调度器重新核对容量。
                  </p>
                ) : null}
                {item.status === "待添加" ? (
                  <OnboardingAccountModelFields
                    form={form}
                    index={index}
                    item={item}
                    disabled={props.pending}
                    onPendingChange={onModelPendingChange}
                  />
                ) : null}
              </section>
            );
          })}
        </div>
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
        width="content"
        height="adaptive"
        className="grid w-[min(48rem,calc(100vw-2rem))] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
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
