import { Autocomplete } from "@base-ui/react/autocomplete";
import { ChevronDown } from "lucide-react";
import { useRef, type ComponentProps, type ReactElement } from "react";
import { ComboboxContent, ComboboxEmpty, ComboboxList } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

type SuggestionInputProps = Omit<
  ComponentProps<"input">,
  "value" | "defaultValue" | "onChange" | "list" | "type" | "size"
> & {
  options: readonly string[];
  value: string;
  onValueChange: (value: string) => void;
  emptyText?: string;
};

/** 可自由输入的建议列表，使用与其他下拉控件一致的主题和浮层。 */
export function SuggestionInput(props: SuggestionInputProps): ReactElement {
  const { options, value, onValueChange, emptyText, className, ...inputProps } = props;
  const anchor = useRef<HTMLDivElement>(null);

  return (
    <Autocomplete.Root
      items={options}
      value={value}
      onValueChange={onValueChange}
      disabled={props.disabled}
      modal={false}
      openOnInputClick
    >
      <div ref={anchor} className={cn("relative min-w-0", className)}>
        <Autocomplete.Input {...inputProps} render={<Input className="pr-8" />} />
        <Autocomplete.Trigger
          aria-label="展开建议"
          className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 absolute inset-y-0 right-0 flex w-8 items-center justify-center rounded-r-lg outline-none focus-visible:ring-2 focus-visible:ring-inset disabled:pointer-events-none disabled:opacity-50 [&[data-popup-open]>svg]:rotate-180"
        >
          <ChevronDown aria-hidden="true" className="size-4 transition-transform duration-150" />
        </Autocomplete.Trigger>
      </div>
      <ComboboxContent anchor={anchor}>
        <ComboboxEmpty className="justify-center px-3 py-3 text-xs leading-relaxed">
          {value ? "无匹配建议" : (emptyText ?? "暂无建议")}
        </ComboboxEmpty>
        <ComboboxList>
          {(option: string) => (
            <Autocomplete.Item
              key={option}
              value={option}
              className="data-highlighted:bg-accent data-highlighted:text-accent-foreground flex min-h-8 w-full min-w-0 cursor-default items-center rounded-md px-2.5 py-1.5 text-sm leading-5 outline-none select-none"
            >
              <span className="min-w-0 [overflow-wrap:anywhere]">{option}</span>
            </Autocomplete.Item>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Autocomplete.Root>
  );
}
