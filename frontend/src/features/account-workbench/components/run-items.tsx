import { useState, type ReactElement } from "react";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableHeader,
  TableHead,
  TableRow,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { JsonEditor } from "@/components/json-editor";
import type { WorkbenchRun, WorkbenchRunItem } from "../types";
import { checkVerdictLabels, runStatusLabels } from "../constants";
import { RunBrowser } from "./run-browser";

export function RunItems(props: {
  run: WorkbenchRun;
  pending: boolean;
  onEnable: (item: WorkbenchRunItem) => void;
}): ReactElement {
  const [browser, setBrowser] = useState<WorkbenchRunItem | null>(null);
  const [check, setCheck] = useState<WorkbenchRunItem | null>(null);
  const liveBrowser = props.run.items.find((item) => item.id === browser?.id && item.browser_ready);
  return (
    <>
      <Table aria-label="账号处理结果" className="min-w-[640px]" containerClassName="overflow-auto">
        <TableHeader>
          <TableRow>
            {["账号", "状态", "模板 / 检测", "操作"].map((label) => (
              <TableHead key={label}>{label}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.run.items.map((item) => (
            <TableRow key={item.id}>
              <TableCell
                className="max-w-72 whitespace-normal wrap-anywhere"
                overflowTooltip={false}
              >
                {item.email || item.name || `第 ${item.index + 1} 项`}
                {item.account_id && (
                  <div className="text-xs text-muted-foreground">站点账号 #{item.account_id}</div>
                )}
              </TableCell>
              <TableCell
                className="max-w-80 whitespace-normal wrap-anywhere"
                overflowTooltip={false}
              >
                <Badge variant="secondary">{runStatusLabels[item.status] || "状态待确认"}</Badge>
                <p className="mt-1 text-xs text-muted-foreground">{item.message}</p>
              </TableCell>
              <TableCell>
                {item.template_name}
                {item.check && (
                  <p className="text-xs text-muted-foreground">
                    {checkVerdictLabels[String(item.check.verdict)] || "待复核结论"}
                  </p>
                )}
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-2">
                  {item.browser_ready && <Button onClick={() => setBrowser(item)}>完成验证</Button>}
                  {item.check && (
                    <Button variant="outline" onClick={() => setCheck(item)}>
                      检测详情
                    </Button>
                  )}
                  {item.status === "review" &&
                    item.check?.verdict === "INCONCLUSIVE" &&
                    item.account_id && (
                      <Button
                        variant="outline"
                        disabled={props.pending}
                        onClick={() => props.onEnable(item)}
                      >
                        启用
                      </Button>
                    )}
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {liveBrowser && (
        <RunBrowser
          runID={props.run.id}
          itemID={liveBrowser.id}
          email={liveBrowser.email}
          onClose={() => setBrowser(null)}
        />
      )}
      {check && (
        <Dialog
          open
          onOpenChange={(open) => {
            if (!open) setCheck(null);
          }}
        >
          <DialogContent width="wide">
            <DialogHeader>
              <DialogTitle>检测详情 · {check.email || check.name}</DialogTitle>
            </DialogHeader>
            <DialogBody>
              <p className="mb-3 text-sm">
                {checkVerdictLabels[String(check.check?.verdict)] || "待复核结论"}
              </p>
              <JsonEditor
                aria-label="检测报告"
                value={JSON.stringify(check.check, null, 2)}
                readOnly
                className="h-80"
              />
            </DialogBody>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
