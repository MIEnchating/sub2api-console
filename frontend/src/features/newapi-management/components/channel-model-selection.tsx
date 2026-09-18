import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";

export function ChannelModelSelection(props: {
  models: string[];
  selected: string[];
  onChange: (models: string[]) => void;
}) {
  const [search, setSearch] = useState("");
  const visible = props.models.filter((model) =>
    model.toLowerCase().includes(search.trim().toLowerCase()),
  );
  return (
    <>
      <TableFilterToolbar>
        <SearchField placeholder="搜索下架模型" value={search} onChange={setSearch} />
        <Button
          variant="outline"
          disabled={visible.length === 0}
          onClick={() => props.onChange([...new Set([...props.selected, ...visible])])}
        >
          全选结果
        </Button>
        <Button
          variant="outline"
          disabled={props.selected.length === 0}
          onClick={() => props.onChange([])}
        >
          清空
        </Button>
      </TableFilterToolbar>
      <div
        role="group"
        aria-label="选择下架模型"
        className="max-h-64 overflow-y-auto rounded-md border"
      >
        {visible.map((model) => (
          <label
            key={model}
            className="flex cursor-pointer items-center gap-3 border-b p-3 last:border-b-0 hover:bg-muted/40"
          >
            <Checkbox
              checked={props.selected.includes(model)}
              onCheckedChange={(checked) =>
                props.onChange(
                  checked
                    ? [...props.selected, model]
                    : props.selected.filter((value) => value !== model),
                )
              }
            />
            <span className="min-w-0 flex-1 break-all font-mono text-sm">{model}</span>
          </label>
        ))}
        {visible.length === 0 && (
          <p className="text-muted-foreground p-6 text-center text-sm">没有匹配的模型</p>
        )}
      </div>
    </>
  );
}
