import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { useQuery, useIsMutating } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { api, type KumaTemplate } from "@/api";
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
import type { TemplateValues } from "../lib/template-schema";
import { TemplateEditorForm } from "./template-editor-form";

export function TemplateDialog(props: {
  item?: KumaTemplate;
  pending: boolean;
  error: Error | null;
  onClose: () => void;
  onSubmit: (value: TemplateValues) => void;
}) {
  const presetPending = useIsMutating({ mutationKey: ["kuma-template-preset"] }) > 0;
  const detail = useQuery({
    queryKey: ["uptime-kuma", "template-editor", props.item?.id],
    queryFn: ({ signal }) => api.kumaTemplate(props.item!.id, signal),
    enabled: !!props.item,
    gcTime: 0,
    staleTime: 0,
    retry: false,
    refetchOnMount: "always",
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const loading = !!props.item && detail.isPending;
  const ready = !props.item || !!detail.data;
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent
        width="wide"
        showCloseButton={!props.pending}
        className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>{props.item ? "编辑功能模板" : "新增功能模板"}</DialogTitle>
          <DialogDescription>保存监控参数、请求头和请求体，供监控复用。</DialogDescription>
        </DialogHeader>
        <DialogBody>
          {loading && <ContentLoading label="正在读取模板内容…" />}
          {!loading && !ready && detail.isError && (
            <ContentRetry onRetry={() => void detail.refetch()} pending={detail.isFetching} />
          )}
          {ready && (
            <TemplateEditorForm
              item={detail.data}
              pending={props.pending}
              error={props.error}
              onSubmit={props.onSubmit}
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose} disabled={props.pending}>
            取消
          </Button>
          <Button
            type="submit"
            form="kuma-template"
            disabled={props.pending || presetPending || !ready}
          >
            <Save aria-hidden="true" />
            {props.pending ? "正在保存…" : "保存模板"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
