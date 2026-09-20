import type { ReactElement } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { TemplateModelMapping } from "./template-model-mapping";
import type { WorkbenchAccount } from "../types";
import { fingerprintLabels } from "../constants";

export function AccountDetails(props: {
  account: WorkbenchAccount;
  onClose: () => void;
}): ReactElement {
  const rows = [
    ["代理", props.account.proxy_name || "直连"],
    ["并发数", props.account.concurrency || "未提供"],
    ["负载因子", props.account.load_factor || "默认"],
    ["计费倍率", props.account.rate_multiplier || "未提供"],
    [
      "Codex 指纹",
      fingerprintLabels[props.account.fingerprint] ||
        (props.account.fingerprint ? "其他指纹模式" : fingerprintLabels.off),
    ],
  ];
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent width="progress">
        <DialogHeader>
          <DialogTitle>账号配置 · {props.account.name || props.account.id}</DialogTitle>
          <DialogDescription>
            账号 #{props.account.id} · {props.account.email || "未提供邮箱"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-5">
          <dl className="grid grid-cols-2 gap-4 rounded-lg bg-muted/30 p-3 text-sm sm:grid-cols-3">
            {rows.map(([name, value]) => (
              <div key={name} className="min-w-0">
                <dt className="text-xs text-muted-foreground">{name}</dt>
                <dd className="mt-1 min-w-0 wrap-anywhere">{value}</dd>
              </div>
            ))}
          </dl>
          <TemplateModelMapping mapping={props.account.model_mapping} />
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
