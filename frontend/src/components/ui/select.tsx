"use client";

import { Select as SelectPrimitive } from "@base-ui/react/select";
import { Check, ChevronDown, CirclePlus, Search } from "lucide-react";
import * as React from "react";
import { knownUpstreamTypeLabel } from "@/lib/domain-dictionaries";
import { cn } from "@/lib/utils";

const selectLabels: Record<string, string> = {
  balanced: "均衡",
  price: "价格优先",
  price_first: "价格优先",
  speed: "速度优先",
  latency_first: "速度优先",
  speed_first: "速度优先",
  cost_first: "价格优先",
  reliability: "稳定优先",
  reliability_first: "稳定优先",
  stable: "稳定优先",
  stability: "稳定优先",
  stability_first: "稳定优先",
  current_cost_wall: "回退当前成本墙",
  fail_closed: "严格关闭",
  fail_open: "允许继续",
  traffic: "真实流量样本",
  logs: "运维平台日志",
  probe: "主动探测",
  active_probe: "主动探测",
  hybrid: "流量与探测",
  new: "新账号",
  existing: "已有账号",
  sub2api_user_token: "Token + 刷新 Token",
  newapi_admin_key: "Admin Key + 用户 ID",
  newapi_user_token: "Token",
  bearer_token: "Bearer Token",
  custom_headers: "自定义 Header / Cookie",
  cookie: "Cookie",
  sub2api_user_login: "密码箱登录",
  newapi_user_login: "密码箱登录",
  sub2api_manual_login: "自定义账号密码",
  newapi_manual_login: "自定义账号密码",
  c2c: "私聊",
  group: "群聊",
  channel: "频道",
};

function selectValueLabel(value: unknown): string {
  const raw = String(value);
  const vaultSeparator = raw.indexOf("\u0000");
  if (vaultSeparator >= 0) {
    return `${raw.slice(0, vaultSeparator)} / ${raw.slice(vaultSeparator + 1)}`;
  }
  return selectLabels[raw] ?? knownUpstreamTypeLabel(raw) ?? raw;
}

function Select<Value>(props: SelectPrimitive.Root.Props<Value, false>) {
  return (
    <SelectPrimitive.Root
      {...props}
      itemToStringLabel={props.itemToStringLabel ?? ((value: Value) => selectValueLabel(value))}
    />
  );
}

function SelectGroup({ className, ...props }: SelectPrimitive.Group.Props) {
  return (
    <SelectPrimitive.Group
      data-slot="select-group"
      className={cn("scroll-my-1 p-1", className)}
      {...props}
    />
  );
}

function SelectValue({ className, ...props }: SelectPrimitive.Value.Props) {
  return (
    <SelectPrimitive.Value
      data-slot="select-value"
      className={cn("flex flex-1 text-left", className)}
      {...props}
    />
  );
}

function SelectTrigger({
  className,
  size = "default",
  appearance = "classic",
  children,
  ...props
}: SelectPrimitive.Trigger.Props & {
  size?: "sm" | "default";
  appearance?: "faceted" | "classic";
}) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      data-size={size}
      data-appearance={appearance}
      className={cn(
        "border-input focus-visible:border-ring focus-visible:ring-ring/50 data-popup-open:border-ring data-popup-open:ring-ring/30 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-placeholder:text-muted-foreground dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 flex w-full max-w-full items-center gap-1.5 rounded-lg border bg-transparent py-2 text-sm whitespace-nowrap transition-colors outline-none select-none focus-visible:ring-3 data-popup-open:ring-1 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:ring-3 data-[size=default]:h-8 data-[size=sm]:h-7 data-[size=sm]:rounded-md *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:min-w-0 *:data-[slot=select-value]:flex-1 *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-1.5 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        appearance === "classic"
          ? "dark:bg-input/30 dark:hover:bg-input/50 justify-between pr-2 pl-2.5"
          : "dark:bg-input/20 dark:hover:bg-input/40 !h-8 justify-start pr-2.5 pl-2.5",
        className,
      )}
      {...props}
      data-press-animation="none"
    >
      {appearance === "faceted" ? (
        <CirclePlus className="size-4 shrink-0" aria-hidden="true" />
      ) : null}
      {children}
      {appearance === "classic" ? (
        <SelectPrimitive.Icon
          render={
            <ChevronDown className="text-muted-foreground size-4 transition-transform duration-150 data-popup-open:rotate-180" />
          }
        />
      ) : null}
    </SelectPrimitive.Trigger>
  );
}

function selectItemText(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (React.isValidElement<{ children?: React.ReactNode }>(node)) {
    return selectItemText(node.props.children);
  }
  return React.Children.toArray(node).map(selectItemText).join(" ");
}

function selectOptionMatches(value: unknown, label: string, search: string): boolean {
  const query = search.trim().toLocaleLowerCase();
  if (!query) return true;
  return `${String(value)} ${label}`.toLocaleLowerCase().includes(query);
}

function filterSelectChildren(node: React.ReactNode, search: string): React.ReactNode {
  const query = search.trim().toLocaleLowerCase();
  if (!query) return node;

  return React.Children.map(node, (child) => {
    if (!React.isValidElement<{ children?: React.ReactNode; value?: unknown }>(child)) return child;
    if (child.type === SelectItem) {
      return selectOptionMatches(child.props.value, selectItemText(child.props.children), query)
        ? child
        : null;
    }
    if (child.type === SelectGroup || child.type === React.Fragment) {
      const filtered = filterSelectChildren(child.props.children, search);
      if (React.Children.toArray(filtered).length === 0) return null;
      return React.cloneElement(child, undefined, filtered);
    }
    return child;
  });
}

