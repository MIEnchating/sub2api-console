import { Eye, EyeOff, RotateCcw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";

export type NavigationSettingsSection<T extends string> = {
  label: string;
  items: Array<{ id: T; label: string; path: string }>;
};

export type NavigationSettingsCardProps<T extends string> = {
  sections: Array<NavigationSettingsSection<T>>;
  hiddenItemIDs: ReadonlySet<T>;
  lockedItemIDs: ReadonlySet<T>;
  onItemVisibilityChange: (itemID: T, visible: boolean) => void;
  onReset: () => void;
};

export function NavigationSettingsCard<T extends string>(props: NavigationSettingsCardProps<T>) {
  const totalItems = props.sections.reduce((count, section) => count + section.items.length, 0);
  const visibleItems = totalItems - props.hiddenItemIDs.size;

  return (
    <Card size="sm" data-testid="navigation-settings-card">
      <CardHeader className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <CardTitle>菜单设置</CardTitle>
          <CardDescription className="mt-1">
            控制左侧菜单显示的路由入口，不影响通过地址直接访问页面
          </CardDescription>
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={props.hiddenItemIDs.size === 0}
          onClick={props.onReset}
        >
          <RotateCcw aria-hidden="true" />
          恢复默认
        </Button>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="text-muted-foreground flex items-center gap-2 text-xs" role="status">
          {props.hiddenItemIDs.size > 0 ? (
            <EyeOff aria-hidden="true" className="size-3.5" />
          ) : (
            <Eye aria-hidden="true" className="size-3.5" />
          )}
          当前显示 {visibleItems} / {totalItems} 个菜单入口
        </div>
        {props.sections.map((section) => (
          <fieldset key={section.label} className="border-border/70 min-w-0 border-t pt-3">
            <legend className="pr-2 text-xs font-medium">{section.label}</legend>
            <div className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
              {section.items.map((item) => {
                const locked = props.lockedItemIDs.has(item.id);
                const visible = locked || !props.hiddenItemIDs.has(item.id);
                const controlID = `navigation-item-${item.id}`;
                return (
                  <div className="flex min-w-0 items-center justify-between gap-3" key={item.id}>
                    <label
                      className={locked ? "min-w-0 cursor-not-allowed" : "min-w-0 cursor-pointer"}
                      htmlFor={controlID}
                    >
                      <span className="block truncate text-sm font-medium">{item.label}</span>
                      <span className="text-muted-foreground block truncate text-xs">
                        {locked ? `${item.path} · 始终显示` : item.path}
                      </span>
                    </label>
                    <Switch
                      id={controlID}
                      size="sm"
                      checked={visible}
                      disabled={locked}
                      aria-label={`在菜单中显示${item.label}`}
                      onCheckedChange={(checked) => props.onItemVisibilityChange(item.id, checked)}
                    />
                  </div>
                );
              })}
            </div>
          </fieldset>
        ))}
      </CardContent>
    </Card>
  );
}
