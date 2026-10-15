import { useId, type ReactElement } from "react";
import { Checkbox } from "@/components/ui/checkbox";

export function VerificationSetup(props: {
  fingerprint: boolean;
  disabled: boolean;
  onFingerprintChange: (enabled: boolean) => void;
}): ReactElement {
  const controlID = useId();
  return (
    <section
      aria-label="检测步骤"
      className="shrink-0 space-y-2 rounded-md border bg-muted/20 p-3 text-sm"
    >
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <label htmlFor={controlID} className="inline-flex shrink-0 items-center gap-2">
          <Checkbox
            id={controlID}
            checked={props.fingerprint}
            disabled={props.disabled}
            onCheckedChange={props.onFingerprintChange}
            aria-describedby={`${controlID}-description`}
          />
          采集分词指纹
        </label>
        <span id={`${controlID}-description`} className="text-xs text-muted-foreground">
          每个组合额外 10 次请求：基准、中文、英文、代码、Unicode 各采样 2 次。
        </span>
      </div>
    </section>
  );
}
