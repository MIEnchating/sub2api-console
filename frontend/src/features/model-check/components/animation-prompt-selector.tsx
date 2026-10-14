import type { ReactElement } from "react";
import type { AnimationPromptSettings } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const defaultValue = ":default";
export function AnimationPromptSelector(props: {
  settings?: AnimationPromptSettings;
  pending: boolean;
  refreshing: boolean;
  onRetry: () => void;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}): ReactElement {
  if (!props.settings) {
    if (props.pending) return <ContentLoading compact label="正在读取动画提示词" />;
    return <ContentRetry compact onRetry={props.onRetry} pending={props.refreshing} />;
  }
  const templates = props.settings.templates;
  const defaultName = templates.find((item) => item.id === props.settings?.default_id)?.name;
  const defaultLabel = defaultName ? `使用默认（${defaultName}）` : "使用默认";
  const selected = templates.find((item) => item.id === props.value);
  return (
    <div className="grid min-w-0 gap-1.5">
      <span className="text-sm font-medium">动画提示词</span>
      <Select
        value={props.value || defaultValue}
        disabled={props.disabled}
        onValueChange={(value) => {
          if (value) props.onChange(value === defaultValue ? "" : value);
        }}
        itemToStringLabel={(value) =>
          value === defaultValue
            ? defaultLabel
            : (templates.find((item) => item.id === value)?.name ?? "所选提示词已删除，请重新选择")
        }
      >
        <SelectTrigger
          className="w-full min-w-0"
          aria-label="选择动画提示词"
          aria-invalid={!!props.value && !selected}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={defaultValue}>{defaultLabel}</SelectItem>
          {templates.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
