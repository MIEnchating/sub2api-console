import { useEffect, useMemo, useState } from "react";
import { Search } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

type Props = {
  open: boolean;
  models: string[];
  selected: string[];
  pending: boolean;
  error: string;
  onOpenChange: (open: boolean) => void;
  onSelectedChange: (models: string[]) => void;
  onConfirm: () => void;
};

export function filterChannelModels(models: string[], search: string): string[] {
  const query = search.trim().toLocaleLowerCase();
  if (!query) return models;
  return models.filter((model) => model.toLocaleLowerCase().includes(query));
}

export function NewAPIChannelModelDialog(props: Props) {
  const [search, setSearch] = useState("");
  const visibleModels = useMemo(
    () => filterChannelModels(props.models, search),
    [props.models, search],
  );

  useEffect(() => {
    if (!props.open) setSearch("");
  }, [props.open]);

  function toggleModel(model: string, checked: boolean) {
    if (checked) {
      props.onSelectedChange([...new Set([...props.selected, model])]);
      return;
    }
    props.onSelectedChange(props.selected.filter((item) => item !== model));
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent height="large" className="grid grid-rows-[auto_minmax(0,1fr)_auto]">
        <DialogHeader>
          <DialogTitle>选择上游模型</DialogTitle>
          <DialogDescription>选择要添加到 New API 渠道的模型。</DialogDescription>
        </DialogHeader>
        <DialogBody className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-48 flex-1">
              <Search
                className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
                aria-hidden="true"
              />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="搜索模型"
                aria-label="搜索上游模型"
                className="pl-8"
                disabled={props.pending}
              />
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={props.pending || props.models.length === 0}
              onClick={() => props.onSelectedChange(props.models)}
            >
              全选
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={props.pending || props.selected.length === 0}
              onClick={() => props.onSelectedChange([])}
            >
              清空
            </Button>
          </div>
          <div
            className="min-h-0 overflow-y-auto rounded-md border"
            role="list"
            aria-label="上游模型"
          >
            {props.pending && (
              <div className="grid gap-3 p-3" role="status" aria-label="正在从上游获取模型">
                {Array.from({ length: 7 }, (_, index) => (
                  <Skeleton key={index} className="h-10 w-full" />
                ))}
              </div>
            )}
            {!props.pending && props.error && (
              <div className="text-destructive grid min-h-44 place-items-center p-6 text-center text-sm">
                {props.error}
              </div>
            )}
            {!props.pending && !props.error && visibleModels.length === 0 && (
              <div className="text-muted-foreground grid min-h-44 place-items-center p-6 text-center text-sm">
                {search ? "没有匹配的模型" : "上游未返回模型"}
              </div>
            )}
            {!props.pending &&
              !props.error &&
              visibleModels.length > 0 &&
              visibleModels.map((model) => {
                const checked = props.selected.includes(model);
                return (
                  <label
                    key={model}
                    className={cn(
                      "flex min-h-11 cursor-pointer items-center gap-3 border-b px-3 py-2 last:border-b-0",
                      checked ? "bg-primary/5" : "hover:bg-muted/40",
                    )}
                    role="listitem"
                  >
                    <Checkbox
                      checked={checked}
                      onCheckedChange={(next) => toggleModel(model, next)}
                      aria-label={`选择模型 ${model}`}
                    />
                    <span className="min-w-0 flex-1 break-all font-mono text-xs">{model}</span>
                  </label>
                );
              })}
          </div>
        </DialogBody>
        <DialogFooter className="items-center sm:justify-between">
          <span className="text-muted-foreground text-xs">
            已选择 {props.selected.length} 个模型
          </span>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => props.onOpenChange(false)}>
              取消
            </Button>
            <Button
              type="button"
              disabled={props.pending || props.selected.length === 0}
              onClick={props.onConfirm}
            >
              确认模型
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
