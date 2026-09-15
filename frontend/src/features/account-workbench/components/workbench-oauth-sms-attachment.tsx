import { useEffect, useRef, useState, type ReactElement } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormProvider, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { MessageSquare } from "lucide-react";
import { toast } from "sonner";
import { api, type WorkbenchOAuthSMSInput } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { notifyOperationError } from "@/lib/operation-feedback";
import {
  oauthSMSAttachmentSchema,
  oauthSMSDefaults,
  oauthSMSInput,
  type OAuthSMSValues,
} from "../lib/oauth-sms-schema";
import { WorkbenchOAuthSMSFields } from "./workbench-oauth-sms";

export function WorkbenchOAuthSMSAttachment(props: {
  id: string;
  disabled: boolean;
}): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" disabled={props.disabled} onClick={() => setOpen(true)}>
        <MessageSquare aria-hidden="true" />
        配置短信接码
      </Button>
      {open ? <SMSAttachmentDialog id={props.id} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

function SMSAttachmentDialog(props: { id: string; onClose: () => void }): ReactElement {
  const client = useQueryClient();
  const pending = useRef<WorkbenchOAuthSMSInput | null>(null);
  const mounted = useRef(true);
  const form = useForm<OAuthSMSValues>({
    resolver: zodResolver(oauthSMSAttachmentSchema),
    defaultValues: oauthSMSDefaults,
  });
  const query = useQuery({
    queryKey: ["account-workbench", "oauth-sms-attachment", props.id],
    queryFn: (context) => api.workbenchOAuthSMSAttachment(props.id, context.signal),
    retry: false,
    gcTime: 0,
  });
  const id = props.id;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      pending.current = null;
      const key = ["account-workbench", "oauth-sms-attachment", id];
      void client.cancelQueries({ queryKey: key });
      client.removeQueries({ queryKey: key });
    };
  }, [client, id]);
  const save = useMutation({
    gcTime: 0,
    mutationFn: async () => {
      const sms = pending.current;
      pending.current = null;
      if (!sms || !query.data?.can_attach) return;
      await api.attachWorkbenchOAuthSMS(props.id, {
        scope: query.data.scope,
        revision: query.data.revision,
        sms,
      });
    },
    onSuccess: () => {
      if (!mounted.current) return;
      toast.success("短信接码配置已提交");
      props.onClose();
    },
    onError: (error) => {
      notifyOperationError(error, "短信配置未确认，请刷新当前步骤并核对订单");
      if (mounted.current) void query.refetch();
    },
  });
  const enabled = query.data?.can_attach && !query.isError && !query.isFetching && !save.isPending;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>配置当前授权的短信接码</DialogTitle>
        </DialogHeader>
        <DialogBody>
          {query.isPending ? <ContentLoading label="正在核对手机号步骤与原订单" /> : null}
          {query.isError ? (
            <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
          ) : null}
          {query.data && !query.data.can_attach ? (
            <p role="status" className="text-sm">
              {query.data.configured
                ? "当前授权已有接码配置或订单，请先核对原订单"
                : "当前授权不在可配置的手机号步骤"}
            </p>
          ) : null}
          {query.data?.can_attach ? (
            <FormProvider {...form}>
              <form
                id="oauth-live-sms"
                onSubmit={form.handleSubmit((value) => {
                  if (!enabled) return;
                  pending.current = oauthSMSInput(value) ?? null;
                  form.reset();
                  save.mutate();
                })}
              >
                <fieldset disabled={!enabled} className="min-w-0">
                  <WorkbenchOAuthSMSFields />
                </fieldset>
              </form>
            </FormProvider>
          ) : null}
          {save.isPending ? <ContentLoading label="正在提交接码配置" compact /> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            返回授权页面
          </Button>
          {query.data?.can_attach ? (
            <Button
              type="submit"
              form="oauth-live-sms"
              disabled={!enabled || form.watch("sms_provider") === "none"}
            >
              确认用于当前手机号步骤
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
