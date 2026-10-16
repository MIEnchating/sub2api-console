import type { ReactElement } from "react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { verificationProbeLabels } from "../constants";
import {
  verificationNumber as number,
  verificationText as text,
  verificationProbeGroups,
  type VerificationRecord,
} from "../lib/verification-progress";
import { VerificationStateIcon } from "./verification-state";

export function VerificationProbeNavigation(props: {
  probes: VerificationRecord[];
  selected: number;
  onSelect: (index: number) => void;
}): ReactElement {
  const current = props.probes[props.selected] ?? props.probes[0];
  return (
    <>
      <div className="min-w-0 border-b p-3 md:hidden">
        <Select
          value={String(props.selected)}
          onValueChange={(value) => {
            if (value !== null) props.onSelect(Number(value));
          }}
          itemToStringLabel={(value) => text(props.probes[Number(value)]?.label) || "选择检测项"}
          disabled={props.probes.length === 0}
        >
          <SelectTrigger aria-label="选择检测项" className="w-full min-w-0">
            <SelectValue>{text(current?.label) || "暂无检测项"}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {props.probes.map((probe, index) => (
              <SelectItem key={index} value={String(index)}>
                {text(probe.label)} · {verificationProbeLabels[text(probe.status)] ?? "待判定"}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <nav
        aria-label="检测项"
        className="hidden min-h-0 w-56 shrink-0 space-y-4 overflow-y-auto border-r bg-muted/15 p-2 md:block"
      >
        {verificationProbeGroups(props.probes).map((group) => (
          <section key={group.label} aria-label={group.label}>
            <h4 className="flex items-center justify-between px-2 py-2 text-xs font-medium text-muted-foreground">
              <span>{group.label}</span>
              <span className="tabular-nums">{group.probes.length}</span>
            </h4>
            <div className="space-y-0.5">
              {group.probes.map((probe) => {
                const index = props.probes.indexOf(probe);
                return (
                  <Button
                    key={index}
                    variant="ghost"
                    aria-label={`第 ${number(probe.round) ?? "—"} 轮 · ${verificationProbeLabels[text(probe.status)] ?? "待判定"} ${text(probe.label)}`}
                    aria-pressed={props.selected === index}
                    onClick={() => props.onSelect(index)}
                    className="w-full min-w-0 justify-start gap-2 px-2 text-xs font-normal aria-pressed:bg-primary/10 aria-pressed:text-primary"
                  >
                    <VerificationStateIcon status={text(probe.status)} />
                    <span className="min-w-0 flex-1 truncate text-left" title={text(probe.label)}>
                      {text(probe.label)}
                    </span>
                  </Button>
                );
              })}
            </div>
          </section>
        ))}
        {props.probes.length === 0 ? (
          <p className="p-2 text-sm text-muted-foreground">没有逐项记录</p>
        ) : null}
      </nav>
    </>
  );
}
