import type { KumaTemplateDistributionPreview } from "@/api";
import { Checkbox } from "@/components/ui/checkbox";

export function TemplateDistributionSelection(props: {
  preview: KumaTemplateDistributionPreview;
  selectedIDs: number[];
  disabled: boolean;
  onChange: (ids: number[]) => void;
}) {
  const eligibleIDs = props.preview.items
    .filter((item) => !item.reason)
    .map((item) => item.monitor_id);
  const selected = new Set(props.selectedIDs);
  return (
    <>
      <p className="text-sm">
        关联 {props.preview.items.length} 项，可下发 {eligibleIDs.length} 项，已选 {selected.size}{" "}
        项。
      </p>
      <div className="flex items-center gap-2 text-sm">
        <Checkbox
          id="distribution-select-all"
          aria-label="全选可下发项"
          checked={eligibleIDs.length > 0 && selected.size === eligibleIDs.length}
          indeterminate={selected.size > 0 && selected.size < eligibleIDs.length}
          disabled={props.disabled || eligibleIDs.length === 0}
          onCheckedChange={(checked) => props.onChange(checked ? eligibleIDs : [])}
        />
        <label htmlFor="distribution-select-all">全选可下发项</label>
      </div>
      {props.preview.items.length === 0 && (
        <p className="text-sm text-muted-foreground">暂无使用此模板的监控项。</p>
      )}
      <ul aria-label="参数下发影响范围" className="space-y-2">
        {props.preview.items.map((item) => (
          <li key={item.monitor_id} className="rounded-md border p-3 text-sm break-words">
            <div className="flex items-start gap-3">
              <Checkbox
                id={`distribution-monitor-${item.monitor_id}`}
                aria-labelledby={`distribution-name-${item.monitor_id}`}
                aria-describedby={`distribution-reason-${item.monitor_id}`}
                checked={selected.has(item.monitor_id)}
                disabled={props.disabled || !!item.reason}
                className="mt-0.5"
                onCheckedChange={(checked) =>
                  props.onChange(
                    checked
                      ? [...props.selectedIDs, item.monitor_id]
                      : props.selectedIDs.filter((id) => id !== item.monitor_id),
                  )
                }
              />
              <label
                htmlFor={`distribution-monitor-${item.monitor_id}`}
                className="min-w-0 flex-1 cursor-pointer"
              >
                <span id={`distribution-name-${item.monitor_id}`} className="font-medium">
                  <span className="sr-only">选择 </span>
                  {item.name}（#{item.monitor_id}）
                </span>
                <span
                  id={`distribution-reason-${item.monitor_id}`}
                  className="block text-muted-foreground"
                >
                  {item.reason || `模板版本 ${item.template_revision} → ${props.preview.revision}`}
                </span>
              </label>
            </div>
          </li>
        ))}
      </ul>
    </>
  );
}
