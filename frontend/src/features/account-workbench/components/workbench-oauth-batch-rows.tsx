import type { ReactElement } from "react";
import type { WorkbenchOAuthBatchRow } from "@/api";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { batchRowStatusLabels } from "../constants";
import { smsProviderOptions } from "../lib/oauth-sms-schema";

export function WorkbenchOAuthBatchRows(props: { items: WorkbenchOAuthBatchRow[] }): ReactElement {
  return (
    <Table
      aria-label="批量授权账号"
      className="min-w-[38rem]"
      containerClassName="max-h-80 overflow-auto"
    >
      <TableHeader>
        <TableRow>
          <TableHead>账号</TableHead>
          <TableHead>登录方式</TableHead>
          <TableHead>短信验证码</TableHead>
          <TableHead>状态</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.items.map((item) => (
          <TableRow key={item.index}>
            <TableCell overflowTooltip={false} className="whitespace-normal wrap-anywhere">
              <p>{item.email}</p>
              {item.account_id && (
                <p className="text-xs text-muted-foreground">
                  账号 ID：{item.account_id}；资料版本：{item.profile_revision}
                </p>
              )}
              {item.workspace_id && (
                <p className="text-xs text-muted-foreground">工作区：{item.workspace_id}</p>
              )}
            </TableCell>
            <TableCell overflowTooltip={false} className="whitespace-normal wrap-anywhere">
              {[
                item.has_password ? "密码" : "邮箱验证码",
                item.has_totp ? "2FA" : "",
                item.mail_kind === "microsoft" ? "Microsoft 邮箱" : "",
                item.mail_kind === "http" ? "HTTP 邮箱" : "",
              ]
                .filter(Boolean)
                .join("、")}
            </TableCell>
            <TableCell>
              {smsProviderOptions.find((option) => option.value === item.sms_provider)?.label ||
                "人工填写"}
            </TableCell>
            <TableCell overflowTooltip={false} className="whitespace-normal wrap-anywhere">
              <Badge variant="outline">{batchRowStatusLabels[item.status]}</Badge>
              <p className="mt-1 text-xs text-muted-foreground">{item.message}</p>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
