import type { ReactElement } from "react";

import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatLogValue, logStatusLabel, logStatusVariant } from "../lib/log-display";

type LogRecord = Record<string, unknown>;

function recordValue(record: LogRecord, key: string): string | null {
  const value = record[key];
  if (value === null || value === undefined || value === "") return null;
  return formatLogValue(value, key);
}

function recordRawText(record: LogRecord, key: string): string | null {
  const value = record[key];
  if (value === null || value === undefined || value === "") return null;
  return String(value);
}

function accountLabel(record: LogRecord, index: number): string {
  return (
    recordRawText(record, "account_name") ??
    recordRawText(record, "account_id") ??
    `第 ${index + 1} 项`
  );
}

function changedValue(record: LogRecord, beforeKey: string, afterKey: string): string {
  const before = recordValue(record, beforeKey);
  const after = recordValue(record, afterKey);
  if (before === null && after === null) return "未记录";
  if (before === null) return after ?? "未记录";
  if (after === null || before === after) return before;
  return `${before} → ${after}`;
}

function rateValue(record: LogRecord): string {
  if (record.before !== undefined || record.after !== undefined) {
    return changedValue(record, "before", "after");
  }
  return recordValue(record, "account_multiplier") ?? "未记录";
}

function nameChange(record: LogRecord): string {
  const before = recordRawText(record, "name_before");
  const after = recordRawText(record, "name_after");
  if (after === null) return "无需调整";
  if (before === after) return "未变化";
  if (before === null) return `改为 ${after}`;
  return `${before} → ${after}`;
}

function statusVariant(status: string): StatusVariant {
  if (/失败|错误|异常/.test(status)) return "danger";
  if (/降级|跳过|警告|不存在|未绑定/.test(status)) return "warning";
  if (/成功|同步|一致|完成/.test(status)) return "success";
  return logStatusVariant(status);
}

function statusDetail(record: LogRecord): string | null {
  return (
    recordValue(record, "error") ??
    recordValue(record, "reason") ??
    recordValue(record, "probe_error")
  );
}

function remoteWriteLabel(record: LogRecord): string {
  if (record.remote_write !== true) return "未写入";
  return record.readback_confirmed === true ? "已写入并确认" : "已写入，未确认";
}

export function isAccountRateSyncItems(records: LogRecord[]): boolean {
  return records.some(
    (record) => "account_multiplier" in record || "upstream_raw_multiplier" in record,
  );
}

export function AccountRateSyncResultTable(props: {
  records: LogRecord[];
  startIndex: number;
}): ReactElement {
  return (
    <Table aria-label="账号倍率同步执行明细" className="min-w-[1100px]" overflowTooltip={false}>
      <TableHeader>
        <TableRow>
          <TableHead className="w-44">账号</TableHead>
          <TableHead className="w-52">上游</TableHead>
          <TableHead className="w-52">倍率</TableHead>
          <TableHead className="w-52">名称处理</TableHead>
          <TableHead className="w-28">数据来源</TableHead>
          <TableHead className="w-32">远程执行</TableHead>
          <TableHead className="w-52">状态</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody className="[&>tr]:h-auto">
        {props.records.map((record, index) => {
          const absoluteIndex = props.startIndex + index;
          const account = accountLabel(record, absoluteIndex);
          const accountID = recordRawText(record, "account_id");
          const upstreamHost = recordRawText(record, "upstream_host");
          const platform = recordRawText(record, "platform");
          const upstreamRate = recordValue(record, "upstream_raw_multiplier");
          const rechargeRate = recordValue(record, "recharge_rate");
          const source = recordValue(record, "observation_source") ?? "未记录";
          const status = recordRawText(record, "status") ?? "未记录";
          const detail = statusDetail(record);
          return (
            <TableRow key={accountID ?? `${account}:${absoluteIndex}`}>
              <TableCell className="whitespace-normal">
                <span className="block break-words font-medium">{account}</span>
                {accountID ? (
                  <span className="text-muted-foreground block text-xs">ID {accountID}</span>
                ) : null}
              </TableCell>
              <TableCell className="whitespace-normal">
                <span className="block break-words">{upstreamHost ?? "未记录"}</span>
                {platform ? (
                  <span className="text-muted-foreground block text-xs">{platform}</span>
                ) : null}
              </TableCell>
              <TableCell className="whitespace-normal">
                <span className="block font-medium">{rateValue(record)}</span>
                {upstreamRate || rechargeRate ? (
                  <span className="text-muted-foreground block text-xs">
                    上游 {upstreamRate ?? "-"} · 充值 {rechargeRate ?? "-"}
                  </span>
                ) : null}
              </TableCell>
              <TableCell className="whitespace-normal">
                <span className="block break-words">{nameChange(record)}</span>
              </TableCell>
              <TableCell className="whitespace-normal">{source}</TableCell>
              <TableCell className="whitespace-normal">{remoteWriteLabel(record)}</TableCell>
              <TableCell className="whitespace-normal">
                <StatusBadge label={logStatusLabel(status)} variant={statusVariant(status)} />
                {detail ? (
                  <span className="text-muted-foreground mt-1 block break-words text-xs leading-5">
                    {detail}
                  </span>
                ) : null}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
