import { zodResolver } from "@hookform/resolvers/zod";
import { ShieldCheck } from "lucide-react";
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
import { OnboardingAccountSummary } from "./onboarding-account-summary";
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
  const newAccountCount = props.items.filter((item) => item.status === "待添加").length;
  const updateAccountCount = props.items.length - newAccountCount;
  const hasNewAccounts = newAccountCount > 0;
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
        <div
          role="group"
          aria-label="变更概览"
          className="flex flex-wrap items-center gap-2 pt-1 text-xs"
        >
          <span className="text-muted-foreground mr-1">共 {props.items.length} 项变更</span>
          {newAccountCount > 0 ? <Badge variant="secondary">新增 {newAccountCount}</Badge> : null}
          {updateAccountCount > 0 ? (
            <Badge variant="outline">更新 {updateAccountCount}</Badge>
          ) : null}
        </div>
      </DialogHeader>
      <DialogBody className="pr-0 [scrollbar-gutter:stable]">
        <div className="grid gap-3">
          {props.items.map((item, index) => {
            const identity = `${item.upstreamGroup} → ${item.localGroup}`;
            return (
              <section
                key={item.id}
                aria-label={identity}
                className="bg-background min-w-0 overflow-hidden rounded-lg border"
              >
                <OnboardingAccountSummary item={item} index={index} />
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
