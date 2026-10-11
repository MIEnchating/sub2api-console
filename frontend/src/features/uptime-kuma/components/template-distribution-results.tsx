import type { Task } from "@/api";
import { templateDistributionStatusLabels } from "../constants";

export function TemplateDistributionResults(props: { task: Task | null }) {
  const items: unknown = props.task?.result.items;
  if (!Array.isArray(items)) return null;
  return (
    <ul aria-label="参数下发结果" className="space-y-2">
      {items.map((value: unknown) => {
        if (
          typeof value !== "object" ||
          value === null ||
          !("monitor_id" in value) ||
          !("name" in value) ||
          !("status" in value) ||
          !("message" in value)
        )
          return null;
        return (
          <li key={String(value.monitor_id)} className="rounded-md border p-3 text-sm break-words">
            {String(value.name)}（#{String(value.monitor_id)}）：
            {templateDistributionStatusLabels[String(value.status)] ?? String(value.status)}
            <p className="text-muted-foreground">{String(value.message)}</p>
          </li>
        );
      })}
    </ul>
  );
}
