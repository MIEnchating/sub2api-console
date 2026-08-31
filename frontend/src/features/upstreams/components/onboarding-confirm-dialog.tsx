import { ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
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

export type OnboardingBindingPreview = {
  upstream: string;
  upstreamGroup: string;
  multiplier: string;
  localGroupMultiplier: string;
  localGroup: string;
  concurrency: number;
  priority: number;
  status: "待添加" | "待更新";
};

export function OnboardingConfirmContent(props: {
  items: OnboardingBindingPreview[];
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>确认账号绑定变更</DialogTitle>
        <DialogDescription>
          请核对 {props.items.length} 条分组绑定；新增项将创建账号，更新项只修改现有账号分组。
        </DialogDescription>
      </DialogHeader>
      <div className="min-h-0 overflow-auto rounded-md border">
        <Table className="min-w-[960px] table-fixed">
          <TableHeader className="sticky top-0 z-10">
            <TableRow>
              <TableHead className="w-[16%]">上游</TableHead>
              <TableHead className="w-[18%]">上游分组</TableHead>
              <TableHead className="w-[10%]">上游倍率</TableHead>
              <TableHead className="w-[13%]">本地分组倍率</TableHead>
              <TableHead className="w-[17%]">本地分组</TableHead>
              <TableHead className="w-[8%]">并发</TableHead>
              <TableHead className="w-[8%]">优先级</TableHead>
              <TableHead className="w-[10%]">状态</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.items.map((item, index) => (
              <TableRow key={`${item.upstream}:${item.upstreamGroup}:${item.localGroup}:${index}`}>
                <TableCell className="font-medium">{item.upstream}</TableCell>
                <TableCell>{item.upstreamGroup}</TableCell>
                <TableCell className="tabular-nums">{item.multiplier}</TableCell>
                <TableCell className="tabular-nums">{item.localGroupMultiplier}</TableCell>
                <TableCell>{item.localGroup}</TableCell>
                <TableCell className="tabular-nums">{item.concurrency}</TableCell>
                <TableCell className="tabular-nums">{item.priority}</TableCell>
                <TableCell>
                  <Badge variant="outline">{item.status}</Badge>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <DialogFooter>
        <Button variant="outline" disabled={props.pending} onClick={props.onCancel}>
          取消
        </Button>
        <Button disabled={props.pending || props.items.length === 0} onClick={props.onConfirm}>
          <ShieldCheck aria-hidden="true" />
          {props.pending ? "正在提交" : `确认提交 ${props.items.length} 项变更`}
        </Button>
      </DialogFooter>
    </>
  );
}

export function OnboardingConfirmDialog(props: {
  open: boolean;
  items: OnboardingBindingPreview[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!props.pending) props.onOpenChange(open);
      }}
    >
      <DialogContent
        width="wide"
        height="large"
        className="grid min-w-0 grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <OnboardingConfirmContent
          items={props.items}
          pending={props.pending}
          onCancel={() => props.onOpenChange(false)}
          onConfirm={props.onConfirm}
        />
      </DialogContent>
    </Dialog>
  );
}
