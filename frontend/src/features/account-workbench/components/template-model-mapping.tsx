import { useId, useState, type ReactElement } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

export function TemplateModelMapping(props: { mapping: unknown; compact?: boolean }): ReactElement {
  const id = useId();
  const [expanded, setExpanded] = useState(false);
  const entries =
    typeof props.mapping === "object" && props.mapping !== null
      ? Object.entries(props.mapping)
      : [];
  const collapsible = props.compact && entries.length > 3;
  const visible = collapsible && !expanded ? entries.slice(0, 3) : entries;
  return (
    <section
      aria-label="模型映射"
      className={cn(
        "col-span-full min-w-0 space-y-2",
        props.compact &&
          "@min-[48rem]:col-span-1 @min-[48rem]:col-start-2 @min-[48rem]:row-span-2 @min-[48rem]:row-start-1",
      )}
    >
      <div className="flex items-center gap-2">
        <h4 className="text-xs font-medium text-muted-foreground">模型映射</h4>
        {entries.length > 0 && (
          <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground">
            {entries.length} 条
          </span>
        )}
      </div>
      {entries.length === 0 ? (
        <p className="text-sm">不限制</p>
      ) : (
        <div className="overflow-hidden rounded-lg border">
          <table id={id} aria-label="模型映射" className="w-full table-fixed text-left text-xs">
            <thead className="bg-muted/40 text-muted-foreground">
              <tr>
                <th scope="col" className="px-3 py-2 font-medium">
                  来源模型
                </th>
                <th scope="col" className="px-3 py-2 font-medium">
                  目标模型
                </th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {visible.map(([from, to]) => (
                <tr key={from}>
                  <td className="px-3 py-2 align-top wrap-anywhere">{from}</td>
                  <td className="px-3 py-2 align-top wrap-anywhere">{String(to)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {collapsible && (
            <div className="border-t px-2 py-1">
              <Button
                type="button"
                variant="ghost"
                className="w-full text-muted-foreground"
                aria-expanded={expanded}
                aria-controls={id}
                onClick={() => setExpanded(!expanded)}
              >
                {expanded ? <ChevronUp aria-hidden="true" /> : <ChevronDown aria-hidden="true" />}
                {expanded ? "收起映射" : `展开全部 ${entries.length} 条映射`}
              </Button>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
