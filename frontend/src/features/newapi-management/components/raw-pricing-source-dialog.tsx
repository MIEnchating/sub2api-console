import { useId, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { JsonEditor } from "@/components/json-editor";
import type { RemoteModelPricingSource } from "@/api";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { parseRawPricingSource } from "../lib/raw-pricing-source";
import { RawPricingModelBrowser } from "./raw-pricing-model-browser";
import { RawPricingSourceActions, RawPricingSourceMetadata } from "./raw-pricing-source-tools";

type RawPricingSourceContentProps = {
  source?: RemoteModelPricingSource;
  pending: boolean;
  error: string;
  onRetry?: () => void;
};

function RawPricingReader(props: { source: RemoteModelPricingSource }): ReactNode {
  const id = useId();
  const [requestedView, setView] = useState<"models" | "raw">("models");
  const document = useMemo(
    () => parseRawPricingSource(props.source.content),
    [props.source.content],
  );
  const view = document.valid ? requestedView : "raw";
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-3">
      <div className="min-w-0 shrink-0 space-y-2">
        {props.source.warning ? (
          <p role="status" className="text-destructive text-xs">
            {props.source.warning}
          </p>
        ) : null}
        <RawPricingSourceMetadata source={props.source} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <SegmentedControl role="tablist" aria-label="价卡视图">
            <SegmentedControlItem
              id={`${id}-models-tab`}
              role="tab"
              aria-controls={`${id}-models`}
              selected={view === "models"}
              disabled={!document.valid}
              onClick={() => setView("models")}
            >
              模型明细
            </SegmentedControlItem>
            <SegmentedControlItem
              id={`${id}-raw-tab`}
              role="tab"
              aria-controls={`${id}-raw`}
              selected={view === "raw"}
              onClick={() => setView("raw")}
            >
              原始 JSON
            </SegmentedControlItem>
          </SegmentedControl>
          <RawPricingSourceActions source={props.source} showCopy={view !== "raw"} />
        </div>
      </div>
      <div
        id={`${id}-models`}
        role="tabpanel"
        aria-labelledby={`${id}-models-tab`}
        hidden={view !== "models"}
        className="min-h-0 min-w-0 flex-1"
      >
        {document.valid ? <RawPricingModelBrowser entries={document.entries} /> : null}
      </div>
      <div
        id={`${id}-raw`}
        role="tabpanel"
        aria-labelledby={`${id}-raw-tab`}
        hidden={view !== "raw"}
        className="min-h-0 min-w-0 flex-1"
      >
        {view === "raw" ? (
          <JsonEditor
            aria-label="原始价卡内容"
            className="h-full"
            readOnly
            value={props.source.content}
          />
        ) : null}
      </div>
    </div>
  );
}

export function RawPricingSourceContent(props: RawPricingSourceContentProps): ReactNode {
  if (props.pending && !props.source)
    return <ContentLoading label="正在读取远程价卡原始文件" className="h-full" />;
  if (props.error && !props.source)
    return props.onRetry ? <ContentRetry onRetry={props.onRetry} /> : null;
  if (!props.source)
    return (
      <div className="text-muted-foreground grid h-full place-items-center text-sm">
        尚未读取到原始价卡
      </div>
    );
  return <RawPricingReader source={props.source} />;
}

export function RawPricingSourceDialog(
  props: RawPricingSourceContentProps & {
    open: boolean;
    onOpenChange: (open: boolean) => void;
  },
): ReactNode {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        width="wide"
        height="tall"
        className="h-[min(44rem,calc(100svh-2rem))] grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>远程价卡原始文件</DialogTitle>
          <DialogDescription className="sr-only">远程仓库模型价格与原始文件</DialogDescription>
        </DialogHeader>
        <DialogBody className="min-h-0 overflow-hidden pr-0">
          <RawPricingSourceContent
            source={props.source}
            pending={props.pending}
            error={props.error}
            onRetry={props.onRetry}
          />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
