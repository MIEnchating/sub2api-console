import { useEffect, useMemo, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Save } from "lucide-react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { notifyOperationError } from "@/lib/operation-feedback";

import { api, type AlertPolicy } from "@/api";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { DetectionSettings } from "./detection-settings";
import { NotificationSettings } from "./notification-settings";
import { ThresholdSettings } from "./threshold-settings";
import {
  alertPolicyFormSchema,
  defaultAlertPolicyForm,
  type AlertPolicyFormValues,
} from "../lib/alert-policy-schema";

type AlertPolicyPageProps = {
  onOpenSettings: () => void;
};

function policyToForm(policy: AlertPolicy): AlertPolicyFormValues {
  return {
    ...policy,
    routing_degraded_types:
      policy.routing_degraded_types ?? defaultAlertPolicyForm.routing_degraded_types,
    recovery_notification_types:
      policy.recovery_notification_types ?? defaultAlertPolicyForm.recovery_notification_types,
    balance_thresholds: policy.balance_thresholds.map((value) => ({ value })),
  };
}

function LoadingPolicy(): ReactElement {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {[0, 1, 2, 3].map((item) => (
        <Card key={item} className="min-h-52 p-4">
          <Skeleton className="h-6 w-28" />
          <Skeleton className="mt-5 h-10 w-full" />
          <Skeleton className="mt-3 h-10 w-full" />
          <Skeleton className="mt-3 h-10 w-3/4" />
        </Card>
      ))}
    </div>
  );
}

function PolicyUnavailable(props: { isFetching: boolean; onRetry: () => void }): ReactElement {
  return (
    <Card>
      <CardContent className="grid min-h-52 place-items-center p-6 text-center">
        <div>
          <RefreshButton
            pending={props.isFetching}
            ariaLabel="刷新告警策略"
            onClick={props.onRetry}
            className="mt-4"
          />
        </div>
      </CardContent>
    </Card>
  );
}

export function AlertPolicyPage(props: AlertPolicyPageProps): ReactElement {
  const queryClient = useQueryClient();
  const policy = useQuery({ queryKey: ["alert-policy"], queryFn: api.alertPolicy });
  const notification = useQuery({
    queryKey: ["notification-status"],
    queryFn: api.notificationStatus,
  });
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const groupOptions = useMemo(
    () => (groups.data ?? []).map((group) => ({ value: group.name, label: group.name })),
    [groups.data],
  );
  const form = useForm<AlertPolicyFormValues>({
    resolver: zodResolver(alertPolicyFormSchema),
    defaultValues: policy.data ? policyToForm(policy.data) : defaultAlertPolicyForm,
  });
  const update = useMutation({
    mutationFn: api.updateAlertPolicy,
    onSuccess: (saved) => {
      queryClient.setQueryData(["alert-policy"], saved);
      form.reset(policyToForm(saved));
      toast.success("告警策略已保存");
    },
    onError: (error) => notifyOperationError(error, "告警策略保存失败"),
  });

  useEffect(() => {
    if (policy.data) form.reset(policyToForm(policy.data));
  }, [form, policy.data]);

  const enabled = form.watch("enabled");
  const policyReady = policy.data !== undefined && !policy.error;

  const submit = form.handleSubmit((values) => {
    if (!policyReady) {
      toast.error("告警策略尚未成功读取，请重试后再保存");
      return;
    }
    update.mutate({
      ...values,
      balance_thresholds: values.balance_thresholds.map((item) => item.value.trim()),
    });
  });

  return (
    <PageLayout>
      <PageHeading
        eyebrow="OPERATIONS / ALERT POLICY"
        title="告警策略"
        description="配置告警检测范围、触发阈值和通知发送行为；渠道凭据统一在系统设置中管理。"
        action={
          <PageActions>
            <Button
              data-testid="alert-policy-reset"
              variant="outline"
              onClick={() => form.reset(defaultAlertPolicyForm)}
              disabled={update.isPending || !policyReady}
            >
              <RotateCcw aria-hidden="true" /> 恢复默认
            </Button>
            <Button
              data-testid="alert-policy-save"
              onClick={() => void submit()}
              disabled={update.isPending || !policyReady}
            >
              <Save aria-hidden="true" /> {update.isPending ? "保存中" : "保存策略"}
            </Button>
          </PageActions>
        }
      />

      {policy.error && <QueryErrorToast error={policy.error} fallback="告警策略读取失败" />}
      {policy.isLoading && <LoadingPolicy />}
      {!policy.isLoading && !policyReady && (
        <PolicyUnavailable isFetching={policy.isFetching} onRetry={() => void policy.refetch()} />
      )}
      {!policy.isLoading && policyReady && (
        <form
          onSubmit={submit}
          data-slot="alert-policy-columns"
          className="grid items-start gap-4 lg:grid-cols-2"
        >
          <DetectionSettings form={form} enabled={enabled} />
          <div className="grid min-w-0 gap-4" data-slot="alert-policy-notification-column">
            <ThresholdSettings
              form={form}
              enabled={enabled}
              groupOptions={groupOptions}
              groupsLoading={groups.isLoading}
              groupsError={groups.isError}
            />
            <NotificationSettings
              form={form}
              enabled={enabled}
              notification={notification.data}
              onOpenSettings={props.onOpenSettings}
            />
          </div>
        </form>
      )}
    </PageLayout>
  );
}
