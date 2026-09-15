import type { ReactElement } from "react";
import type { WorkbenchRunRow } from "@/api";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const kindLabels: Record<string, string> = {
  oauth_login: "登录资料",
  refresh_token: "Refresh Token",
  codex_json: "Codex JSON",
  sub2api_json: "Sub2API JSON",
};
const statusLabels: Record<string, string> = {
  queued: "等待处理",
  running: "正在处理",
  waiting_input: "等待完成登录",
  ready: "已就绪",
  succeeded: "处理成功",
  failed: "处理失败",
  cancelled: "已取消",
};
export function WorkbenchMixedRows(props: { rows: WorkbenchRunRow[] }): ReactElement {
  return (
    <Table
      aria-label="本批账号"
      className="min-w-[34rem]"
      containerClassName="max-h-72 overflow-auto"
    >
      <TableHeader>
        <TableRow>
          <TableHead>序号</TableHead>
          <TableHead>账号</TableHead>
          <TableHead>来源</TableHead>
          <TableHead>状态</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.rows.map((row) => (
          <TableRow key={row.index}>
            <TableCell>{row.index + 1}</TableCell>
            <TableCell overflowTooltip={false} className="max-w-xs whitespace-normal wrap-anywhere">
              <p>{row.name || row.email || `第 ${row.index + 1} 项`}</p>
              {row.email && <p className="text-xs text-muted-foreground">{row.email}</p>}
              {row.workspace_id && (
                <p className="text-xs text-muted-foreground">工作区：{row.workspace_id}</p>
              )}
            </TableCell>
            <TableCell overflowTooltip={false} className="whitespace-normal wrap-anywhere">
              <p>{kindLabels[row.kind] ?? "账号资料"}</p>
              <p className="text-xs text-muted-foreground">
                {[
                  row.has_password ? "密码" : "",
                  row.has_totp ? "TOTP" : "",
                  row.has_proxy ? "登录代理" : "",
                  row.sms_provider ? "自动接码" : "",
                ]
                  .filter(Boolean)
                  .join("、")}
              </p>
            </TableCell>
            <TableCell overflowTooltip={false} className="max-w-xs whitespace-normal wrap-anywhere">
              <Badge variant="outline">{statusLabels[row.status] ?? "等待处理"}</Badge>
              <p className="mt-1 text-xs text-muted-foreground">{row.message}</p>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
