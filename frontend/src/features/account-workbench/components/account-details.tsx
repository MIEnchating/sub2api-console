import type { ReactElement } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from "@/components/ui/dialog";
import type { WorkbenchAccount } from "../types";
import { fingerprintLabels } from "../constants";

export function AccountDetails(props: {
  account: WorkbenchAccount;
  onClose: () => void;
}): ReactElement {
  const models = Object.entries(props.account.model_mapping).map(([source, target]) =>
    source === target ? source : `${source} → ${target}`,
  );
  const rows = [
    ["代理", props.account.proxy_name || "直连"],
    ["并发数", props.account.concurrency || "未提供"],
    ["负载因子", props.account.load_factor || "默认"],
    ["计费倍率", props.account.rate_multiplier || "未提供"],
    ["模型限制", models.join("、") || "不限制"],
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
      <DialogContent>
        <DialogHeader>
          <DialogTitle>账号配置 · {props.account.name || props.account.id}</DialogTitle>
        </DialogHeader>
        <DialogBody>
          <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-3 text-sm">
            {rows.map(([name, value]) => (
              <div key={name} className="contents">
                <dt className="text-muted-foreground">{name}</dt>
                <dd className="min-w-0 wrap-anywhere">{value}</dd>
              </div>
            ))}
          </dl>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
