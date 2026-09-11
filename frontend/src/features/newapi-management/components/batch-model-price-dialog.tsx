import { QueryErrorToast } from "@/components/query-error-toast";
import type { NewAPIModelPrice, Sub2APIModelPrice } from "@/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export type BatchModelPricePreview = {
  model: string;
  price?: NewAPIModelPrice;
  reference?: Sub2APIModelPrice;
  reason?: string;
  differences: Array<{ label: string; configured: string; remote: string; matched: boolean }>;
};

export function PriceSelectionCheckbox(props: {
  models: string[];
  selected: ReadonlySet<string>;
  label: string;
  disabled?: boolean;
  onChange: (models: string[], checked: boolean) => void;
}) {
  const count = props.models.filter((model) => props.selected.has(model)).length;
  return (
    <Checkbox
      aria-label={props.label}
      checked={props.models.length > 0 && count === props.models.length}
      indeterminate={count > 0 && count < props.models.length}
      disabled={props.disabled || props.models.length === 0}
      onCheckedChange={(checked) => props.onChange(props.models, checked)}
    />
  );
}

export function BatchModelPriceDialog(props: {
  preview: BatchModelPricePreview[] | null;
  selectedCount: number;
  preparing: boolean;
  writing: boolean;
  error: string;
  results: Record<string, string> | null;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const writable = props.preview?.filter((row) => row.price) ?? [];
  const busy = props.preparing || props.writing;
  return (
    <Dialog
      open={props.preview !== null}
      onOpenChange={(open) => {
        if (!open && !busy) props.onClose();
      }}
    >
      <DialogContent width="wide" height="adaptive" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>{props.results ? "批量同步结果" : "批量同步模型价格"}</DialogTitle>
          <DialogDescription>
            {props.results
              ? `本次提交 ${writable.length} 个模型。请核对下方读回结果，未匹配和跳过的模型仍保留选择。`
              : `已选择 ${props.selectedCount} 个模型，可同步 ${writable.length} 个。确认后将覆盖这些模型的价格配置；缺失或不支持的模型会跳过。`}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          {props.preparing ? <p role="status">正在准备批量价格预览…</p> : null}
          {props.error ? <QueryErrorToast error={props.error} fallback="批量价格操作失败" /> : null}
          {!props.preparing && props.preview?.length ? (
            <Table
              className="min-w-[56rem]"
              containerClassName="max-h-[55vh] overflow-auto"
              overflowTooltip={false}
            >
              <TableHeader>
                <TableRow>
                  <TableHead>模型</TableHead>
                  <TableHead>当前价格</TableHead>
                  <TableHead>同步后价格</TableHead>
                  <TableHead>状态</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {props.preview.map((row) => (
                  <TableRow key={row.model}>
                    <TableCell className="min-w-40 max-w-64 break-all whitespace-normal align-top">
                      {row.model}
                    </TableCell>
                    <TableCell className="align-top">
                      {row.differences.map((field) => (
                        <div key={field.label} className="whitespace-nowrap text-xs leading-6">
                          {field.label}：{field.configured}
                        </div>
                      ))}
                    </TableCell>
                    <TableCell className="align-top">
                      {row.differences.map((field) => (
                        <div key={field.label} className="whitespace-nowrap text-xs leading-6">
                          {field.label}：{field.remote}
                        </div>
                      ))}
                    </TableCell>
                    <TableCell className="min-w-32 whitespace-normal align-top">
                      {props.results?.[row.model] ?? row.reason ?? "待同步"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={props.onClose}>
            {props.results ? "关闭" : "取消"}
          </Button>
          {!props.results ? (
            <Button disabled={busy || writable.length === 0} onClick={props.onConfirm}>
              {props.writing ? "正在同步并读回…" : `确认同步 ${writable.length} 个模型`}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
