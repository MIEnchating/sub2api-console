import type { RemoteModelPricingSource } from "@/api";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

type RawPricingSourceContentProps = {
  source?: RemoteModelPricingSource;
  pending: boolean;
  error: string;
};

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / (1024 * 1024)).toFixed(2)} MB`;
}

function SourceMetadata(props: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border bg-muted/20 px-3 py-2">
      <dt className="text-muted-foreground text-xs">{props.label}</dt>
      <dd className="mt-1 overflow-x-auto font-mono text-xs whitespace-nowrap">{props.value}</dd>
    </div>
  );
}

export function RawPricingSourceContent(props: RawPricingSourceContentProps) {
  if (props.pending && !props.source) {
    return (
      <div className="text-muted-foreground grid min-h-80 place-items-center text-sm" role="status">
        正在读取远程价卡原始文件
      </div>
    );
  }
  if (props.error && !props.source) {
    return (
      <div className="text-destructive grid min-h-80 place-items-center px-6 text-center text-sm">
        {props.error}
      </div>
    );
  }
  if (!props.source) {
    return (
      <div className="text-muted-foreground grid min-h-80 place-items-center text-sm">
        尚未读取到原始价卡
      </div>
    );
  }

  return (
    <div className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-3">
      <div className="grid gap-2 lg:grid-cols-2">
        <SourceMetadata label="来源 URL" value={props.source.source_url} />
        <SourceMetadata
          label="后端抓取时间"
          value={new Date(props.source.fetched_at).toLocaleString("zh-CN")}
        />
        <SourceMetadata label="文件大小" value={formatBytes(props.source.size_bytes)} />
        <SourceMetadata label="SHA-256" value={props.source.sha256} />
      </div>
      <div className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-2">
        <p className="text-muted-foreground text-xs">
          后端实际拉取的原始内容，未经过 JSON 重排、补字段或价格换算。
        </p>
        <pre
          className="min-h-0 overflow-auto rounded-md border bg-muted/20 p-3 font-mono text-xs leading-5 whitespace-pre"
          data-slot="raw-pricing-source"
        >
          {props.source.content}
        </pre>
      </div>
    </div>
  );
}

export function RawPricingSourceDialog(props: {
  open: boolean;
  source?: RemoteModelPricingSource;
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent width="table" height="tall" className="grid grid-rows-[auto_minmax(0,1fr)]">
        <DialogHeader>
          <DialogTitle>远程价卡原始文件</DialogTitle>
          <DialogDescription>用于核对远程仓库内容与 Console 后端解析结果。</DialogDescription>
        </DialogHeader>
        <DialogBody className="min-h-0 overflow-hidden pr-0">
          <RawPricingSourceContent
            source={props.source}
            pending={props.pending}
            error={props.error}
          />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
