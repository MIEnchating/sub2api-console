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
import { runStatusLabels } from "../constants";
import { checkError, checkLabel, runItemMessage } from "../lib/check-result";
import { RunLoginInput } from "./run-login-input";

export function RunItems(props: {
  run: WorkbenchRun;
  pending: boolean;
  onEnable: (item: WorkbenchRunItem) => void;
}): ReactElement {
  const [login, setLogin] = useState<WorkbenchRunItem | null>(null);
  const [check, setCheck] = useState<WorkbenchRunItem | null>(null);
  const liveLogin = props.run.items.find(
    (item) => item.id === login?.id && item.login_prompt?.id === login.login_prompt?.id,
  );
  return (
    <>
      <Table
        aria-label="账号处理结果"
        uniformTextSize={false}
        className="min-w-[720px]"
        containerClassName="overflow-auto rounded-lg border"
      >
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
                <p className="mt-1 text-xs text-muted-foreground">{runItemMessage(item)}</p>
              </TableCell>
              <TableCell className="whitespace-normal wrap-anywhere" overflowTooltip={false}>
                {item.template_name}
                {item.check && (
                  <p className="text-xs text-muted-foreground">{checkLabel(item.check)}</p>
                )}
              </TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-2">
                  {item.login_prompt && (
                    <Button
                      disabled={
                        props.run.status !== "running" ||
                        new Date(props.run.expires_at).getTime() <= Date.now()
                      }
                      onClick={() => setLogin(item)}
                    >
                      {item.login_prompt.kind === "password" ? "输入密码" : "输入验证码"}
                    </Button>
                  )}
                  {item.check && (
                    <Button variant="outline" onClick={() => setCheck(item)}>
                      检测详情
                    </Button>
                  )}
                  {item.status === "review" && item.check && item.account_id && (
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
      {liveLogin?.login_prompt && (
        <RunLoginInput
          runID={props.run.id}
          key={liveLogin.login_prompt.id}
          prompt={liveLogin.login_prompt}
          itemID={liveLogin.id}
          email={liveLogin.email}
          onClose={() => setLogin(null)}
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
              <p className="mb-3 text-sm">{checkLabel(check.check)}</p>
              {checkError(check.check) && (
                <p className="mb-3 text-sm text-destructive">{checkError(check.check)}</p>
              )}
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