export const selectContentAppearanceLayouts = {
  classic: "flex min-w-36 flex-col overflow-hidden",
  faceted: "flex min-w-60 flex-col overflow-hidden",
} as const;

export const selectContentSearchableByDefault = true;

function SelectContent({
  className,
  children,
  portalContainer,
  side = "bottom",
  sideOffset = 4,
  align = "center",
  alignOffset = 0,
  alignItemWithTrigger = false,
  appearance = "classic",
  searchable = selectContentSearchableByDefault,
  ...props
}: SelectPrimitive.Popup.Props &
  Pick<
    SelectPrimitive.Positioner.Props,
    "align" | "alignOffset" | "side" | "sideOffset" | "alignItemWithTrigger"
  > & {
    portalContainer?: SelectPrimitive.Portal.Props["container"];
    appearance?: "faceted" | "classic";
    searchable?: boolean;
  }) {
  const [search, setSearch] = React.useState("");
  const filteredChildren = filterSelectChildren(children, search);
  const hasFilteredChildren = React.Children.toArray(filteredChildren).length > 0;
  const content = (
    <SelectPrimitive.Positioner
      side={side}
      sideOffset={sideOffset}
      align={align}
      alignOffset={alignOffset}
      alignItemWithTrigger={alignItemWithTrigger}
      className="isolate z-50"
    >
      <SelectPrimitive.Popup
        data-slot="select-content"
        data-align-trigger={alignItemWithTrigger}
        data-appearance={appearance}
        className={cn(
          "bg-popover text-popover-foreground ring-foreground/10 relative isolate z-50 max-h-(--available-height) w-(--anchor-width) origin-(--transform-origin) rounded-lg shadow-md ring-1",
          selectContentAppearanceLayouts[appearance],
          !alignItemWithTrigger &&
            "transition-[opacity,scale] duration-100 ease-out data-ending-style:scale-95 data-ending-style:opacity-0 data-starting-style:scale-95 data-starting-style:opacity-0",
          className,
        )}
        {...props}
      >
        {searchable ? (
          <div className="relative m-2 mb-1">
            <Search
              className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
              aria-hidden="true"
            />
            <input
              type="text"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              onKeyDown={(event) => {
                if (!["ArrowDown", "ArrowUp", "Enter", "Escape"].includes(event.key)) {
                  event.stopPropagation();
                }
              }}
              placeholder="搜索选项"
              aria-label="搜索选项"
              data-slot="select-search"
              className="border-input/50 bg-input/30 placeholder:text-muted-foreground h-8 w-full rounded-lg border pr-2.5 pl-8 text-sm outline-none"
            />
          </div>
        ) : null}
        <SelectPrimitive.List className="no-scrollbar min-h-0 overflow-y-auto p-1">
          {hasFilteredChildren ? (
            filteredChildren
          ) : (
            <div className="text-muted-foreground py-6 text-center text-sm">没有匹配的选项</div>
          )}
        </SelectPrimitive.List>
      </SelectPrimitive.Popup>
    </SelectPrimitive.Positioner>
  );
  return <SelectPrimitive.Portal container={portalContainer}>{content}</SelectPrimitive.Portal>;
}

function SelectItem({
  className,
  children,
  appearance = "classic",
  ...props
}: SelectPrimitive.Item.Props & { appearance?: "faceted" | "classic" }) {
  return (
    <SelectPrimitive.Item
      data-slot="select-item"
      data-appearance={appearance}
      className={cn(
        "focus:bg-accent focus:text-accent-foreground not-data-[variant=destructive]:focus:**:text-accent-foreground relative flex w-full cursor-default items-center rounded-md text-sm outline-hidden select-none data-disabled:pointer-events-none data-disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        appearance === "classic"
          ? "gap-1.5 py-1 pr-8 pl-1.5"
          : "group/select-item gap-2 px-2 py-1.5",
        className,
      )}
      {...props}
    >
      {appearance === "faceted" ? (
        <span className="border-primary/50 flex size-4 shrink-0 items-center justify-center rounded-full border">
          <SelectPrimitive.ItemIndicator
            data-slot="select-item-indicator"
            className="bg-primary text-primary-foreground -m-px flex size-4 items-center justify-center rounded-full"
          >
            <Check className="size-3" aria-hidden="true" />
          </SelectPrimitive.ItemIndicator>
        </span>
      ) : null}
      <SelectPrimitive.ItemText
        data-slot="select-item-text"
        className={cn(
          "flex flex-1 gap-2",
          appearance === "classic" ? "shrink-0 whitespace-nowrap" : "min-w-0",
        )}
      >
        {children}
      </SelectPrimitive.ItemText>
      {appearance === "classic" ? (
        <SelectPrimitive.ItemIndicator
          render={
            <span className="pointer-events-none absolute right-2 flex size-4 items-center justify-center" />
          }
        >
          <Check className="size-4" aria-hidden="true" />
        </SelectPrimitive.ItemIndicator>
      ) : null}
    </SelectPrimitive.Item>
  );
}

export {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  selectOptionMatches,
  selectValueLabel,
};
