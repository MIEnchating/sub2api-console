import { BadgeCheck, SpellCheck2, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";

export function OnboardingMaintenanceActions(props: {
  accountCount: number;
  pending: boolean;
  onRevalidate: () => void;
  onRepairNames: () => void;
  onCleanupKeys: () => void;
}) {
  const disabled = props.pending || props.accountCount === 0;
  return (
    <>
      <Button type="button" variant="outline" disabled={disabled} onClick={props.onRevalidate}>
        <BadgeCheck aria-hidden="true" />
        复验绑定
      </Button>
      <Button type="button" variant="outline" disabled={disabled} onClick={props.onRepairNames}>
        <SpellCheck2 aria-hidden="true" />
        名称修复
      </Button>
      <Button
        type="button"
        variant="outline"
        disabled={props.pending}
        onClick={props.onCleanupKeys}
      >
        <Trash2 aria-hidden="true" />
        清理无用 Key
      </Button>
    </>
  );
}
