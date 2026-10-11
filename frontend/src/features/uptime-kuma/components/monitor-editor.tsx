import type { ComponentProps } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { QueryErrorToast } from "@/components/query-error-toast";
import { kumaTemplatesKey } from "../constants";
import { MonitorDialog } from "./monitor-dialog";

type MonitorEditorProps = Omit<
  ComponentProps<typeof MonitorDialog>,
  "templates" | "templatesPending" | "templatesError" | "onTemplatesRetry"
> & { management: boolean };

// Keep the template observer inside the open editor so closing it releases the
// request without cancelling another page that still reads the same query.
export function MonitorEditor(props: MonitorEditorProps) {
  const enabled =
    props.management && props.initialType !== "group" && props.monitor?.type !== "group";
  const query = useQuery({
    queryKey: kumaTemplatesKey,
    queryFn: ({ signal }) => api.kumaTemplates(signal),
    enabled,
    retry: false,
  });
  return (
    <>
      {query.error && (
        <QueryErrorToast error={query.error} fallback="模板读取失败，请关闭编辑窗口后刷新重试" />
      )}
      <MonitorDialog
        {...props}
        templates={query.data}
        templatesPending={enabled && query.isPending}
        templatesError={query.isError}
        onTemplatesRetry={() => void query.refetch()}
      />
    </>
  );
}
