import type { ReactElement } from "react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";

export function allocationOverrides(value: unknown): Record<string, boolean> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(
    Object.entries(value).filter(
      (entry): entry is [string, boolean] => typeof entry[1] === "boolean",
    ),
  );
}

export function validAllocationOverrides(value: unknown, accounts: boolean): boolean {
  if (value === undefined) return true;
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  return Object.entries(value).every(
    ([id, enabled]) =>
      id.trim() === id &&
      id.length > 0 &&
      (!accounts || /^[1-9]\d*$/.test(id)) &&
      typeof enabled === "boolean",
  );
}

export function UpstreamAllocationOverrides(props: {
  label: string;
  values: Record<string, boolean>;
  names: Map<string, string>;
  onChange: (value: Record<string, boolean>) => void;
}): ReactElement | null {
  const entries = Object.entries(props.values);
  if (entries.length === 0) return null;
  return (
    <div className="min-w-0 space-y-2 @min-[56rem]/policy-card:col-span-3">
      <p className="text-sm font-medium">{props.label}</p>
      <div className="divide-y rounded-lg border">
        {entries.map(([id, enabled]) => {
          const name = props.names.get(id) ?? id;
          return (
            <div key={id} className="flex flex-wrap items-center gap-3 p-3">
              <span className="min-w-0 flex-1 break-all text-sm">{name}</span>
              <Switch
                aria-label={`${name}共享并发分配`}
                checked={enabled}
                onCheckedChange={(value) => props.onChange({ ...props.values, [id]: value })}
              />
              <Button
                type="button"
                variant="outline"
                aria-label={`${name}恢复跟随`}
                onClick={() => {
                  const next = { ...props.values };
                  delete next[id];
                  props.onChange(next);
                }}
              >
                恢复跟随
              </Button>
            </div>
          );
        })}
      </div>
    </div>
  );
}
