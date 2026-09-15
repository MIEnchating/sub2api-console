import { useState, type ReactElement } from "react";
import { FileSearch, RotateCw } from "lucide-react";
import { JsonEditor } from "@/components/json-editor";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { resultStatusLabels } from "../constants";
import { resultCanRetry, type WorkbenchResultItem } from "../lib/task-results";

export function WorkbenchTaskResults(props: {
  items: WorkbenchResultItem[];
  onRetry?: (indexes: number[]) => void;
}): ReactElement {
  const [selected, setSelected] = useState<number[]>([]);
  const [report, setReport] = useState<WorkbenchResultItem | null>(null);
  const retryItems = props.items.filter(resultCanRetry);
  const indexes = selected.filter((index) => retryItems.some((item) => item.index === index));
  return (
    <>
      <ul aria-label="账号处理结果" className="max-h-96 divide-y overflow-y-auto">
        {props.items.map((item) => (
          <li key={item.position} className="space-y-1 py-2 text-sm wrap-anywhere">
            <div className="flex flex-wrap items-center gap-2">
              {props.onRetry && resultCanRetry(item) && (
                <Checkbox
                  checked={indexes.includes(item.index!)}
                  aria-label={`重试第 ${item.index! + 1} 项 ${item.name}`}
                  onCheckedChange={(checked) =>
                    setSelected(
                      checked
                        ? [...selected, item.index!]
                        : selected.filter((index) => index !== item.index),
                    )
                  }
                />
              )}
              <span>{item.name}</span>
              <Badge variant="outline">
                {Object.hasOwn(resultStatusLabels, item.status)
                  ? resultStatusLabels[item.status]
                  : item.status || "已处理"}
              </Badge>
              {item.accountId && (
                <span className="text-muted-foreground">账号 ID {item.accountId}</span>
              )}
              {item.report && (
                <Button
                  variant="outline"
                  onClick={() => setReport(item)}
                  aria-label={`查看 ${item.name} 的报告`}
                >
                  <FileSearch aria-hidden="true" />
                  报告
                </Button>
              )}
            </div>
            {item.message && <p className="text-muted-foreground">{item.message}</p>}
          </li>
        ))}
      </ul>
      {props.onRetry && retryItems.length > 0 && (
        <Button
          variant="outline"
          disabled={!indexes.length}
          onClick={() => props.onRetry?.(indexes)}
        >
          <RotateCw aria-hidden="true" />
          重新处理 {indexes.length} 项
        </Button>
      )}
      <Dialog
        open={report !== null}
        onOpenChange={(open) => {
          if (!open) setReport(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{report?.name ?? "账号"}处理报告</DialogTitle>
          </DialogHeader>
          {report && (
            <JsonEditor
              value={JSON.stringify(report.report, null, 2)}
              readOnly
              aria-label="账号处理报告 JSON"
              className="h-96 max-h-[65svh]"
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
