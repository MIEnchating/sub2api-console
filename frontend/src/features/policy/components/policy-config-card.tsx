import { useId, type ReactElement, type ReactNode } from "react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";

export type PolicyConfigCardProps = {
  title: string;
  description: string;
  children: ReactNode;
  columns?: 2 | 3;
  wide?: boolean;
  help?: ReactNode;
  switchAction?: {
    checked: boolean;
    label: string;
    text?: string;
    disabled?: boolean;
    onCheckedChange: (value: boolean) => void;
  };
};

export function PolicyConfigCard(props: PolicyConfigCardProps): ReactElement {
  const id = useId();
  const title = <CardTitle id={`${id}-title`}>{props.title}</CardTitle>;
  const action = props.switchAction;
  const actionText = action?.text ?? (action?.checked ? "已启用" : "已关闭");

  return (
    <Card
      role="region"
      aria-labelledby={`${id}-title`}
      className={cn("@container/policy-card", props.wide && "xl:col-span-2")}
      data-policy-section={props.title}
    >
      <CardHeader className="bg-muted/15 flex flex-col items-stretch gap-3">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            {action ? (
              <label
                className={cn(action.disabled ? "cursor-not-allowed" : "cursor-pointer")}
                htmlFor={`${id}-switch`}
              >
                {title}
              </label>
            ) : (
              title
            )}
            <CardDescription className="mt-1.5 text-xs leading-5">
              {props.description}
            </CardDescription>
          </div>
          {action ? (
            <div className="bg-background/60 flex shrink-0 items-center gap-2 rounded-full border px-2.5 py-1.5">
              <span id={`${id}-toggle-name`} className="sr-only">
                {action.label}
              </span>
              <label
                htmlFor={`${id}-switch`}
                className={cn(
                  "text-muted-foreground hidden text-xs whitespace-nowrap @min-[28rem]/policy-card:block",
                  action.disabled ? "cursor-not-allowed" : "cursor-pointer",
                )}
              >
                {actionText}
              </label>
              <Switch
                id={`${id}-switch`}
                checked={action.checked}
                disabled={action.disabled}
                aria-label={action.label}
                aria-labelledby={`${id}-toggle-name`}
                onCheckedChange={action.onCheckedChange}
              />
            </div>
          ) : null}
        </div>
        {props.help}
      </CardHeader>
      <CardContent
        data-slot="policy-fields"
        className={cn(
          "grid grid-cols-1 items-start gap-x-5 gap-y-5 @min-[28rem]/policy-card:grid-cols-2",
          props.columns === 3 && "@min-[56rem]/policy-card:grid-cols-3",
        )}
      >
        {props.children}
      </CardContent>
    </Card>
  );
}
