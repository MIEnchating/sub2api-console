import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import { QueryErrorToast } from "@/components/query-error-toast";
import { RawPricingSourceDialog } from "./raw-pricing-source-dialog";

export function RemotePricingSourceDialog(props: {
  platformId: string;
  onOpenChange: (open: boolean) => void;
}) {
  const source = useQuery({
    queryKey: ["newapi-remote-model-pricing-source", props.platformId],
    queryFn: ({ signal }) => api.remoteModelPricingSource(props.platformId, signal),
    retry: false,
    gcTime: 0,
    staleTime: 60_000,
  });
  return (
    <>
      {source.error && <QueryErrorToast error={source.error} fallback="远程价卡原始文件读取失败" />}
      <RawPricingSourceDialog
        open
        onRetry={() => void source.refetch()}
        source={source.data}
        pending={source.isFetching}
        error={source.error instanceof Error ? source.error.message : ""}
        onOpenChange={props.onOpenChange}
      />
    </>
  );
}
