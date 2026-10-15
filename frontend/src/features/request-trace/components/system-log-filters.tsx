import { zodResolver } from "@hookform/resolvers/zod";
import { RefreshCw, RotateCcw, Search } from "lucide-react";
import { Controller, useForm } from "react-hook-form";

import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { logLevelOptions, timeRangeOptions } from "../constants";
import {
  defaultSystemLogFilters,
  systemLogSearchSchema,
  type SystemLogFilters,
} from "../lib/search-schema";

export function SystemLogFiltersForm(props: {
  pending: boolean;
  onSearch: (filters: SystemLogFilters) => void;
  onReset: () => void;
}): React.ReactElement {
  const form = useForm<SystemLogFilters>({
    resolver: zodResolver(systemLogSearchSchema),
    defaultValues: defaultSystemLogFilters,
  });

  return (
    <TableFilterToolbar className="bg-card rounded-xl border p-3" aria-label="系统日志查询">
      <form
        className="flex w-full min-w-0 flex-wrap items-center gap-3"
        onSubmit={form.handleSubmit(props.onSearch)}
      >
        <label className="flex min-w-0 basis-full items-center gap-2 lg:basis-64 lg:flex-1">
          <span className="text-muted-foreground shrink-0 whitespace-nowrap text-xs font-medium">
            请求 ID
          </span>
          <Input
            {...form.register("requestId")}
            aria-label="request_id"
            autoComplete="off"
            placeholder="输入完整 request_id"
          />
        </label>
        <Controller
          control={form.control}
          name="timeRange"
          render={({ field }) => (
            <div className="flex min-w-0 basis-52 flex-1 items-center gap-2 lg:basis-auto lg:flex-none">
              <span className="text-muted-foreground shrink-0 whitespace-nowrap text-xs font-medium">
                时间范围
              </span>
              <Select
                value={field.value}
                onValueChange={field.onChange}
                items={timeRangeOptions}
                itemToStringLabel={(value) =>
                  timeRangeOptions.find((option) => option.value === value)?.label ?? value
                }
              >
                <SelectTrigger aria-label="时间范围" className="min-w-0 flex-1 lg:w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent searchable={false}>
                  {timeRangeOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        />
        <Controller
          control={form.control}
          name="level"
          render={({ field }) => (
            <div className="flex min-w-0 basis-52 flex-1 items-center gap-2 lg:basis-auto lg:flex-none">
              <span className="text-muted-foreground shrink-0 whitespace-nowrap text-xs font-medium">
                级别
              </span>
              <Select
                value={field.value}
                onValueChange={field.onChange}
                items={logLevelOptions}
                itemToStringLabel={(value) =>
                  logLevelOptions.find((option) => option.value === value)?.label ?? value
                }
              >
                <SelectTrigger aria-label="级别" className="min-w-0 flex-1 lg:w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent searchable={false}>
                  {logLevelOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        />
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <Button type="submit" disabled={props.pending}>
            {props.pending ? (
              <RefreshCw className="animate-spin" aria-hidden="true" />
            ) : (
              <Search aria-hidden="true" />
            )}
            {props.pending ? "查询中" : "查询"}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              form.reset(defaultSystemLogFilters);
              props.onReset();
            }}
          >
            <RotateCcw aria-hidden="true" />
            重置
          </Button>
        </div>
      </form>
    </TableFilterToolbar>
  );
}
