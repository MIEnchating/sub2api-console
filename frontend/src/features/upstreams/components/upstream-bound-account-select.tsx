import { useState } from "react";

import type { UpstreamBoundAccount } from "@/api";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

function accountLabel(account: UpstreamBoundAccount): string {
  if (!account.account_exists || account.binding_status === "missing") return "管理平台不存在";
  return account.account_name?.trim() || "账号已删除";
}

function accountSummary(account: UpstreamBoundAccount): string {
  const label = accountLabel(account);
  if (!account.account_exists || account.binding_status === "missing") return label;
  return `${label} · 本地分组：${account.local_group}`;
}

export function UpstreamBoundAccountSelect(props: { accounts: UpstreamBoundAccount[] }) {
  if (props.accounts.length === 0) {
    return <span className="text-muted-foreground text-sm">未绑定</span>;
  }
  return (
    <BoundAccountSelect
      key={props.accounts.map((account) => account.binding_id).join(":")}
      accounts={props.accounts}
    />
  );
}

function BoundAccountSelect(props: { accounts: UpstreamBoundAccount[] }) {
  const first = props.accounts[0];
  const [selectedBindingID, setSelectedBindingID] = useState(String(first.binding_id));
  const selectedAccount =
    props.accounts.find((account) => String(account.binding_id) === selectedBindingID) ?? first;
  const selectedSummary = accountSummary(selectedAccount);
  const selectedAccountExists =
    selectedAccount.account_exists && selectedAccount.binding_status !== "missing";
  return (
    <Select
      value={String(selectedAccount.binding_id)}
      onValueChange={(value) => {
        if (value !== null) setSelectedBindingID(value);
      }}
      itemToStringLabel={(bindingID) => {
        const account = props.accounts.find((item) => String(item.binding_id) === bindingID);
        return account ? accountLabel(account) : "绑定账号不可用";
      }}
    >
      <SelectTrigger size="sm" className="min-w-0" aria-label="绑定账号" data-layout="single-line">
        <Tooltip>
          <TooltipTrigger
            render={
              <span className="min-w-0 flex-1 truncate text-left" aria-label={selectedSummary} />
            }
          >
            <span className="font-medium">{accountLabel(selectedAccount)}</span>
            {selectedAccountExists ? (
              <span className="text-muted-foreground">
                {" · "}本地分组：{selectedAccount.local_group}
              </span>
            ) : null}
          </TooltipTrigger>
          <TooltipContent>{selectedSummary}</TooltipContent>
        </Tooltip>
      </SelectTrigger>
      <SelectContent className="min-w-72" align="start">
        {props.accounts.map((account) => (
          <SelectItem key={account.binding_id} value={String(account.binding_id)}>
            <span className="min-w-0 truncate">
              <span>{accountLabel(account)}</span>
              {account.account_exists && account.binding_status !== "missing" ? (
                <span className="text-muted-foreground">
                  {" · "}本地分组：{account.local_group}
                </span>
              ) : null}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export { accountLabel as upstreamBoundAccountLabel };
