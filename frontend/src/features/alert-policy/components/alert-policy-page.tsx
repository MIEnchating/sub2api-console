import { useEffect, useMemo, type ReactElement } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Save } from "lucide-react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { notifyOperationError } from "@/lib/operation-feedback";

import { api, type AlertPolicy } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { RefreshButton } from "@/components/refresh-button";
import { QueryErrorToast } from "@/components/query-error-toast";
import { Button } from "@/components/ui/button";
import { AlertPolicyLayout } from "./alert-policy-layout";
import { AlertPolicySkeleton } from "./alert-policy-skeleton";
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

  const isDirty = form.formState.isDirty;
  useEffect(() => {
    if (policy.data && !isDirty) form.reset(policyToForm(policy.data));
  }, [form, isDirty, policy.data]);

  const enabled = form.watch("enabled");
  const policyReady = policy.data !== undefined && !policy.error && !policy.isFetching;

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
            <RefreshButton
              pending={policy.isFetching}
              disabled={update.isPending}
              ariaLabel="刷新告警策略"
              onClick={() => void policy.refetch()}
            />
            <Button
              data-testid="alert-policy-reset"
              variant="outline"
              onClick={() => form.reset(defaultAlertPolicyForm, { keepDefaultValues: true })}
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
      {!policy.data && policy.isLoading && <AlertPolicySkeleton />}
      {!policy.data && !policy.isLoading && (
        <ContentRetry pending={policy.isFetching} onRetry={() => void policy.refetch()} />
      )}
      {policy.data && (
        <form onSubmit={submit}>
          <AlertPolicyLayout detection={<DetectionSettings form={form} enabled={enabled} />}>
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
          </AlertPolicyLayout>
        </form>
      )}
    </PageLayout>
  );
}
