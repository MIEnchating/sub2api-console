import type { ReactElement } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { TaskStartupState } from "@/components/task-startup-state";
import {
  Table,
  TableHeader,
  TableHead,
  TableRow,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import type { WorkbenchPreview } from "../types";
import { inputKindLabels } from "../constants";

export function ImportPreview(props: {
  preview: WorkbenchPreview;
  pending: boolean;
  onClose: () => void;
  onConfirm: () => void;
}): ReactElement {
  const isImport = props.preview.action === "import";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent width="wide">
        <DialogHeader>
          <DialogTitle>确认本批{isImport ? "导入" : "输出"}</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid gap-4">
          <p className="text-sm">
            共 {props.preview.items.length} 项，已合并 {props.preview.duplicate_count} 项重复资料。
            {isImport ? "账号先以停用状态导入。" : "本批将生成服务器私有 JSON。"}
          </p>
          {isImport && (
            <p className="text-sm text-muted-foreground">
              模板：{props.preview.template?.name || "默认配置"} ·{" "}
              {props.preview.check ? "执行检测" : "不检测"} ·{" "}
              {props.preview.promote ? "通过后启用" : "保持停用"}
            </p>
          )}
          {props.preview.errors.length > 0 && (
            <ul className="grid gap-1 text-sm text-destructive">
              {props.preview.errors.map((item) => (
                <li key={item.index}>
                  第 {item.index + 1} 项：{item.message}
                </li>
              ))}
            </ul>
          )}
          <Table aria-label="本批账号预览" containerClassName="max-h-80 overflow-auto">
            <TableHeader>
              <TableRow>
                <TableHead>序号</TableHead>
                <TableHead>账号</TableHead>
                <TableHead>识别类型</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.preview.items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell>{item.index + 1}</TableCell>
                  <TableCell
                    className="max-w-80 whitespace-normal wrap-anywhere"
                    overflowTooltip={false}
                  >
                    {item.email || item.name || "刷新后识别身份"}
                  </TableCell>
                  <TableCell>{inputKindLabels[item.kind] || "其他资料格式"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {props.pending && <TaskStartupState message="正在创建账号处理任务" />}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
            返回修改
          </Button>
          <Button
            disabled={
              props.pending || props.preview.errors.length > 0 || props.preview.items.length === 0
            }
            onClick={props.onConfirm}
          >
            确认并开始
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
