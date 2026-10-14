import type { AccountStatus } from "@/api";
import type { ReactElement, ReactNode } from "react";
import { Popover } from "@base-ui/react/popover";
import { TextCursorInput, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { probePlaceholderLabel } from "../constants";

function DetailRow(props: { label: string; children: ReactNode }): ReactElement {
  return (
    <div className="grid grid-cols-[5rem_minmax(0,1fr)] items-baseline gap-3">
      <dt className="text-muted-foreground">{props.label}</dt>
      <dd className="min-w-0 break-all">{props.children}</dd>
    </div>
  );
}

export function AccountProbePlaceholder(props: { account: AccountStatus }): ReactElement | null {
  if (!props.account.probe_placeholder_at) return null;
  const observedAt = new Date(props.account.probe_placeholder_at);
  const hasTime = !Number.isNaN(observedAt.getTime());
  const details = props.account.probe_placeholder;
  return (
    <Popover.Root>
      <Popover.Trigger
        aria-label="查看探活首字异常详情"
        title={probePlaceholderLabel}
        render={
          <Button variant="ghost" size="icon" className="text-amber-700 dark:text-amber-400" />
        }
      >
        <TextCursorInput aria-hidden="true" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner
          side="bottom"
          align="start"
          sideOffset={6}
          collisionPadding={16}
          className="z-50"
        >
          <Popover.Popup className="max-h-[var(--available-height)] w-96 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-xl border bg-popover text-popover-foreground shadow-lg outline-none">
            <header className="flex items-start justify-between gap-3 border-b p-4">
              <div className="min-w-0 space-y-1">
                <Popover.Title className="flex items-center gap-2 text-sm font-semibold">
                  <TextCursorInput
                    aria-hidden="true"
                    className="size-4 shrink-0 text-amber-700 dark:text-amber-400"
                  />
                  {probePlaceholderLabel}
                </Popover.Title>
                <Popover.Description className="text-xs leading-relaxed text-muted-foreground">
                  最近一次异常记录，不代表本次探活失败。
                </Popover.Description>
              </div>
              <Popover.Close
                render={<Button variant="ghost" size="icon" aria-label="关闭异常详情" />}
              >
                <X aria-hidden="true" />
              </Popover.Close>
            </header>
            <div className="space-y-4 p-4">
              {details?.preview ? (
                <section aria-label="异常片段" className="space-y-2">
                  <h3 className="text-xs font-medium">异常片段</h3>
                  <pre
                    aria-label="异常片段原文"
                    className="rounded-lg border bg-muted/50 p-3 font-mono text-sm leading-relaxed break-all whitespace-pre-wrap"
                  >
                    {details.preview}
                  </pre>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    引号内为返回内容，换行、制表符等不可见字符以转义形式显示。
                  </p>
                </section>
              ) : (
                <section
                  aria-label="原文缺失"
                  className="space-y-1 rounded-lg border border-dashed bg-muted/30 p-3"
                >
                  <h3 className="text-sm font-medium">未保存原文</h3>
                  <p className="text-xs leading-relaxed text-muted-foreground">
                    这条历史记录仅保留了异常时间，无法还原当时的字符。
                  </p>
                </section>
              )}
              <dl className="space-y-2 text-xs leading-relaxed">
                <DetailRow label="发生时间">
                  {hasTime ? (
                    <time dateTime={props.account.probe_placeholder_at}>
                      {observedAt.toLocaleString("zh-CN", { hour12: false })}
                    </time>
                  ) : (
                    "时间未记录"
                  )}
                </DetailRow>
                {details?.group_name ? (
                  <DetailRow label="探活分组">{details.group_name}</DetailRow>
                ) : null}
                {details?.request_model ? (
                  <DetailRow label="请求模型">{details.request_model}</DetailRow>
                ) : null}
                {details?.attempt ? (
                  <DetailRow label="异常出现于">第 {details.attempt} 次尝试</DetailRow>
                ) : null}
                {details?.result ? <DetailRow label="当次结果">{details.result}</DetailRow> : null}
              </dl>
            </div>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
