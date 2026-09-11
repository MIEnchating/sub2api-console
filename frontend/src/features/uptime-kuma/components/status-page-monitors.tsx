import { ArrowDown, ArrowUp, Trash2 } from "lucide-react";
import type { KumaMonitor } from "@/api";
import { FormField } from "@/App";
import { MultiSelect } from "@/components/multi-select";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import type { ResourceValues } from "../lib/resource-schemas";

type PublicMonitors = ResourceValues["status_page"]["groups"][number]["monitorList"];

export function StatusPageMonitors(props: {
  groupIndex: number;
  monitors: KumaMonitor[];
  value: PublicMonitors;
  pending: boolean;
  onChange: (value: PublicMonitors) => void;
}) {
  const names = new Map(props.monitors.map((monitor) => [monitor.id, monitor.name]));
  const nameOf = (id: number): string => names.get(id) ?? `监控项 #${id}`;
  const move = (index: number, target: number): void => {
    if (props.pending || target < 0 || target >= props.value.length) return;
    const next = [...props.value];
    const [item] = next.splice(index, 1);
    next.splice(target, 0, item);
    props.onChange(next);
  };
  return (
    <div className="grid min-w-0 gap-3">
      <FormField label={`分组 ${props.groupIndex + 1} 监控项`}>
        <MultiSelect
          ariaLabel={`分组 ${props.groupIndex + 1} 监控项`}
          title="选择公开展示的监控项"
          options={[
            ...props.monitors.map((monitor) => ({
              label: monitor.name,
              value: String(monitor.id),
            })),
            ...props.value
              .filter((monitor) => !names.has(monitor.id))
              .map((monitor) => ({ label: nameOf(monitor.id), value: String(monitor.id) })),
          ]}
          selected={props.value.map((monitor) => String(monitor.id))}
          onChange={(values) => {
            const selected = new Set(values.map(Number));
            const next = props.value.filter((monitor) => selected.has(monitor.id));
            const existing = new Set(next.map((monitor) => monitor.id));
            for (const id of selected) {
              if (!existing.has(id)) next.push({ id, sendUrl: false });
            }
            props.onChange(next);
          }}
          disabled={props.pending}
        />
      </FormField>
      {props.value.length === 0 && <p className="text-muted-foreground text-sm">暂无展示监控项</p>}
      <ol aria-label={`分组 ${props.groupIndex + 1} 展示顺序`} className="grid min-w-0 gap-2">
        {props.value.map((monitor, index) => (
          <li
            key={monitor.id}
            className="flex min-w-0 flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center"
          >
            <span className="min-w-0 flex-1 text-sm [overflow-wrap:anywhere]">
              {nameOf(monitor.id)}
            </span>
            <div className="flex shrink-0 flex-wrap items-center gap-2">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  aria-label={`公开${nameOf(monitor.id)}的监控地址`}
                  checked={monitor.sendUrl}
                  disabled={props.pending}
                  onCheckedChange={(checked) =>
                    props.onChange(
                      props.value.map((item) =>
                        item.id === monitor.id ? { ...item, sendUrl: checked } : item,
                      ),
                    )
                  }
                />
                公开地址
              </label>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`上移监控项 ${nameOf(monitor.id)}`}
                  disabled={props.pending || index === 0}
                  onClick={() => move(index, index - 1)}
                >
                  <ArrowUp aria-hidden="true" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`下移监控项 ${nameOf(monitor.id)}`}
                  disabled={props.pending || index === props.value.length - 1}
                  onClick={() => move(index, index + 1)}
                >
                  <ArrowDown aria-hidden="true" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`移除监控项 ${nameOf(monitor.id)}`}
                  disabled={props.pending}
                  onClick={() =>
                    props.onChange(props.value.filter((item) => item.id !== monitor.id))
                  }
                >
                  <Trash2 aria-hidden="true" />
                </Button>
              </div>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
