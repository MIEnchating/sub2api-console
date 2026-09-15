import { useState, type ReactElement } from "react";
import { FileSearch, RotateCw } from "lucide-react";
import { JsonEditor } from "@/components/json-editor";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { resultStatusLabels } from "../constants";
import { resultCanRetry, type WorkbenchResultItem } from "../lib/task-results";
import { WorkbenchBehaviorSummary } from "./workbench-behavior-summary";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

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
      <Table
        aria-label="账号处理结果"
        className="min-w-[560px]"
        containerClassName="max-h-96 overflow-auto"
      >
        <TableHeader>
          <TableRow>
            <TableHead>账号</TableHead>
            <TableHead>配置</TableHead>
            <TableHead>进度</TableHead>
            <TableHead>行为检测</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.items.map((item) => (
            <TableRow key={item.position}>
              <TableCell
                className="max-w-56 whitespace-normal wrap-anywhere"
                overflowTooltip={false}
              >
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
                </div>
                {item.accountId && (
                  <p className="text-xs text-muted-foreground">账号 ID {item.accountId}</p>
                )}
              </TableCell>
              <TableCell className="max-w-40 whitespace-normal wrap-anywhere">
                {item.templateName || "默认配置"}
              </TableCell>
              <TableCell
                className="max-w-64 whitespace-normal wrap-anywhere"
                overflowTooltip={false}
              >
                <Badge variant="outline">
                  {Object.hasOwn(resultStatusLabels, item.status)
                    ? resultStatusLabels[item.status]
                    : item.status || "已处理"}
                </Badge>
                {item.message && (
                  <p className="mt-1 text-xs text-muted-foreground">{item.message}</p>
                )}
              </TableCell>
              <TableCell
                className="max-w-56 whitespace-normal wrap-anywhere"
                overflowTooltip={false}
              >
                <WorkbenchBehaviorSummary report={item.report} />
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
                {!item.report && <span className="text-muted-foreground">-</span>}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
